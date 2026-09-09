import * as React from "react";
import { FileCheck2, Plus, Send } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { usePagedList } from "../hooks";
import { dateTime, money } from "../lib";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "../components/ui/dialog";
import { Badge, EmptyState, ErrorState, Pager, Skeleton, Table, Td, Th } from "../components/ui/data";
import { Field, FieldGroup, Input, Select, Textarea } from "../components/ui/input";
import { PatientPicker } from "../components/patient-search";
import { InventoryPicker } from "../components/inventory-picker";
import type { InventoryItem } from "../types";

interface Quote { id: string; quoteNumber: string; patientId: string; customerName: string; status: string; currency: string; totalMinor: number; validUntil: string; expired: boolean; convertedInvoiceId: string; convertedInvoiceNumber: string; version: number; createdAt: string }
interface QuoteLine { inventoryItemId: string; description: string; quantity: number; unitPriceMinor: number; discountMinor: number }

const statusTone = (status: string, expired: boolean): "neutral" | "success" | "warning" | "danger" =>
  status === "converted" || status === "accepted" ? "success" : status === "declined" ? "danger" : expired ? "warning" : "neutral";

/**
 * A quote is what the clinic offers before anyone owes anything. It becomes an
 * invoice only when the patient accepts, and only once.
 */
