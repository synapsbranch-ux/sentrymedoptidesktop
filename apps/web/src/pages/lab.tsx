import * as React from "react";
import {
  ArrowRight,
  CheckCircle2,
  FlaskConical,
  Plus,
  Printer,
  Send,
} from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { PrintHeader, triggerPrint } from "../components/print";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "../components/ui/dialog";
import { Badge, EmptyState, ErrorState, Pager, Skeleton } from "../components/ui/data";
import { Field, FieldGroup, Input, Select, Textarea } from "../components/ui/input";
import { DateTimeInput } from "../components/ui/date-time";
import { useLoad, usePagedList } from "../hooks";
import { dateTime } from "../lib";
import { useRealtime } from "../realtime";
import type { LabOrder } from "../types";
import { PatientPicker } from "../components/patient-search";
import { ImageGallery } from "../components/image-gallery";

const stages = [
  "draft",
  "ordered",
  "at_lab",
  "received",
  "edging_mounting",
  "quality_control",
  "ready",
  "delivered",
];
export function LabPage() {
  const { revision } = useRealtime();
  const [open, setOpen] = React.useState(false);
  const [printing, setPrinting] = React.useState<LabOrder | null>(null);
  // A clinic sends a day's work to the lab together, so orders are selected on
  // the board and moved or printed as one batch.
  const [selected, setSelected] = React.useState<string[]>([]);
  const [batchPrinting, setBatchPrinting] = React.useState(false);
  const [sending, setSending] = React.useState(false);
  const toggle = (id: string) => setSelected((current) => current.includes(id) ? current.filter((candidate) => candidate !== id) : [...current, id]);
  const sendBatch = async (status: string) => {
    setSending(true);
    try {
      const result = await api.post<{ updated: number }>("/lab-orders/bulk-status", { orderIds: selected, status });
      toast.success(`${result.updated} lab order(s) marked ${status.replaceAll("_", " ")}`);
      setSelected([]);
      orders.reload();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "The batch was rolled back; nothing changed."); }
    finally { setSending(false); }
  };
  const orders = usePagedList<LabOrder>((page, limit) => `/lab-orders?page=${page}&limit=${limit}`, [revision]);
  const move = async (order: LabOrder) => {
    const next =
      stages[Math.min(stages.indexOf(order.status) + 1, stages.length - 1)];
    if (next === "ready") {
      try {
        await api.post(`/lab-orders/${order.id}/quality-control`, {
          prescriptionVerified: true,
          powerVerified: true,
          axisVerified: true,
          frameCondition: true,
          lensCondition: true,
          fittingVerified: true,
          finalCleaning: true,
          notes: "QC checklist completed from lab board",
        });
      } catch (reason) {
        toast.error(reason instanceof Error ? reason.message : "QC failed");
        return;
      }
    }
    try {
      await api.patch(`/lab-orders/${order.id}/status`, {
        status: next,
        notes: "",
        receivedByName: next === "delivered" ? "Patient" : "",
        version: order.version,
      });
      toast.success(`Order moved to ${next.replaceAll("_", " ")}`);
      orders.reload();
    } catch (reason) {
      toast.error(
        reason instanceof Error ? reason.message : "Status update failed",
      );
    }
  };
  return (
    <div className="page">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="section-title">Optical workshop</p>
          <h1 className="page-title">Lab orders</h1>
          <p className="page-description">
            From prescription and fitting measurements through QC and delivery.
          </p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4" />
              New lab order
            </Button>
          </DialogTrigger>
          <LabOrderForm
            onSaved={() => {
              setOpen(false);
              orders.reload();
            }}
          />
        </Dialog>
      </div>
      {selected.length > 0 && (
        <div className="mt-4 flex flex-wrap items-center gap-3 rounded-lg border border-black bg-[var(--muted)] p-3">
          <span className="text-sm font-semibold">{selected.length} order(s) selected</span>
          <div className="ml-auto flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => setBatchPrinting(true)}><Printer className="h-3.5 w-3.5" />Print requisition</Button>
            <Button size="sm" disabled={sending} onClick={() => sendBatch("at_lab")}><Send className="h-3.5 w-3.5" />Send to lab</Button>
            <Button size="sm" variant="ghost" onClick={() => setSelected([])}>Clear</Button>
          </div>
        </div>
      )}
      {orders.loading ? (
        <Skeleton className="mt-6 h-96" />
      ) : orders.error ? (
        <div className="mt-6">
          <ErrorState message={orders.error.message} retry={orders.reload} />
        </div>
      ) : orders.items.length ? (
        <>
        <div className="mt-6 flex gap-4 overflow-x-auto pb-4">
          {stages.map((stage) => (
            <section key={stage} className="w-72 shrink-0">
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-xs font-bold uppercase tracking-wide">
                  {stage.replaceAll("_", " ")}
                </h2>
                <Badge>
                  {orders.items.filter((item) => item.status === stage)
                    .length ?? 0}
                </Badge>
              </div>
              <div className="space-y-3">
                {orders.items
                  .filter((item) => item.status === stage)
                  .map((order) => (
                    <Card key={order.id}>
                      <CardContent className="p-4">
                        <div className="flex items-center justify-between">
                          <label className="flex items-center gap-2">
                            <input type="checkbox" aria-label={`Select ${order.orderNumber}`} checked={selected.includes(order.id)} onChange={() => toggle(order.id)} />
                            <span className="font-mono text-[11px] font-bold">{order.orderNumber}</span>
                          </label>
                          {order.expectedAt &&
                            new Date(order.expectedAt) < new Date() &&
                            !["ready", "delivered"].includes(order.status) && (
                              <Badge tone="danger">Delayed</Badge>
                            )}
                        </div>
                        <div className="mt-3 font-semibold">
                          {order.patientName}
                        </div>
                        <div className="mt-1 text-xs text-zinc-500">
                          {order.frameName || "Frame pending"} ·{" "}
                          {order.lensType || "Lens pending"}
                        </div>
                        <div className="mt-3 text-[11px] text-zinc-500">
                          Expected{" "}
                          {order.expectedAt
                            ? dateTime(order.expectedAt)
                            : "not set"}
                        </div>
                        <div className="mt-4 flex gap-2">
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setPrinting(order)}
                            aria-label={`Print ${order.orderNumber}`}
                          >
                            <Printer className="h-3 w-3" />
                          </Button>
                          {stage !== "delivered" && (
                            <Button
                              className="flex-1"
                              size="sm"
                              variant="outline"
                              onClick={() => move(order)}
                            >
                              {stage === "quality_control" ? (
                                <CheckCircle2 className="h-3 w-3" />
                              ) : (
                                <ArrowRight className="h-3 w-3" />
                              )}
                              Move to{" "}
                              {stages[
                                Math.min(
                                  stages.indexOf(stage) + 1,
                                  stages.length - 1,
                                )
                              ].replaceAll("_", " ")}
                            </Button>
                          )}
                        </div>
                      </CardContent>
                    </Card>
                  ))}
              </div>
            </section>
          ))}
        </div>
        {/* The board shows one page of orders; the rest stay on the server. */}
        <Pager page={orders.page} pageSize={orders.pageSize} total={orders.total} hasMore={orders.hasMore} onPrevious={orders.previous} onNext={orders.next} />
        </>
      ) : (
        <div className="mt-6">
          <EmptyState
            title="No optical lab orders"
            description="Create an order from a patient's prescription or sale."
            action={
              <Button onClick={() => setOpen(true)}>Create lab order</Button>
            }
          />
        </div>
      )}
      <Dialog
        open={Boolean(printing)}
        onOpenChange={(value) => !value && setPrinting(null)}
      >
        {printing && <LabOrderPrint order={printing} />}
      </Dialog>
      <Dialog open={batchPrinting} onOpenChange={(value) => !value && setBatchPrinting(false)}>
        {batchPrinting && <BatchRequisition orderIds={selected} />}
      </Dialog>
    </div>
  );
}
function LabOrderPrint({ order }: { order: LabOrder }) {
  const measurements = Object.entries(order.measurements ?? {}).filter(
    ([, value]) => value !== "" && value != null,
  );
  return (
    <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto">
      <DialogHeader className="no-print">
        <DialogTitle>Lab order {order.orderNumber}</DialogTitle>
        <DialogDescription>
          Print or save this workshop document as PDF.
        </DialogDescription>
      </DialogHeader>
      {/* Reference photographs — the frame the patient brought in, a note the
          workshop needs to see — live with the order rather than beside it. */}
      <div className="no-print rounded-lg border p-4">
        <h3 className="mb-3 text-xs font-bold uppercase tracking-wider">Reference images</h3>
        <ImageGallery entityType="lab_order" entityId={order.id} canEdit emptyHint="No reference images on this order yet." />
      </div>
      <article className="print-area document-print space-y-6 bg-white p-3 text-black sm:p-8">
        <PrintHeader
          documentTitle="Optical laboratory order"
          number={order.orderNumber}
          date={order.orderedAt ? dateTime(order.orderedAt) : ""}
        />
        <div className="grid grid-cols-2 gap-4 border-y border-zinc-300 py-4 text-sm">
          <PrintField label="Patient" value={order.patientName} />
          <PrintField
            label="Status"
            value={order.status.replaceAll("_", " ")}
          />
          <PrintField
            label="Ordered"
            value={order.orderedAt ? dateTime(order.orderedAt) : "—"}
          />
          <PrintField
            label="Expected"
            value={order.expectedAt ? dateTime(order.expectedAt) : "—"}
          />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <PrintField label="Frame" value={order.frameName} />
          <PrintField label="Lens" value={order.lensName || order.lensType} />
          <PrintField label="Material" value={order.material} />
          <PrintField label="Supplier / lab" value={order.supplierName} />
          <PrintField label="Coatings" value={order.coatings?.join(", ")} />
          <PrintField label="Treatments" value={order.treatments?.join(", ")} />
        </div>
        <section>
          <h3 className="mb-2 text-xs font-bold uppercase tracking-wider">
            Fitting measurements
          </h3>
          {measurements.length ? (
            <div className="grid grid-cols-2 gap-px border border-zinc-300 bg-zinc-300 sm:grid-cols-3">
              {measurements.map(([key, value]) => (
                <div className="bg-white p-3" key={key}>
                  <div className="text-[10px] font-bold uppercase text-zinc-500">
                    {key.replace(/([A-Z])/g, " $1")}
                  </div>
                  <div className="font-mono text-sm">{String(value)}</div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-zinc-500">
              No fitting measurements recorded.
            </p>
          )}
        </section>
        <section>
          <h3 className="text-xs font-bold uppercase tracking-wider">
            Workshop notes
          </h3>
          <p className="mt-2 min-h-20 whitespace-pre-wrap border border-zinc-300 p-3 text-sm">
            {order.notes || "—"}
          </p>
        </section>
        <div className="grid grid-cols-2 gap-8 pt-12 text-xs">
          <div className="border-t border-black pt-2">Prepared by</div>
          <div className="border-t border-black pt-2">Quality control</div>
        </div>
      </article>
      <DialogFooter className="no-print">
        <Button onClick={triggerPrint}>
          <Printer className="h-4 w-4" />
          Print / Save PDF
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
function PrintField({ label, value }: { label: string; value?: string }) {
  return (
    <div>
      <div className="text-[10px] font-bold uppercase text-zinc-500">
        {label}
      </div>
      <div className="mt-1 text-sm">{value || "—"}</div>
    </div>
  );
}

function LabOrderForm({ onSaved }: { onSaved(): void }) {
  const [form, setForm] = React.useState({
    patientId: "",
    prescriptionId: "",
    invoiceId: "",
    supplierId: "",
    frameItemId: "",
    lensItemId: "",
    lensType: "single_vision",
    material: "",
    coatings: [] as string[],
    tint: "",
    treatments: [] as string[],
    measurements: {
      pd: "",
      fittingHeight: "",
      vertexDistance: "",
      pantoscopicTilt: "",
      wrapAngle: "",
    },
    notes: "",
    expectedAt: "",
    costMinor: 0,
    salePriceMinor: 0,
  });
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      await api.post("/lab-orders", { ...form, expectedAt: form.expectedAt ? new Date(form.expectedAt).toISOString() : "" });
      toast.success("Lab order created");
      onSaved();
    } catch (reason) {
      toast.error(
        reason instanceof Error ? reason.message : "Could not create lab order",
      );
    } finally {
      setSaving(false);
    }
  };
  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Create optical lab order</DialogTitle>
        <DialogDescription>
          Advanced fitting measurements are optional.
        </DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={submit}>
        <FieldGroup label="Patient">
          <PatientPicker
            required
            value={form.patientId}
            onChange={(patientId) => setForm({ ...form, patientId })}
          />
        </FieldGroup>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Lens type">
            <Select
              value={form.lensType}
              onChange={(event) =>
                setForm({ ...form, lensType: event.target.value })
              }
            >
              <option value="single_vision">Single vision</option>
              <option value="progressive">Progressive</option>
              <option value="bifocal">Bifocal</option>
            </Select>
          </Field>
          <Field label="Material">
            <Input
              value={form.material}
              onChange={(event) =>
                setForm({ ...form, material: event.target.value })
              }
            />
          </Field>
          <FieldGroup label="Expected date">
            {/* The field stores a local "YYYY-MM-DDTHH:MM" and converts to ISO on
                submit. It previously held an ISO string, which no date/time input
                accepts as a value, so the chosen date never redisplayed. */}
            <DateTimeInput
              label="Expected"
              value={form.expectedAt}
              onChange={(expectedAt) => setForm({ ...form, expectedAt })}
            />
          </FieldGroup>
          {Object.entries(form.measurements).map(([key, value]) => (
            <Field key={key} label={key.replace(/([A-Z])/g, " $1")}>
              <Input
                value={value}
                onChange={(event) =>
                  setForm({
                    ...form,
                    measurements: {
                      ...form.measurements,
                      [key]: event.target.value,
                    },
                  })
                }
              />
            </Field>
          ))}
        </div>
        <Field label="Workshop notes">
          <Textarea
            value={form.notes}
            onChange={(event) =>
              setForm({ ...form, notes: event.target.value })
            }
          />
        </Field>
        <DialogFooter>
          <Button type="submit" disabled={saving}>
            <FlaskConical className="h-4 w-4" />
            {saving ? "Creating…" : "Create order"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

interface RequisitionOrder { id: string; orderNumber: string; medicalRecordNumber: string; patientName: string; patientPhone: string; supplierName: string; frameName: string; lensName: string; lensType: string; material: string; coatings: string[]; tint: string; treatments: string[]; measurements: Record<string, string>; notes: string; expectedAt: string; status: string }
interface Requisition { clinic: { name: string; address: string; phone: string }; generatedAt: string; orders: RequisitionOrder[] }

/**
 * One printed requisition covering a whole batch — what a courier carries to the
 * lab — instead of a stack printed one screen at a time. It is a standard-paper
 * document, not a till receipt, so it uses the A4/Letter print path.
 */
function BatchRequisition({ orderIds }: { orderIds: string[] }) {
  const requisition = useLoad(() => api.get<Requisition>(`/lab-orders/requisition?orderIds=${orderIds.join(",")}`), [orderIds.join(",")]);
  return <DialogContent className="max-h-[92vh] max-w-4xl overflow-y-auto">
    <DialogHeader className="no-print">
      <DialogTitle>Lab requisition · {orderIds.length} order(s)</DialogTitle>
      <DialogDescription>One sheet for the whole batch, grouped by lab. Print it on standard paper and send it with the work.</DialogDescription>
    </DialogHeader>
    {requisition.loading ? <Skeleton className="h-72" /> : requisition.error || !requisition.data ? <ErrorState message={requisition.error?.message ?? "Requisition unavailable"} retry={requisition.reload} /> : <>
      <article className="print-area document-print space-y-5 bg-white p-3 text-black sm:p-8">
        <PrintHeader documentTitle="Optical laboratory requisition" number={`${requisition.data.orders.length} order(s)`} date={dateTime(requisition.data.generatedAt)} />
        <table className="w-full text-left text-xs">
          <thead><tr className="border-b border-black">{["Order", "Patient", "Lab", "Frame", "Lens", "Expected"].map((heading) => <th key={heading} className="py-2 pr-3 font-bold uppercase tracking-wide">{heading}</th>)}</tr></thead>
          <tbody>{requisition.data.orders.map((order) => <tr key={order.id} className="border-b border-zinc-300 align-top">
            <td className="py-2 pr-3 font-mono font-bold">{order.orderNumber}</td>
            <td className="py-2 pr-3">{order.patientName}<div className="font-mono text-[10px] text-zinc-500">{order.medicalRecordNumber}</div></td>
            <td className="py-2 pr-3">{order.supplierName || "—"}</td>
            <td className="py-2 pr-3">{order.frameName || "—"}</td>
            <td className="py-2 pr-3">{order.lensName || order.lensType || "—"}{order.material ? ` · ${order.material}` : ""}{order.coatings?.length ? ` · ${order.coatings.join(", ")}` : ""}</td>
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
