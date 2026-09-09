import * as React from "react";
import { Barcode, ClipboardList, Minus, PauseCircle, Plus, Search, ShoppingCart, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useDebouncedValue, useLoad } from "../hooks";
import { money } from "../lib";
import type { InventoryItem } from "../types";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Badge, EmptyState, ErrorState, Skeleton } from "../components/ui/data";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "../components/ui/dialog";
import { Field, FieldGroup, Input, Select } from "../components/ui/input";
import { PatientPicker } from "../components/patient-search";
import { printReceipt } from "../components/printing";
import { useClinicIdentity, usePrintingPreferences } from "../clinic";
import { useAuth } from "../auth";

/** A cart line carries its own discount, so a concession on one item does not have to be applied to the whole sale. */
interface CartLine { item: InventoryItem; quantity: number; discountMinor: number }
interface Tender { id: string; paymentMethodId: string; amountMinor: number }
interface ServiceType { id: string; sku: string; name: string; salePriceMinor: number; currency: string; durationMinutes: number; bookable: boolean; procedureCode: string; unpriced: boolean }
interface PrescriptionOption { id: string; sku: string; name: string; brand: string; salePriceMinor: number; currency: string; quantity: number; trackStock: boolean; inStock: boolean }
interface PrescriptionCart {
  prescription: { id: string; prescriptionNumber: string; type: string; issuedAt: string; expiresAt: string; expired: boolean; patientId: string; patientName: string };
  lines: { role: string; searchTerm: string; options: PrescriptionOption[]; needsLabOrder: boolean }[];
}
interface ResumedLine { inventoryItemId: string; sku: string; barcode: string; category: string; name: string; brand: string; salePriceMinor: number; currency: string; quantity: number; trackStock: boolean; procedureCode: string; cartQuantity: number; discountMinor: number; inStock: boolean }
interface ParkedSale { id: string; label: string; patientId: string; patientName: string; currency: string; totalMinor: number; note: string; createdAt: string; parkedBy: string }

const lineGross = (line: CartLine) => line.item.salePriceMinor * line.quantity;
const lineTotal = (line: CartLine) => Math.max(0, lineGross(line) - line.discountMinor);

