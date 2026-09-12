import * as React from "react";
import { ArrowRight, CheckCircle2, FlaskConical, Glasses, Plus, Printer, Send, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { LabOrderDocumentDialog } from "../components/lab-document";
import { PrintHeader, triggerPrint } from "../components/print";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "../components/ui/dialog";
import { Badge, EmptyState, ErrorState, Pager, Skeleton } from "../components/ui/data";
import { Field, FieldGroup, Input, Select, Textarea } from "../components/ui/input";
import { DateTimeInput } from "../components/ui/date-time";
import { useLoad, usePagedList } from "../hooks";
import { dateTime, money } from "../lib";
import { useRealtime } from "../realtime";
import type { LabOrder, LabOrderKind, LabOrderTest } from "../types";
import { PatientPicker } from "../components/patient-search";
import { useCatalog, type CatalogName } from "../components/catalogs";

/**
 * Two kinds of order leave the clinic and come back: glasses to the company that
 * glazes and mounts them, and examinations to a laboratory. They travel the same
 * road — raised, sent, chased, collected — so they share this board and differ
 * in the stages between.
 *
 * A blood sample is never edged and never passes optical quality control, so a
 * request skips straight from the laboratory's bench to the patient.
 */
const stagesByKind: Record<LabOrderKind, string[]> = {
  optical: ["draft", "ordered", "at_lab", "received", "edging_mounting", "quality_control", "ready", "delivered"],
  medical: ["draft", "ordered", "at_lab", "received", "delivered"],
};

const emptyChecklist = {
  prescriptionVerified: false, powerVerified: false, axisVerified: false, frameCondition: false,
  lensCondition: false, fittingVerified: false, finalCleaning: false,
};
const checklistLabels: Record<keyof typeof emptyChecklist, string> = {
  prescriptionVerified: "Prescription matches the order",
  powerVerified: "Powers verified on the focimeter",
  axisVerified: "Cylinder axis verified",
  frameCondition: "Frame undamaged",
  lensCondition: "Lenses free of scratches and chips",
  fittingVerified: "Fitting and alignment checked",
  finalCleaning: "Cleaned and cased",
};

export function LabPage() {
  const { revision } = useRealtime();
  const [kind, setKind] = React.useState<LabOrderKind>("optical");
  const [creating, setCreating] = React.useState(false);
  const [opened, setOpened] = React.useState<string | null>(null);
  const [checking, setChecking] = React.useState<LabOrder | null>(null);
  const [delivering, setDelivering] = React.useState<LabOrder | null>(null);
  // A clinic sends a day's work out together, so orders are selected on the
  // board and moved or printed as one batch.
  const [selected, setSelected] = React.useState<string[]>([]);
  const [batchPrinting, setBatchPrinting] = React.useState(false);
  const [sending, setSending] = React.useState(false);
  const orders = usePagedList<LabOrder>((page, limit) => `/lab-orders?kind=${kind}&page=${page}&limit=${limit}`, [kind, revision]);
  const stages = stagesByKind[kind];

  const toggle = (id: string) => setSelected((current) => current.includes(id) ? current.filter((candidate) => candidate !== id) : [...current, id]);
  const switchKind = (next: LabOrderKind) => { setKind(next); setSelected([]); };

  const sendBatch = async (status: string) => {
    setSending(true);
    try {
      const result = await api.post<{ updated: number }>("/lab-orders/bulk-status", { orderIds: selected, status });
      toast.success(`${result.updated} order(s) marked ${status.replaceAll("_", " ")}`);
      setSelected([]);
      orders.reload();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "The batch was rolled back; nothing changed."); }
    finally { setSending(false); }
  };

  const moveTo = async (order: LabOrder, next: string, extra: Record<string, unknown> = {}) => {
    try {
      await api.patch(`/lab-orders/${order.id}/status`, { status: next, notes: "", receivedByName: "", version: order.version, ...extra });
      toast.success(`Order moved to ${next.replaceAll("_", " ")}`);
      orders.reload();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Status update failed"); }
  };

  // Quality control and delivery are the two steps that record a person's
  // judgement — that the glasses are right, and that somebody collected them.
  // Neither can be answered on the user's behalf by a button.
  const advance = (order: LabOrder) => {
    const next = stages[Math.min(stages.indexOf(order.status) + 1, stages.length - 1)];
    if (next === "ready" && order.kind === "optical") { setChecking(order); return; }
    if (next === "delivered") { setDelivering(order); return; }
    void moveTo(order, next);
  };

  const cancelled = orders.items.filter((item) => item.status === "cancelled");
  return (
    <div className="page">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="section-title">Outside work</p>
          <h1 className="page-title">Laboratory &amp; optical orders</h1>
          <p className="page-description">Glasses sent to the glazing company, and examinations sent to the laboratory — from the consultation through to the patient.</p>
        </div>
        <Button onClick={() => setCreating(true)}>
          <Plus className="h-4 w-4" />{kind === "optical" ? "New optical order" : "New exam request"}
        </Button>
      </div>

      <div className="mt-5 flex w-fit gap-1 rounded-md bg-[var(--muted)] p-1" role="tablist">
        {([["optical", "Optical orders", Glasses], ["medical", "Laboratory exams", FlaskConical]] as const).map(([value, label, Icon]) => (
          <button key={value} type="button" role="tab" aria-selected={kind === value} onClick={() => switchKind(value)}
            className={`flex min-h-10 items-center gap-2 rounded-[var(--radius)] px-4 text-sm font-semibold ${kind === value ? "bg-[var(--card)] shadow-sm" : "text-[var(--muted-foreground)]"}`}>
            <Icon className="h-4 w-4" />{label}
          </button>
        ))}
      </div>

      {selected.length > 0 && (
        <div className="mt-4 flex flex-wrap items-center gap-3 rounded-lg border border-black bg-[var(--muted)] p-3">
          <span className="text-sm font-semibold">{selected.length} order(s) selected</span>
          <div className="ml-auto flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => setBatchPrinting(true)}><Printer className="h-3.5 w-3.5" />Print requisition</Button>
            <Button size="sm" disabled={sending} onClick={() => sendBatch("at_lab")}><Send className="h-3.5 w-3.5" />Send out</Button>
            <Button size="sm" variant="ghost" onClick={() => setSelected([])}>Clear</Button>
          </div>
        </div>
      )}

      {orders.loading && !orders.items.length ? <Skeleton className="mt-6 h-96" />
        : orders.error ? <div className="mt-6"><ErrorState message={orders.error.message} retry={orders.reload} /></div>
        : orders.items.length ? <>
          <div className="mt-6 flex gap-4 overflow-x-auto pb-4">
            {stages.map((stage) => {
              const column = orders.items.filter((item) => item.status === stage);
              return <section key={stage} className="w-72 shrink-0">
                <div className="mb-3 flex items-center justify-between">
                  <h2 className="text-xs font-bold uppercase tracking-wide">{stage.replaceAll("_", " ")}</h2>
                  <Badge>{column.length}</Badge>
                </div>
                <div className="space-y-3">
                  {column.map((order) => (
                    <Card key={order.id}>
                      <CardContent className="p-4">
                        <div className="flex items-center justify-between">
                          <label className="flex items-center gap-2">
                            <input type="checkbox" aria-label={`Select ${order.orderNumber}`} checked={selected.includes(order.id)} onChange={() => toggle(order.id)} />
                            <span className="font-mono text-[11px] font-bold">{order.orderNumber}</span>
                          </label>
                          {order.expectedAt && new Date(order.expectedAt) < new Date() && !["ready", "delivered"].includes(order.status) && <Badge tone="danger">Delayed</Badge>}
                        </div>
                        <div className="mt-3 font-semibold">{order.patientName}</div>
                        <div className="mt-1 text-xs text-zinc-500">
                          {order.kind === "medical"
                            ? order.supplierName || "Laboratory not assigned"
                            : `${order.frameName || "Frame pending"} · ${order.lensType || "Lens pending"}`}
                        </div>
                        <div className="mt-3 text-[11px] text-zinc-500">Expected {order.expectedAt ? dateTime(order.expectedAt) : "not set"}</div>
                        <div className="mt-4 flex gap-2">
                          <Button size="sm" variant="ghost" onClick={() => setOpened(order.id)} aria-label={`Open ${order.orderNumber}`}><Printer className="h-3 w-3" /></Button>
                          {stage !== "delivered" && (
                            <Button className="flex-1" size="sm" variant="outline" onClick={() => advance(order)}>
                              {stage === "quality_control" ? <CheckCircle2 className="h-3 w-3" /> : <ArrowRight className="h-3 w-3" />}
                              Move to {stages[Math.min(stages.indexOf(stage) + 1, stages.length - 1)].replaceAll("_", " ")}
                            </Button>
                          )}
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </section>;
            })}
          </div>
          {/* Cancelled orders used to load into the page and appear in no column
              at all, which read as an order that had vanished. */}
          {cancelled.length > 0 && (
            <div className="rounded-lg border border-dashed p-3">
              <h2 className="text-xs font-bold uppercase tracking-wide text-zinc-500">Cancelled</h2>
              <div className="mt-2 flex flex-wrap gap-2">
                {cancelled.map((order) => (
                  <button key={order.id} type="button" onClick={() => setOpened(order.id)} className="rounded-md border px-3 py-1.5 text-left text-xs">
                    <span className="font-mono font-bold">{order.orderNumber}</span> · {order.patientName}
                  </button>
                ))}
              </div>
            </div>
          )}
          {/* The board shows one page of orders; the rest stay on the server. */}
          <Pager page={orders.page} pageSize={orders.pageSize} total={orders.total} hasMore={orders.hasMore} onPrevious={orders.previous} onNext={orders.next} />
        </> : (
          <div className="mt-6">
            <EmptyState
              title={kind === "optical" ? "No optical orders" : "No laboratory exam requests"}
              description={kind === "optical" ? "Start one from the patient's prescription and it carries the powers, the consultation and the sale with it." : "Choose the examinations from the clinic's list and print the request for the laboratory."}
              action={<Button onClick={() => setCreating(true)}>{kind === "optical" ? "Create optical order" : "Create exam request"}</Button>}
            />
          </div>
        )}

      <Dialog open={creating} onOpenChange={setCreating}>
        {creating && (kind === "optical"
          ? <OpticalOrderForm onSaved={() => { setCreating(false); orders.reload(); }} />
          : <ExamRequestForm onSaved={() => { setCreating(false); orders.reload(); }} />)}
      </Dialog>
      <Dialog open={Boolean(opened)} onOpenChange={(value) => !value && setOpened(null)}>
        {opened && <LabOrderDocumentDialog orderId={opened} />}
      </Dialog>
      <Dialog open={Boolean(checking)} onOpenChange={(value) => !value && setChecking(null)}>
        {checking && <QualityControlDialog order={checking} onPassed={async () => { const order = checking; setChecking(null); await moveTo(order, "ready"); }} />}
      </Dialog>
      <Dialog open={Boolean(delivering)} onOpenChange={(value) => !value && setDelivering(null)}>
        {delivering && <DeliveryDialog order={delivering} onDelivered={async (receivedByName) => { const order = delivering; setDelivering(null); await moveTo(order, "delivered", { receivedByName }); }} />}
      </Dialog>
      <Dialog open={batchPrinting} onOpenChange={(value) => !value && setBatchPrinting(false)}>
        {batchPrinting && <BatchRequisition orderIds={selected} />}
      </Dialog>
    </div>
  );
}

/**
 * The checklist a person actually completes.
 *
 * The board used to tick every box on the user's behalf and post it with the
 * note "QC checklist completed from lab board" as it advanced the order. That
 * wrote a record of an inspection nobody had carried out.
 */
function QualityControlDialog({ order, onPassed }: { order: LabOrder; onPassed(): void | Promise<void> }) {
  const [checks, setChecks] = React.useState(emptyChecklist);
  const [notes, setNotes] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const required = (Object.keys(emptyChecklist) as (keyof typeof emptyChecklist)[]).filter((key) => key !== "axisVerified");
  const complete = required.every((key) => checks[key]);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      await api.post(`/lab-orders/${order.id}/quality-control`, { ...checks, notes });
      await onPassed();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Quality control could not be saved"); }
    finally { setSaving(false); }
  };
  return <DialogContent>
    <DialogHeader>
      <DialogTitle>Quality control · {order.orderNumber}</DialogTitle>
      <DialogDescription>Check the glasses against the order before they are put aside for {order.patientName}.</DialogDescription>
    </DialogHeader>
    <form className="grid gap-3" onSubmit={submit}>
      {(Object.keys(emptyChecklist) as (keyof typeof emptyChecklist)[]).map((key) => (
        <label key={key} className="flex items-start gap-3 rounded-md border p-3 text-sm">
          <input className="mt-0.5" type="checkbox" checked={checks[key]} onChange={(event) => setChecks({ ...checks, [key]: event.target.checked })} />
          <span>{checklistLabels[key]}{key === "axisVerified" && <span className="text-zinc-500"> — optional</span>}</span>
        </label>
      ))}
      <Field label="Notes"><Textarea value={notes} onChange={(event) => setNotes(event.target.value)} /></Field>
      <DialogFooter>
        <Button type="submit" disabled={saving || !complete}><CheckCircle2 className="h-4 w-4" />{saving ? "Saving…" : "Pass and mark ready"}</Button>
      </DialogFooter>
    </form>
  </DialogContent>;
}

/** Who collected the glasses is part of the record, not a default. */
function DeliveryDialog({ order, onDelivered }: { order: LabOrder; onDelivered(receivedByName: string): void | Promise<void> }) {
  const [receivedBy, setReceivedBy] = React.useState(order.patientName);
  const [saving, setSaving] = React.useState(false);
  return <DialogContent>
    <DialogHeader>
      <DialogTitle>Hand over {order.orderNumber}</DialogTitle>
      <DialogDescription>Record who collected the work, which may be a relative or a courier rather than the patient.</DialogDescription>
    </DialogHeader>
    <form className="grid gap-4" onSubmit={async (event) => { event.preventDefault(); setSaving(true); await onDelivered(receivedBy.trim()); setSaving(false); }}>
      <Field label="Collected by"><Input required value={receivedBy} onChange={(event) => setReceivedBy(event.target.value)} /></Field>
      <DialogFooter><Button type="submit" disabled={saving || !receivedBy.trim()}>{saving ? "Recording…" : "Record handover"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

interface PrescriptionOption { id: string; prescriptionNumber: string; type: string; issuedAt: string; od: Record<string, string>; os: Record<string, string>; details: Record<string, string>; encounterId: string }
interface InvoiceOption { id: string; invoiceNumber: string; currency: string; totalMinor: number; createdAt: string }
interface InventoryOption { id: string; name: string; brand: string; sku: string; salePriceMinor: number; currency: string }
interface SupplierOption { id: string; company: string }

/** The clinic's own list, as a dropdown. */
function CatalogSelect({ catalog, label, value, onChange, placeholder }: { catalog: CatalogName; label: string; value: string; onChange(value: string): void; placeholder: string }) {
  const entries = useCatalog(catalog);
  return <Field label={label}>
    <Select value={value} onChange={(event) => onChange(event.target.value)}>
      <option value="">{placeholder}</option>
      {entries.data?.items.map((entry) => <option key={entry.id} value={entry.label}>{entry.label}</option>)}
    </Select>
  </Field>;
}

/** Several entries from one list — coatings and treatments are rarely just one. */
function CatalogChoices({ catalog, label, values, onChange }: { catalog: CatalogName; label: string; values: string[]; onChange(values: string[]): void }) {
  const entries = useCatalog(catalog);
  const toggle = (entry: string) => onChange(values.includes(entry) ? values.filter((candidate) => candidate !== entry) : [...values, entry]);
  return <FieldGroup label={label}>
    <div className="flex flex-wrap gap-2">
      {entries.data?.items.length ? entries.data.items.map((entry) => (
        <button key={entry.id} type="button" aria-pressed={values.includes(entry.label)} onClick={() => toggle(entry.label)}
          className={`min-h-9 rounded-full border px-3 text-xs font-semibold ${values.includes(entry.label) ? "border-black bg-black text-white" : "border-[var(--input)]"}`}>
          {entry.label}
        </button>
      )) : <p className="text-xs text-zinc-500">Nothing in this list yet — add entries under System → Catalogs.</p>}
    </div>
  </FieldGroup>;
}

/**
 * An optical order starts from the prescription, because that is where the work
 * starts: the powers decide the lens, the consultation explains the powers, and
 * the sale says what the patient already paid for. Choosing the prescription
 * fills all three in, so the sheet the workshop receives is complete.
 */
function OpticalOrderForm({ onSaved }: { onSaved(): void }) {
  const [patientId, setPatientId] = React.useState("");
  const [form, setForm] = React.useState({
    prescriptionId: "", encounterId: "", invoiceId: "", supplierId: "", frameItemId: "", lensItemId: "",
    lensType: "", material: "", tint: "", coatings: [] as string[], treatments: [] as string[],
    measurements: { pd: "", fittingHeight: "", vertexDistance: "", pantoscopicTilt: "", wrapAngle: "" },
    notes: "", expectedAt: "", costMinor: 0, salePriceMinor: 0,
  });
  const [saving, setSaving] = React.useState(false);

  const prescriptions = useLoad(() => patientId ? api.get<{ items: PrescriptionOption[] }>(`/prescriptions?patientId=${patientId}&limit=20`) : Promise.resolve({ items: [] }), [patientId]);
  const invoices = useLoad(() => patientId ? api.get<{ items: InvoiceOption[] }>(`/invoices?patientId=${patientId}&limit=20`) : Promise.resolve({ items: [] }), [patientId]);
  const frames = useLoad(() => api.get<{ items: InventoryOption[] }>("/inventory?category=frame&limit=200"), []);
  const lenses = useLoad(() => api.get<{ items: InventoryOption[] }>("/inventory?category=ophthalmic_lens&limit=200"), []);
  const suppliers = useLoad(() => api.get<{ items: SupplierOption[] }>("/suppliers?limit=200"), []);
  const chosen = prescriptions.data?.items.find((item) => item.id === form.prescriptionId);

  const choosePrescription = (id: string) => {
    const prescription = prescriptions.data?.items.find((item) => item.id === id);
    setForm((current) => ({
      ...current, prescriptionId: id, encounterId: prescription?.encounterId ?? "",
      // The pupillary distance was measured in the consultation; asking for it a
      // second time is how the two copies end up disagreeing.
      measurements: { ...current.measurements, pd: prescription?.details?.pd || current.measurements.pd },
    }));
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      await api.post("/lab-orders", { ...form, kind: "optical", patientId, expectedAt: form.expectedAt ? new Date(form.expectedAt).toISOString() : "" });
      toast.success("Optical order created");
      onSaved();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not create the order"); }
    finally { setSaving(false); }
  };

  return <DialogContent className="max-h-[92vh] max-w-2xl overflow-y-auto">
    <DialogHeader>
      <DialogTitle>New optical order</DialogTitle>
      <DialogDescription>For the company that glazes and mounts the glasses. Start from the prescription and the powers, the consultation and the sale travel with the order.</DialogDescription>
    </DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <FieldGroup label="Patient"><PatientPicker required value={patientId} onChange={(value) => { setPatientId(value); setForm((current) => ({ ...current, prescriptionId: "", encounterId: "", invoiceId: "" })); }} /></FieldGroup>

      <Field label="Prescription" hint={patientId ? undefined : "Choose the patient first."}>
        <Select value={form.prescriptionId} onChange={(event) => choosePrescription(event.target.value)} disabled={!patientId}>
          <option value="">No prescription — repair or remake</option>
          {prescriptions.data?.items.filter((item) => item.type === "spectacle").map((item) => (
            <option key={item.id} value={item.id}>{item.prescriptionNumber} · {dateTime(item.issuedAt)}</option>
          ))}
        </Select>
      </Field>
      {chosen && (
        <div className="rounded-md border p-3 text-sm">
          <div className="text-xs font-bold uppercase tracking-wide text-zinc-500">Powers going to the workshop</div>
          <div className="mt-2 grid gap-1 font-mono text-xs sm:grid-cols-2">
            <div>OD {["sphere", "cylinder", "axis", "add"].map((field) => chosen.od?.[field] ?? chosen.od?.[`od${field}`] ?? "—").join(" / ")}</div>
            <div>OS {["sphere", "cylinder", "axis", "add"].map((field) => chosen.os?.[field] ?? chosen.os?.[`os${field}`] ?? "—").join(" / ")}</div>
          </div>
          {chosen.encounterId && <p className="mt-2 text-xs text-zinc-500">The consultation this came from is attached to the order.</p>}
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Glazing company">
          <Select value={form.supplierId} onChange={(event) => setForm({ ...form, supplierId: event.target.value })}>
            <option value="">Not assigned</option>
            {suppliers.data?.items.map((item) => <option key={item.id} value={item.id}>{item.company}</option>)}
          </Select>
        </Field>
        <Field label="Sale at the till" hint={patientId ? undefined : "Choose the patient first."}>
          <Select value={form.invoiceId} onChange={(event) => setForm({ ...form, invoiceId: event.target.value })} disabled={!patientId}>
            <option value="">Not linked</option>
            {invoices.data?.items.map((item) => <option key={item.id} value={item.id}>{item.invoiceNumber} · {money(item.totalMinor, item.currency)}</option>)}
          </Select>
        </Field>
        <Field label="Frame">
          <Select value={form.frameItemId} onChange={(event) => setForm({ ...form, frameItemId: event.target.value })}>
            <option value="">Patient's own frame</option>
            {frames.data?.items.map((item) => <option key={item.id} value={item.id}>{item.name}{item.brand ? ` · ${item.brand}` : ""}</option>)}
          </Select>
        </Field>
        <Field label="Lens from stock">
          <Select value={form.lensItemId} onChange={(event) => setForm({ ...form, lensItemId: event.target.value })}>
            <option value="">Ordered from the lab</option>
            {lenses.data?.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </Select>
        </Field>
        <CatalogSelect catalog="lens_type" label="Lens type" value={form.lensType} onChange={(lensType) => setForm({ ...form, lensType })} placeholder="Not specified" />
        <CatalogSelect catalog="lens_material" label="Material / index" value={form.material} onChange={(material) => setForm({ ...form, material })} placeholder="Not specified" />
        <CatalogSelect catalog="lens_tint" label="Tint" value={form.tint} onChange={(tint) => setForm({ ...form, tint })} placeholder="Clear" />
        <FieldGroup label="Expected date">
          {/* The field stores a local "YYYY-MM-DDTHH:MM" and converts to ISO on
              submit. It previously held an ISO string, which no date/time input
              accepts as a value, so the chosen date never redisplayed. */}
          <DateTimeInput label="Expected" value={form.expectedAt} onChange={(expectedAt) => setForm({ ...form, expectedAt })} />
        </FieldGroup>
      </div>

      <CatalogChoices catalog="lens_coating" label="Coatings" values={form.coatings} onChange={(coatings) => setForm({ ...form, coatings })} />
      <CatalogChoices catalog="lens_treatment" label="Treatments" values={form.treatments} onChange={(treatments) => setForm({ ...form, treatments })} />

      <FieldGroup label="Fitting measurements">
        <div className="grid gap-3 sm:grid-cols-3">
          {Object.entries(form.measurements).map(([key, value]) => (
            <Field key={key} label={key.replace(/([A-Z])/g, " $1")}>
              <Input value={value} onChange={(event) => setForm({ ...form, measurements: { ...form.measurements, [key]: event.target.value } })} />
            </Field>
          ))}
        </div>
      </FieldGroup>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Lab cost (minor units)"><Input type="number" min={0} value={form.costMinor} onChange={(event) => setForm({ ...form, costMinor: Number(event.target.value) || 0 })} /></Field>
        <Field label="Price to the patient (minor units)"><Input type="number" min={0} value={form.salePriceMinor} onChange={(event) => setForm({ ...form, salePriceMinor: Number(event.target.value) || 0 })} /></Field>
      </div>
      <Field label="Workshop notes"><Textarea value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} /></Field>
      <DialogFooter>
        <Button type="submit" disabled={saving || !patientId}><Glasses className="h-4 w-4" />{saving ? "Creating…" : "Create order"}</Button>
      </DialogFooter>
    </form>
  </DialogContent>;
}

/** Examinations sent to an outside laboratory; the results come back as a document on the patient's record. */
function ExamRequestForm({ onSaved }: { onSaved(): void }) {
  const [patientId, setPatientId] = React.useState("");
  const [supplierId, setSupplierId] = React.useState("");
  const [encounterId, setEncounterId] = React.useState("");
  const [expectedAt, setExpectedAt] = React.useState("");
  const [notes, setNotes] = React.useState("");
  const [tests, setTests] = React.useState<LabOrderTest[]>([]);
  const [saving, setSaving] = React.useState(false);
  const catalogue = useCatalog("lab_test");
  const suppliers = useLoad(() => api.get<{ items: SupplierOption[] }>("/suppliers?limit=200"), []);
  const consultations = useLoad(() => patientId ? api.get<{ items: { id: string; encounterNumber: string; createdAt: string }[] }>(`/encounters?patientId=${patientId}&limit=20`) : Promise.resolve({ items: [] }), [patientId]);

  const add = (label: string) => { if (label && !tests.some((test) => test.label === label)) setTests([...tests, { label, code: "", specimen: "", notes: "" }]); };
  const update = (index: number, patch: Partial<LabOrderTest>) => setTests(tests.map((test, position) => position === index ? { ...test, ...patch } : test));

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      await api.post("/lab-orders", { kind: "medical", patientId, encounterId, supplierId, notes, tests, expectedAt: expectedAt ? new Date(expectedAt).toISOString() : "" });
      toast.success("Exam request created");
      onSaved();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not create the request"); }
    finally { setSaving(false); }
  };

  return <DialogContent className="max-h-[92vh] max-w-2xl overflow-y-auto">
    <DialogHeader>
      <DialogTitle>New laboratory exam request</DialogTitle>
      <DialogDescription>Choose the examinations from the clinic's list and print the request for the laboratory. Results are filed back as a document on the patient's record.</DialogDescription>
    </DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <FieldGroup label="Patient"><PatientPicker required value={patientId} onChange={(value) => { setPatientId(value); setEncounterId(""); }} /></FieldGroup>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Consultation" hint={patientId ? undefined : "Choose the patient first."}>
          <Select value={encounterId} onChange={(event) => setEncounterId(event.target.value)} disabled={!patientId}>
            <option value="">Not linked</option>
            {consultations.data?.items.map((item) => <option key={item.id} value={item.id}>{item.encounterNumber} · {dateTime(item.createdAt)}</option>)}
          </Select>
        </Field>
        <Field label="Laboratory">
          <Select value={supplierId} onChange={(event) => setSupplierId(event.target.value)}>
            <option value="">Not assigned</option>
            {suppliers.data?.items.map((item) => <option key={item.id} value={item.id}>{item.company}</option>)}
          </Select>
        </Field>
      </div>

      <FieldGroup label="Examinations">
        {catalogue.data?.items.length ? (
          <div className="flex flex-wrap gap-2">
            {catalogue.data.items.map((entry) => (
              <button key={entry.id} type="button" onClick={() => add(entry.label)} className="min-h-9 rounded-full border border-[var(--input)] px-3 text-xs font-semibold">
                <Plus className="mr-1 inline h-3 w-3" />{entry.label}
              </button>
            ))}
          </div>
        ) : <p className="text-xs text-zinc-500">The exam list is empty. Add the examinations this clinic orders under System → Catalogs, so every request names them the same way.</p>}
        {tests.length > 0 && (
          <div className="mt-3 grid gap-2">
            {tests.map((test, index) => (
              <div key={`${test.label}-${index}`} className="grid gap-2 rounded-md border p-3 sm:grid-cols-[1fr_auto]">
                <div className="font-semibold">{test.label}</div>
                <button type="button" aria-label={`Remove ${test.label}`} className="justify-self-end text-zinc-400 hover:text-red-700" onClick={() => setTests(tests.filter((_, position) => position !== index))}><Trash2 className="h-4 w-4" /></button>
                <div className="grid gap-2 sm:col-span-2 sm:grid-cols-3">
                  <Field label="Code"><Input value={test.code} onChange={(event) => update(index, { code: event.target.value })} /></Field>
                  <Field label="Specimen"><Input value={test.specimen} onChange={(event) => update(index, { specimen: event.target.value })} /></Field>
                  <Field label="Instructions"><Input value={test.notes} onChange={(event) => update(index, { notes: event.target.value })} /></Field>
                </div>
              </div>
            ))}
          </div>
        )}
      </FieldGroup>

      <FieldGroup label="Expected date"><DateTimeInput label="Expected" value={expectedAt} onChange={setExpectedAt} /></FieldGroup>
      <Field label="Clinical notes for the laboratory"><Textarea value={notes} onChange={(event) => setNotes(event.target.value)} /></Field>
      <DialogFooter>
        <Button type="submit" disabled={saving || !patientId || tests.length === 0}><FlaskConical className="h-4 w-4" />{saving ? "Creating…" : "Create request"}</Button>
      </DialogFooter>
    </form>
  </DialogContent>;
}

interface RequisitionOrder {
  id: string; kind: LabOrderKind; orderNumber: string; medicalRecordNumber: string; patientName: string; patientPhone: string;
  supplierName: string; frameName: string; lensName: string; lensType: string; material: string; coatings: string[]; tint: string;
  treatments: string[]; measurements: Record<string, string>; notes: string; expectedAt: string; status: string;
  prescriptionNumber: string; od: Record<string, string>; os: Record<string, string>; prescriptionDetails: Record<string, string>;
  tests: LabOrderTest[];
}
interface Requisition { clinic: { name: string; address: string; phone: string }; generatedAt: string; orders: RequisitionOrder[] }

const power = (values: Record<string, string> | undefined, eye: "od" | "os") =>
  ["sphere", "cylinder", "axis", "add"].map((field) => values?.[field] ?? values?.[`${eye}${field}`] ?? values?.[`${eye}${field}`.toLowerCase()] ?? "—").join(" / ");

/**
 * One printed requisition covering a whole batch — what a courier carries out —
 * instead of a stack printed one screen at a time. It carries the powers for
 * each order, because a batch that arrives without them is a batch the company
 * telephones back about, one order at a time.
 */
function BatchRequisition({ orderIds }: { orderIds: string[] }) {
  const requisition = useLoad(() => api.get<Requisition>(`/lab-orders/requisition?orderIds=${orderIds.join(",")}`), [orderIds.join(",")]);
  return <DialogContent className="max-h-[92vh] max-w-4xl overflow-y-auto">
    <DialogHeader className="no-print">
      <DialogTitle>Requisition · {orderIds.length} order(s)</DialogTitle>
      <DialogDescription>One sheet for the whole batch, grouped by the firm it goes to. Print it on standard paper and send it with the work.</DialogDescription>
    </DialogHeader>
    {requisition.loading ? <Skeleton className="h-72" /> : requisition.error || !requisition.data ? <ErrorState message={requisition.error?.message ?? "Requisition unavailable"} retry={requisition.reload} /> : <>
      <article className="print-area document-print space-y-5 bg-white p-3 text-black sm:p-8">
        <PrintHeader documentTitle="Laboratory requisition" number={`${requisition.data.orders.length} order(s)`} date={dateTime(requisition.data.generatedAt)} />
        <table className="w-full text-left text-xs">
          <thead><tr className="border-b border-black">{["Order", "Patient", "Sent to", "Work", "Expected"].map((heading) => <th key={heading} className="py-2 pr-3 font-bold uppercase tracking-wide">{heading}</th>)}</tr></thead>
          <tbody>{requisition.data.orders.map((order) => <tr key={order.id} className="border-b border-zinc-300 align-top">
            <td className="py-2 pr-3 font-mono font-bold">{order.orderNumber}</td>
            <td className="py-2 pr-3">{order.patientName}<div className="font-mono text-[10px] text-zinc-500">{order.medicalRecordNumber}</div></td>
            <td className="py-2 pr-3">{order.supplierName || "—"}</td>
            <td className="py-2 pr-3">
              {order.kind === "medical" ? (
                <span>{order.tests?.map((test) => test.label).join(", ") || "—"}</span>
              ) : <>
                <div>{order.frameName || "Patient's own frame"} · {order.lensName || order.lensType || "—"}{order.material ? ` · ${order.material}` : ""}</div>
                {(order.tint || order.coatings?.length || order.treatments?.length) && <div className="text-[10px] text-zinc-600">{[order.tint, ...(order.coatings ?? []), ...(order.treatments ?? [])].filter(Boolean).join(" · ")}</div>}
                <div className="mt-1 font-mono text-[10px]">OD {power(order.od, "od")} · OS {power(order.os, "os")}{order.prescriptionDetails?.pd ? ` · PD ${order.prescriptionDetails.pd}` : ""}</div>
              </>}
            </td>
            <td className="py-2 pr-3">{order.expectedAt ? dateTime(order.expectedAt) : "—"}</td>
          </tr>)}</tbody>
        </table>
        <div className="mt-10 flex justify-between gap-8 text-xs">
          <div className="w-64 border-t border-black pt-2">Released by (clinic)</div>
          <div className="w-64 border-t border-black pt-2">Received by (laboratory)</div>
        </div>
      </article>
      <div className="no-print flex justify-end"><Button onClick={triggerPrint}><Printer className="h-4 w-4" />Print requisition</Button></div>
    </>}
  </DialogContent>;
}