export function QuotesPage() {
  const [status, setStatus] = React.useState("");
  const [creating, setCreating] = React.useState(false);
  const quotes = usePagedList<Quote>((page, limit) => `/quotes?status=${status}&page=${page}&limit=${limit}`, [status]);

  const setQuoteStatus = async (quote: Quote, next: string) => {
    try { await api.patch(`/quotes/${quote.id}/status`, { status: next, version: quote.version }); toast.success(`Quote marked ${next}`); quotes.reload(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not update the quote"); }
  };
  const convert = async (quote: Quote) => {
    if (!window.confirm(`Turn ${quote.quoteNumber} into an invoice for ${money(quote.totalMinor, quote.currency)}? The patient will owe this amount.`)) return;
    try {
      const invoice = await api.post<{ invoiceNumber: string }>(`/quotes/${quote.id}/convert`, { version: quote.version });
      toast.success(`Invoice ${invoice.invoiceNumber} raised from ${quote.quoteNumber}`);
      quotes.reload();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not raise the invoice"); }
  };

  return <div className="page">
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div><p className="section-title">Before the sale</p><h1 className="page-title">Quotes</h1><p className="page-description">An estimate a patient can take away. Nothing is owed until a quote is converted into an invoice.</p></div>
      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogTrigger asChild><Button><Plus className="h-4 w-4" />New quote</Button></DialogTrigger>
        {creating && <QuoteForm onSaved={() => { setCreating(false); quotes.reload(); }} />}
      </Dialog>
    </div>
    <div className="mt-6 w-56"><Field label="Status"><Select value={status} onChange={(event) => setStatus(event.target.value)}><option value="">All quotes</option><option value="draft">Draft</option><option value="sent">Sent</option><option value="accepted">Accepted</option><option value="declined">Declined</option><option value="converted">Converted</option></Select></Field></div>
    <Card className="mt-4 overflow-hidden">
      <CardHeader><CardTitle>Quote register</CardTitle><CardDescription>A quote past its validity date is shown as expired without anything having to run overnight.</CardDescription></CardHeader>
      <CardContent>
        {quotes.loading ? <Skeleton className="h-64" /> : quotes.error ? <ErrorState message={quotes.error.message} retry={quotes.reload} /> : quotes.items.length ? <>
          <Table>
            <thead><tr><Th>Quote</Th><Th>Customer</Th><Th>Status</Th><Th>Valid until</Th><Th>Total</Th><Th>Action</Th></tr></thead>
            <tbody>{quotes.items.map((quote) => <tr key={quote.id}>
              <Td className="font-mono text-xs font-bold">{quote.quoteNumber}<div className="text-[11px] font-normal text-zinc-500">{dateTime(quote.createdAt)}</div></Td>
              <Td>{quote.customerName || "—"}</Td>
              <Td><Badge tone={statusTone(quote.status, quote.expired)}>{quote.expired && quote.status !== "converted" ? "expired" : quote.status}</Badge>{quote.convertedInvoiceNumber && <div className="mt-1 font-mono text-[11px] text-zinc-500">{quote.convertedInvoiceNumber}</div>}</Td>
              <Td>{quote.validUntil || "—"}</Td>
              <Td className="font-mono font-bold">{money(quote.totalMinor, quote.currency)}</Td>
              <Td><div className="flex flex-wrap gap-2">
                {quote.status === "draft" && <Button size="sm" variant="outline" onClick={() => setQuoteStatus(quote, "sent")}><Send className="h-3 w-3" />Mark sent</Button>}
                {(quote.status === "sent" || quote.status === "draft") && <Button size="sm" variant="ghost" onClick={() => setQuoteStatus(quote, "declined")}>Declined</Button>}
                {quote.status !== "converted" && quote.status !== "declined" && <Button size="sm" onClick={() => convert(quote)}><FileCheck2 className="h-3 w-3" />Raise invoice</Button>}
              </div></Td>
            </tr>)}</tbody>
          </Table>
          <Pager page={quotes.page} pageSize={quotes.pageSize} total={quotes.total} hasMore={quotes.hasMore} onPrevious={quotes.previous} onNext={quotes.next} />
        </> : <EmptyState title="No quotes" description="Quote a patient for frames and lenses before they commit to the purchase." action={<Button onClick={() => setCreating(true)}>Write the first quote</Button>} />}
      </CardContent>
    </Card>
  </div>;
}

function QuoteForm({ onSaved }: { onSaved(): void }) {
  const [patientId, setPatientId] = React.useState("");
  const [customerName, setCustomerName] = React.useState("");
  const [validUntil, setValidUntil] = React.useState("");
  const [notes, setNotes] = React.useState("");
  const [lines, setLines] = React.useState<QuoteLine[]>([]);
  const [currency, setCurrency] = React.useState("HTG");
  const [saving, setSaving] = React.useState(false);

  const addItem = (item: InventoryItem) => {
    if (lines.some((line) => line.inventoryItemId === item.id)) return;
    if (lines.length === 0) setCurrency(item.currency);
    setLines([...lines, { inventoryItemId: item.id, description: item.name, quantity: 1, unitPriceMinor: item.salePriceMinor, discountMinor: 0 }]);
  };
  const update = (index: number, changes: Partial<QuoteLine>) => setLines(lines.map((line, position) => position === index ? { ...line, ...changes } : line));
  const total = lines.reduce((sum, line) => sum + Math.max(0, line.quantity * line.unitPriceMinor - line.discountMinor), 0);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      await api.post("/quotes", { patientId, customerName, currency, validUntil, notes, status: "draft", items: lines });
      toast.success("Quote created");
      onSaved();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not create the quote"); }
    finally { setSaving(false); }
  };

  return <DialogContent className="max-w-3xl">
    <DialogHeader><DialogTitle>New quote</DialogTitle><DialogDescription>Prices come from the catalogue and can be adjusted per line. Nothing is owed and no stock moves until the quote is converted.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <div className="grid gap-4 sm:grid-cols-2">
        <FieldGroup label="Patient" hint="Leave empty and give a name instead for somebody not on file"><PatientPicker value={patientId} onChange={setPatientId} placeholder="Search patient…" /></FieldGroup>
        <Field label="Customer name"><Input value={customerName} onChange={(event) => setCustomerName(event.target.value)} placeholder="For an enquiry with no patient record" /></Field>
        <Field label="Valid until"><Input type="date" value={validUntil} onChange={(event) => setValidUntil(event.target.value)} /></Field>
        <Field label="Currency"><Select value={currency} onChange={(event) => setCurrency(event.target.value)}><option>HTG</option><option>USD</option></Select></Field>
      </div>
      <div className="rounded-lg border bg-zinc-50 p-3"><InventoryPicker label="Add an item" exclude={lines.map((line) => line.inventoryItemId)} onSelect={addItem} /></div>
      {lines.length ? <div className="grid gap-2">{lines.map((line, index) => (
        <div className="flex flex-wrap items-center gap-3 rounded-md border p-3 text-sm" key={line.inventoryItemId || index}>
          <div className="min-w-0 flex-1 truncate font-semibold">{line.description}</div>
          <label className="flex items-center gap-2 text-xs">Qty<Input className="h-9 w-20" type="number" min={1} value={line.quantity} onChange={(event) => update(index, { quantity: Number(event.target.value) })} /></label>
          <label className="flex items-center gap-2 text-xs">Unit<Input className="h-9 w-28" type="number" min={0} value={line.unitPriceMinor} onChange={(event) => update(index, { unitPriceMinor: Number(event.target.value) })} /></label>
          <label className="flex items-center gap-2 text-xs">Discount<Input className="h-9 w-24" type="number" min={0} value={line.discountMinor} onChange={(event) => update(index, { discountMinor: Number(event.target.value) })} /></label>
          <div className="font-mono font-bold">{money(Math.max(0, line.quantity * line.unitPriceMinor - line.discountMinor), currency)}</div>
          <Button aria-label="Remove line" size="icon" type="button" variant="ghost" onClick={() => setLines(lines.filter((_, position) => position !== index))}>×</Button>
        </div>
      ))}</div> : <div className="rounded-md border border-dashed p-5 text-center text-sm text-zinc-500">Search for an item to quote for.</div>}
      <Field label="Notes"><Textarea value={notes} onChange={(event) => setNotes(event.target.value)} /></Field>
      <DialogFooter>
        <div className="mr-auto font-mono text-sm font-bold">Total: {money(total, currency)}</div>
        <Button type="submit" disabled={saving || lines.length === 0 || (!patientId && !customerName.trim())}>{saving ? "Creating…" : "Create quote"}</Button>
      </DialogFooter>
    </form>
  </DialogContent>;
}
