import * as React from "react";
import {
  ArrowRight,
  CheckCircle2,
  FlaskConical,
  Plus,
  Printer,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "../api";
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
import { Badge, EmptyState, ErrorState, Skeleton } from "../components/ui/data";
import { Field, FieldGroup, Input, Select, Textarea } from "../components/ui/input";
import { DateTimeInput } from "../components/ui/date-time";
import { useLoad } from "../hooks";
import { dateTime } from "../lib";
import { useRealtime } from "../realtime";
import type { LabOrder } from "../types";
import { PatientPicker } from "../components/patient-search";

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
  const orders = useLoad(
    () => api.get<{ items: LabOrder[] }>("/lab-orders"),
    [revision],
  );
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
      {orders.loading ? (
        <Skeleton className="mt-6 h-96" />
      ) : orders.error ? (
        <div className="mt-6">
          <ErrorState message={orders.error.message} retry={orders.reload} />
        </div>
      ) : orders.data?.items.length ? (
        <div className="mt-6 flex gap-4 overflow-x-auto pb-4">
          {stages.map((stage) => (
            <section key={stage} className="w-72 shrink-0">
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-xs font-bold uppercase tracking-wide">
                  {stage.replaceAll("_", " ")}
                </h2>
                <Badge>
                  {orders.data?.items.filter((item) => item.status === stage)
                    .length ?? 0}
                </Badge>
              </div>
              <div className="space-y-3">
                {orders.data?.items
                  .filter((item) => item.status === stage)
                  .map((order) => (
                    <Card key={order.id}>
                      <CardContent className="p-4">
                        <div className="flex items-center justify-between">
                          <span className="font-mono text-[11px] font-bold">
                            {order.orderNumber}
                          </span>
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
      <article className="print-area space-y-6 bg-white p-3 text-black sm:p-8">
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