export function POSPage() {
  const [query, setQuery] = React.useState("");
  // The catalogue is searched on the server: a POS that filtered a preloaded
  // list would quietly stop showing items once the catalogue outgrew one page.
  const search = useDebouncedValue(query, 200);
  const inventory = useLoad(() => api.get<{ items: InventoryItem[]; total: number }>(`/inventory?q=${encodeURIComponent(search)}&limit=50`), [search]);
  const services = useLoad(() => api.get<{ items: ServiceType[] }>("/pos/service-types"), []);
  const methods = useLoad(() => api.get<{ items: { id: string; name: string }[] }>("/payment-methods"), []);
  const register = useLoad(() => api.get<{ open: boolean; id?: string }>("/cash-register"), []);
  const parked = useLoad(() => api.get<{ items: ParkedSale[] }>("/pos/parked"), []);

  const [cart, setCart] = React.useState<CartLine[]>([]);
  const [patientId, setPatientId] = React.useState("");
  const [orderDiscount, setOrderDiscount] = React.useState(0);
  const [tenders, setTenders] = React.useState<Tender[]>([]);
  const [saving, setSaving] = React.useState(false);
  const [showParked, setShowParked] = React.useState(false);
  const [prescription, setPrescription] = React.useState<PrescriptionCart | null>(null);
  const clinic = useClinicIdentity();
  const printing = usePrintingPreferences();
  const { user } = useAuth();

  const visible = inventory.data?.items ?? [];
  const currency = cart[0]?.item.currency ?? "HTG";
  const subtotal = cart.reduce((sum, line) => sum + lineTotal(line), 0);
  const total = Math.max(0, subtotal - orderDiscount);
  const tendered = tenders.reduce((sum, tender) => sum + tender.amountMinor, 0);
  const defaultMethod = methods.data?.items[0]?.id ?? "";

  const add = (item: InventoryItem) => setCart((current) => {
    if (current.length && current[0].item.currency !== item.currency) { toast.error("Complete this sale before adding an item in another currency."); return current; }
    const existing = current.find((line) => line.item.id === item.id);
    return existing
      ? current.map((line) => line.item.id === item.id ? { ...line, quantity: line.quantity + 1 } : line)
      : [...current, { item, quantity: 1, discountMinor: 0 }];
  });
  const change = (id: string, delta: number) => setCart((current) => current.map((line) => line.item.id === id ? { ...line, quantity: Math.max(0, line.quantity + delta) } : line).filter((line) => line.quantity > 0));
  const discountLine = (id: string, value: number) => setCart((current) => current.map((line) => line.item.id === id ? { ...line, discountMinor: Math.max(0, Math.min(value, lineGross(line))) } : line));
  const clear = () => { setCart([]); setTenders([]); setOrderDiscount(0); setPrescription(null); };

  const checkout = async () => {
    setSaving(true);
    try {
      const payments = tenders.filter((tender) => tender.amountMinor > 0).map((tender) => ({ paymentMethodId: tender.paymentMethodId, registerSessionId: register.data?.id ?? "", amountMinor: tender.amountMinor, currency, exchangeRate: "1", reference: "", notes: "" }));
      const result = await api.post<{ invoiceNumber: string; balanceMinor: number; payments: { receiptNumber: string }[] }>("/pos/checkout", {
        invoice: {
          patientId, currency, exchangeRate: "1", discountMinor: orderDiscount, taxMinor: 0, dueAt: "", notes: "POS sale",
          items: cart.map((line) => ({ inventoryItemId: line.item.id, description: line.item.name, quantity: line.quantity, unitPriceMinor: line.item.salePriceMinor, discountMinor: line.discountMinor, taxMinor: 0 })),
        },
        payments,
      });
      toast.success(`${result.invoiceNumber} created${result.balanceMinor > 0 ? ` · ${money(result.balanceMinor, currency)} still owed` : ""}`);
      // The receipt goes to the thermal roll, never through the document path:
      // a till receipt on A4 wastes a sheet and looks nothing like a receipt.
      printReceipt({
        clinicName: clinic.name ?? "Clinic", clinicAddress: clinic.address, clinicPhone: clinic.phone,
        invoiceNumber: result.invoiceNumber, issuedAt: new Date().toLocaleString(), cashier: user?.displayName,
        currency, lines: cart.map((line) => ({ description: line.item.name, quantity: line.quantity, unitPriceMinor: line.item.salePriceMinor, discountMinor: line.discountMinor })),
        discountMinor: orderDiscount, totalMinor: total,
        payments: tenders.filter((tender) => tender.amountMinor > 0).map((tender) => ({ method: methods.data?.items.find((method) => method.id === tender.paymentMethodId)?.name ?? "Payment", amountMinor: tender.amountMinor })),
        balanceMinor: result.balanceMinor,
      }, printing.receiptWidth);
      clear();
      inventory.reload();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Checkout failed");
    } finally { setSaving(false); }
  };

  const park = async () => {
    const label = window.prompt("Park this sale as:", cart[0]?.item.name ?? "Held sale");
    if (!label) return;
    try {
      await api.post("/pos/parked", { label, patientId, currency, totalMinor: total, cart: cart.map((line) => ({ inventoryItemId: line.item.id, description: line.item.name, quantity: line.quantity, unitPriceMinor: line.item.salePriceMinor, discountMinor: line.discountMinor })) });
      toast.success("Sale parked");
      clear();
      parked.reload();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not park the sale"); }
  };

  // A parked cart is stored as line data, not as inventory objects, so resuming
  // it re-reads each item from the catalogue — prices and stock are today's, not
  // the ones from when the sale was set aside.
  const resume = async (sale: ParkedSale) => {
    try {
      const held = await api.post<{ patientId: string; cart: ResumedLine[]; unavailable: { description: string }[] }>(`/pos/parked/${sale.id}/resume`, {});
      setCart(held.cart.map((line) => ({
        item: { id: line.inventoryItemId, sku: line.sku, barcode: line.barcode, category: line.category, name: line.name, brand: line.brand, model: "", attributes: {}, supplierId: "", costMinor: 0, salePriceMinor: line.salePriceMinor, currency: line.currency, quantity: line.quantity, reorderLevel: 0, trackStock: line.trackStock, procedureCode: line.procedureCode, lowStock: false, version: 1, updatedAt: "" } as InventoryItem,
        quantity: line.cartQuantity,
        discountMinor: line.discountMinor,
      })));
      setPatientId(held.patientId ?? "");
      setShowParked(false);
      parked.reload();
      if (held.unavailable.length) toast.warning(`${held.unavailable.map((line) => line.description).join(", ")} left the catalogue and could not be restored.`);
      else toast.success("Parked sale resumed at today's prices");
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not resume the sale"); }
  };

  const lookUpPrescription = async () => {
    if (!patientId) { toast.error("Choose the patient first."); return; }
    try {
      setPrescription(await api.get<PrescriptionCart>(`/pos/prescription-cart?patientId=${encodeURIComponent(patientId)}`));
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not read a prescription for this patient"); }
  };

  return <div className="page">
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div><p className="section-title">Transactional checkout</p><h1 className="page-title">Point of Sale</h1><p className="page-description">Invoice, payment and stock deduction commit together or roll back together.</p></div>
      <div className="flex gap-2">
        <Button variant="outline" onClick={() => setShowParked(true)}><PauseCircle className="h-4 w-4" />Parked sales{parked.data?.items.length ? ` (${parked.data.items.length})` : ""}</Button>
      </div>
    </div>
    <div className="mt-6 grid gap-4 xl:grid-cols-[1fr_420px]">
      <div className="grid gap-4">
        <ScanField onScanned={add} />
        <Card>
          <CardHeader><CardTitle>Products & services</CardTitle><CardDescription>Showing {visible.length} of {inventory.data?.total ?? 0} matching items — narrow the search to see more.</CardDescription></CardHeader>
          <CardContent>
            <div className="relative mb-4"><Search className="absolute left-3 top-3.5 h-4 w-4 text-zinc-400" /><Input className="pl-10" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search the catalogue…" /></div>
            {inventory.loading ? <Skeleton className="h-72" /> : inventory.error ? <ErrorState message={inventory.error.message} retry={inventory.reload} /> : visible.length ? (
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{visible.map((item) => (
                <button key={item.id} onClick={() => add(item)} disabled={item.trackStock && item.quantity <= 0} className="rounded-lg border p-4 text-left hover:border-black disabled:cursor-not-allowed disabled:opacity-40">
                  <div className="font-mono text-[10px] font-bold text-zinc-500">{item.sku}</div>
                  <div className="mt-1 font-semibold">{item.name}</div>
                  <div className="mt-4 flex items-end justify-between"><span className="font-mono font-bold">{money(item.salePriceMinor, item.currency)}</span><span className="text-xs text-zinc-500">{item.trackStock ? `${item.quantity} in stock` : "Service"}</span></div>
                </button>
              ))}</div>
            ) : <EmptyState title="No matching items" description="Change your search or add items in Inventory." />}
          </CardContent>
        </Card>
        <ServiceStrip services={services.data?.items ?? []} onAdd={add} />
      </div>

      <Card className="h-fit xl:sticky xl:top-20">
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><ShoppingCart className="h-4 w-4" />Current sale</CardTitle>
          <CardDescription>{cart.reduce((sum, line) => sum + line.quantity, 0)} line units</CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup label="Patient / customer" hint="Leave empty for an anonymous retail customer"><PatientPicker value={patientId} onChange={setPatientId} placeholder="Search patient, or leave empty…" /></FieldGroup>
          <div className="mt-3 flex gap-2"><Button size="sm" variant="outline" onClick={lookUpPrescription}><ClipboardList className="h-3.5 w-3.5" />Look up prescription</Button></div>
          {prescription && <PrescriptionSuggestions cart={prescription} onAdd={add} onDismiss={() => setPrescription(null)} />}
          {cart.length ? <div className="mt-4 space-y-3">
            {cart.map((line) => (
              <div key={line.item.id} className="grid gap-2 border-b pb-3">
                <div className="flex items-center gap-3">
                  <div className="min-w-0 flex-1"><div className="truncate text-sm font-semibold">{line.item.name}</div><div className="font-mono text-xs text-zinc-500">{money(line.item.salePriceMinor, line.item.currency)}</div></div>
                  <div className="flex items-center rounded-md border">
                    <button aria-label={`Decrease ${line.item.name}`} className="grid h-11 w-11 place-items-center sm:h-9 sm:w-9" onClick={() => change(line.item.id, -1)}><Minus className="h-3 w-3" /></button>
                    <span className="w-8 text-center font-mono text-xs">{line.quantity}</span>
                    <button aria-label={`Increase ${line.item.name}`} className="grid h-11 w-11 place-items-center sm:h-9 sm:w-9" onClick={() => change(line.item.id, 1)}><Plus className="h-3 w-3" /></button>
                  </div>
                  <button aria-label="Remove" className="grid h-11 w-11 place-items-center text-zinc-400 hover:text-red-700 sm:h-9 sm:w-9" onClick={() => setCart(cart.filter((candidate) => candidate.item.id !== line.item.id))}><Trash2 className="h-4 w-4" /></button>
                </div>
                <label className="flex items-center justify-between gap-2 text-xs text-zinc-500">
                  Line discount
                  <span className="flex items-center gap-2"><Input aria-label={`Discount on ${line.item.name}`} className="h-9 w-28" type="number" min={0} max={lineGross(line)} value={line.discountMinor} onChange={(event) => discountLine(line.item.id, Number(event.target.value))} /><span className="font-mono font-bold text-[var(--foreground)]">{money(lineTotal(line), currency)}</span></span>
                </label>
              </div>
            ))}
            <Field label={`Order discount (${currency} minor units)`}><Input type="number" min={0} max={subtotal} value={orderDiscount} onChange={(event) => setOrderDiscount(Math.max(0, Math.min(Number(event.target.value), subtotal)))} /></Field>
            <TenderList tenders={tenders} setTenders={setTenders} methods={methods.data?.items ?? []} defaultMethod={defaultMethod} total={total} currency={currency} />
            <div className="flex items-center justify-between border-y py-4"><span className="font-semibold">Total</span><span className="font-mono text-2xl font-bold">{money(total, currency)}</span></div>
            <div className="grid gap-2">
              <Button className="w-full" disabled={saving || cart.length === 0 || tendered > total} onClick={checkout}>
                {saving ? "Completing transaction…"
                  : tenders.length === 0 ? `Complete sale · ${money(total, currency)} owed`
                  : tendered < total ? `Take ${money(tendered, currency)} · ${money(total - tendered, currency)} owed`
                  : `Complete sale · ${money(total, currency)}`}
              </Button>
              <Button className="w-full" variant="outline" disabled={saving || cart.length === 0} onClick={park}><PauseCircle className="h-4 w-4" />Park this sale</Button>
            </div>
          </div> : <div className="mt-4"><EmptyState title="Cart is empty" description="Scan a barcode, select a product, or add a clinic service to begin." /></div>}
        </CardContent>
      </Card>
    </div>

    <Dialog open={showParked} onOpenChange={setShowParked}>
      <DialogContent>
        <DialogHeader><DialogTitle>Parked sales</DialogTitle><DialogDescription>A parked sale is resumed at today's prices and stock, and can only be resumed once.</DialogDescription></DialogHeader>
        {parked.data?.items.length ? <div className="grid gap-2">{parked.data.items.map((sale) => (
          <button key={sale.id} className="rounded-md border p-3 text-left hover:border-black" onClick={() => resume(sale)}>
            <div className="flex justify-between gap-3"><span className="font-semibold">{sale.label}</span><span className="font-mono font-bold">{money(sale.totalMinor, sale.currency)}</span></div>
            <div className="mt-1 text-xs text-zinc-500">{sale.patientName || "Retail customer"} · parked by {sale.parkedBy}</div>
          </button>
        ))}</div> : <EmptyState title="Nothing parked" description="Use “Park this sale” to set a cart aside and pick it up later." />}
      </DialogContent>
    </Dialog>
  </div>;
}

/**
 * Most USB and Bluetooth barcode scanners present as a keyboard: they type the
 * code and press Enter. This field stays ready for that burst, so no driver and
 * no pointer are involved — the cashier scans and the item lands in the cart.
 */
function ScanField({ onScanned }: { onScanned(item: InventoryItem): void }) {
  const [code, setCode] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const scanned = code.trim();
    if (!scanned) return;
    setBusy(true);
    try {
      const found = await api.get<{ items: InventoryItem[] }>(`/inventory?q=${encodeURIComponent(scanned)}&limit=10`);
      const exact = found.items.find((item) => item.barcode.toLowerCase() === scanned.toLowerCase() || item.sku.toLowerCase() === scanned.toLowerCase());
      if (!exact) { toast.error(`Nothing in the catalogue matches ${scanned}.`); return; }
      if (exact.trackStock && exact.quantity <= 0) { toast.error(`${exact.name} is out of stock.`); return; }
      onScanned(exact);
    } catch { toast.error("Could not look up that code."); }
    finally { setBusy(false); setCode(""); }
  };
  return <form onSubmit={submit}>
    <label className="flex items-center gap-3 rounded-lg border bg-[var(--card)] p-3">
      <Barcode className="h-5 w-5 shrink-0 text-zinc-400" />
      <Input autoFocus aria-label="Scan a barcode" className="border-0 focus-visible:ring-0" value={code} disabled={busy} onChange={(event) => setCode(event.target.value)} placeholder="Scan a barcode or type a SKU, then press Enter…" />
    </label>
  </form>;
}

function ServiceStrip({ services, onAdd }: { services: ServiceType[]; onAdd(item: InventoryItem): void }) {
  if (!services.length) return null;
  return <Card>
    <CardHeader><CardTitle>Clinic services</CardTitle><CardDescription>Appointment, consultation and procedure types, priced once and sold here.</CardDescription></CardHeader>
    <CardContent><div className="flex flex-wrap gap-2">{services.map((service) => (
      <button key={service.id} className="rounded-md border px-3 py-2 text-left text-sm hover:border-black" onClick={() => onAdd({ id: service.id, sku: service.sku, barcode: "", category: "service", name: service.name, brand: "", model: "", attributes: {}, supplierId: "", costMinor: 0, salePriceMinor: service.salePriceMinor, currency: service.currency, quantity: 0, reorderLevel: 0, trackStock: false, procedureCode: service.procedureCode, lowStock: false, version: 1, updatedAt: "" } as InventoryItem)}>
        <span className="font-semibold">{service.name}</span>
        <span className="ml-2 font-mono text-xs">{money(service.salePriceMinor, service.currency)}</span>
        {service.unpriced && <Badge tone="warning" className="ml-2">No price set</Badge>}
      </button>
    ))}</div></CardContent>
  </Card>;
}

function PrescriptionSuggestions({ cart, onAdd, onDismiss }: { cart: PrescriptionCart; onAdd(item: InventoryItem): void; onDismiss(): void }) {
  return <div className="mt-3 grid gap-2 rounded-md border p-3 text-sm">
    <div className="flex items-start justify-between gap-2">
      <div><strong>{cart.prescription.prescriptionNumber}</strong> · {cart.prescription.type.replaceAll("_", " ")}{cart.prescription.expired && <Badge tone="danger" className="ml-2">Expired</Badge>}</div>
      <button className="text-xs text-zinc-500 underline" onClick={onDismiss}>Dismiss</button>
    </div>
    {cart.lines.map((line) => (
      <div key={line.role} className="grid gap-1">
        <div className="text-xs font-semibold uppercase text-zinc-500">{line.role.replaceAll("_", " ")}</div>
        {line.options.length ? line.options.slice(0, 4).map((option) => (
          <button key={option.id} className="flex items-center justify-between gap-3 rounded border px-2 py-1.5 text-left hover:border-black" onClick={() => onAdd({ id: option.id, sku: option.sku, barcode: "", category: "", name: option.name, brand: option.brand, model: "", attributes: {}, supplierId: "", costMinor: 0, salePriceMinor: option.salePriceMinor, currency: option.currency, quantity: option.quantity, reorderLevel: 0, trackStock: option.trackStock, procedureCode: "", lowStock: false, version: 1, updatedAt: "" } as InventoryItem)}>
            <span className="min-w-0 truncate">{option.name}</span>
            <span className="flex shrink-0 items-center gap-2"><span className="font-mono">{money(option.salePriceMinor, option.currency)}</span>{option.inStock ? <Badge tone="success">In stock</Badge> : <Badge tone="warning">Order in</Badge>}</span>
          </button>
        )) : <div className="rounded border border-dashed px-2 py-1.5 text-xs text-zinc-500">Nothing in the catalogue matches{line.needsLabOrder ? " — this will need a lab order." : "."}</div>}
      </div>
    ))}
  </div>;
}

/** One sale, several tenders: part cash, part card, part insurance. */
function TenderList({ tenders, setTenders, methods, defaultMethod, total, currency }: { tenders: Tender[]; setTenders(value: Tender[]): void; methods: { id: string; name: string }[]; defaultMethod: string; total: number; currency: string }) {
  const taken = tenders.reduce((sum, tender) => sum + tender.amountMinor, 0);
  const addTender = () => setTenders([...tenders, { id: crypto.randomUUID(), paymentMethodId: defaultMethod, amountMinor: Math.max(0, total - taken) }]);
  const update = (id: string, changes: Partial<Tender>) => setTenders(tenders.map((tender) => tender.id === id ? { ...tender, ...changes } : tender));
  return <div className="grid gap-2">
    <div className="flex items-center justify-between text-sm font-semibold">Payment<Button size="sm" variant="outline" type="button" onClick={addTender}><Plus className="h-3 w-3" />Add tender</Button></div>
    {tenders.length === 0 && <p className="text-xs text-zinc-500">No payment recorded — the sale is invoiced and the balance stays owed. Payment is never required to complete a visit.</p>}
    {tenders.map((tender) => (
      <div key={tender.id} className="grid grid-cols-[1fr_130px_auto] items-end gap-2">
        <Field label="Method"><Select value={tender.paymentMethodId} onChange={(event) => update(tender.id, { paymentMethodId: event.target.value })}>{methods.map((method) => <option key={method.id} value={method.id}>{method.name}</option>)}</Select></Field>
        <Field label={`Amount (${currency})`}><Input type="number" min={0} value={tender.amountMinor} onChange={(event) => update(tender.id, { amountMinor: Math.max(0, Number(event.target.value)) })} /></Field>
        <Button aria-label="Remove tender" size="icon" variant="ghost" type="button" onClick={() => setTenders(tenders.filter((candidate) => candidate.id !== tender.id))}><Trash2 className="h-4 w-4" /></Button>
      </div>
    ))}
    {taken > total && <p className="text-xs text-red-700">Tenders exceed the sale total by {money(taken - total, currency)}.</p>}
  </div>;
}
