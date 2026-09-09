import * as React from "react";
import { toast } from "sonner";
import { FileText, Printer } from "lucide-react";
import { api, APIError } from "../api";
import { useLoad } from "../hooks";
import { dateTime, money } from "../lib";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "./ui/dialog";
import { EmptyState, Skeleton } from "./ui/data";
import { Select } from "./ui/input";
import { PrintHeader, SignatureArea, triggerPrint } from "./print";

interface SuperbillData {
  encounterId: string;
  clinic: { name: string; address: string; phone: string; nif: string };
  patient: { id: string; name: string; medicalRecordNumber: string };
  doctorName: string;
  visitReason: string;
  visitDate: string;
  diagnoses: { diagnosis: string; code: string; laterality: string; primary: boolean }[];
  invoices: {
    id: string;
    invoiceNumber: string;
    totalMinor: number;
    currency: string;
    createdAt: string;
    items: { description: string; procedureCode: string; quantity: number; unitPriceMinor: number; lineTotalMinor: number }[];
  }[];
}

export function SuperbillButton({ encounterId }: { encounterId: string }) {
  const [open, setOpen] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button type="button" variant="outline" size="sm">
          <FileText className="h-4 w-4" />
          Superbill
        </Button>
      </DialogTrigger>
      {open && <SuperbillDialog encounterId={encounterId} />}
    </Dialog>
  );
}

function InvoiceLinker({ encounterId, onLinked }: { encounterId: string; onLinked(): void }) {
  const candidates = useLoad(() => api.get<{ items: { id: string; invoiceNumber: string; totalMinor: number; currency: string }[] }>(`/encounters/${encounterId}/superbill/candidate-invoices`), [encounterId]);
  const [invoiceId, setInvoiceId] = React.useState("");
  const [linking, setLinking] = React.useState(false);
  const link = async () => {
    if (!invoiceId) return;
    setLinking(true);
    try {
      await api.post(`/encounters/${encounterId}/superbill/link-invoice`, { invoiceId });
      toast.success("Invoice linked to this consultation");
      onLinked();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Could not link invoice");
    } finally {
      setLinking(false);
    }
  };
  if (candidates.loading) return <Skeleton className="h-11" />;
  if (!candidates.data?.items.length) {
    return <EmptyState title="No billed invoice yet" description="A superbill can still be printed with just the diagnoses. Link an invoice once the patient has been billed to include billed procedures." />;
  }
  return (
    <div className="flex flex-wrap items-end gap-3 rounded-md border border-dashed p-3">
      <div className="min-w-48 flex-1">
        <label className="text-xs font-semibold text-zinc-500">Link a billed invoice (optional)</label>
        <Select value={invoiceId} onChange={(event) => setInvoiceId(event.target.value)}>
          <option value="">Select invoice…</option>
          {candidates.data.items.map((item) => (
            <option key={item.id} value={item.id}>{item.invoiceNumber} — {money(item.totalMinor, item.currency)}</option>
          ))}
        </Select>
      </div>
      <Button type="button" size="sm" disabled={!invoiceId || linking} onClick={link}>{linking ? "Linking…" : "Link invoice"}</Button>
    </div>
  );
}

function SuperbillDialog({ encounterId }: { encounterId: string }) {
  const superbill = useLoad(() => api.get<SuperbillData>(`/encounters/${encounterId}/superbill`), [encounterId]);
  if (superbill.loading) return <DialogContent><Skeleton className="h-96" /></DialogContent>;
  if (!superbill.data) return null;
  const data = superbill.data;
  const allItems = data.invoices.flatMap((invoice) => invoice.items.map((item) => ({ ...item, currency: invoice.currency })));
  const total = allItems.reduce((sum, item) => sum + item.lineTotalMinor, 0);
  const currency = data.invoices[0]?.currency ?? "HTG";
  return (
    <DialogContent className="max-w-3xl">
      <DialogHeader className="no-print">
        <div className="flex items-center justify-between pr-8">
          <DialogTitle>Superbill — {data.patient.name}</DialogTitle>
          <Button variant="outline" onClick={triggerPrint}>
            <Printer className="h-4 w-4" />
            Print / PDF
          </Button>
        </div>
      </DialogHeader>
      {data.invoices.length === 0 && (
        <div className="no-print">
          <InvoiceLinker encounterId={encounterId} onLinked={superbill.reload} />
        </div>
      )}
      <div className="print-area document-print rounded-lg border p-5">
        <PrintHeader documentTitle="Superbill" number={data.patient.medicalRecordNumber} date={dateTime(data.visitDate)} />
        <div className="mt-5 grid gap-1 text-sm">
          <div><strong>Patient:</strong> {data.patient.name} ({data.patient.medicalRecordNumber})</div>
          <div><strong>Provider:</strong> {data.doctorName || "—"}</div>
          <div><strong>Visit reason:</strong> {data.visitReason || "—"}</div>
        </div>
        <div className="mt-6">
          <div className="text-xs font-bold uppercase text-zinc-500">Diagnoses</div>
          <table className="mt-2 w-full text-sm">
            <thead><tr className="border-b text-left text-xs uppercase text-zinc-500"><th className="py-1">Code</th><th>Description</th><th>Eye</th></tr></thead>
            <tbody>
              {data.diagnoses.map((diagnosis, index) => (
                <tr key={index} className="border-b">
                  <td className="py-1.5 font-mono">{diagnosis.code || "—"}</td>
                  <td>{diagnosis.diagnosis}{diagnosis.primary && <span className="ml-2 text-xs text-zinc-400">(primary)</span>}</td>
                  <td>{diagnosis.laterality || "OU"}</td>
                </tr>
              ))}
              {data.diagnoses.length === 0 && <tr><td colSpan={3} className="py-3 text-zinc-400">No diagnoses recorded.</td></tr>}
            </tbody>
          </table>
        </div>
        <div className="mt-6">
          <div className="text-xs font-bold uppercase text-zinc-500">Billed procedures</div>
          <table className="mt-2 w-full text-sm">
            <thead><tr className="border-b text-left text-xs uppercase text-zinc-500"><th className="py-1">Code</th><th>Description</th><th>Qty</th><th className="text-right">Amount</th></tr></thead>
            <tbody>
              {allItems.map((item, index) => (
                <tr key={index} className="border-b">
                  <td className="py-1.5 font-mono">{item.procedureCode || "—"}</td>
                  <td>{item.description}</td>
                  <td>{item.quantity}</td>
                  <td className="text-right font-mono">{money(item.lineTotalMinor, item.currency)}</td>
                </tr>
              ))}
              {allItems.length === 0 && <tr><td colSpan={4} className="py-3 text-zinc-400">No billed procedures linked yet.</td></tr>}
            </tbody>
          </table>
          {allItems.length > 0 && <div className="mt-2 flex justify-end font-mono font-bold">Total: {money(total, currency)}</div>}
        </div>
        <SignatureArea doctor={data.doctorName} />
      </div>
    </DialogContent>
  );
}
