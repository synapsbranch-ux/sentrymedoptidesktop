import * as React from "react";
import { Archive, AlertTriangle, History, Image as ImageIcon, PackagePlus, PackageSearch, RotateCcw, Search, SlidersHorizontal, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useAuth } from "../auth";
import { usePagedList, useLoad } from "../hooks";
import { dateTime } from "../lib";
import { useRealtime } from "../realtime";
import type { InventoryItem, StockMovement, Supplier } from "../types";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "../components/ui/dialog";
import { Badge, EmptyState, ErrorState, Pager, Skeleton, Table, Td, Th } from "../components/ui/data";
import { Field, Input, Select } from "../components/ui/input";
import { MoneyInput } from "../components/ui/money-input";
import { CodeCombobox } from "../components/code-search";
import { FirstImageThumbnail, ImageGallery } from "../components/image-gallery";
import { money } from "../lib";

const categoryOptions = [
  { value: "frame", label: "Frames" },
  { value: "ophthalmic_lens", label: "Ophthalmic lenses" },
  { value: "contact_lens", label: "Contact lenses" },
  { value: "accessory", label: "Accessories" },
  { value: "service", label: "Services" },
];

export function InventoryPage() {
  const { revision } = useRealtime();
  const [query, setQuery] = React.useState("");
  const [category, setCategory] = React.useState("");
  const [low, setLow] = React.useState(false);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [selected, setSelected] = React.useState<InventoryItem | null>(null);
  const inventory = usePagedList<InventoryItem>((page, limit) => `/inventory?q=${encodeURIComponent(query)}&category=${category}&lowStock=${low}&page=${page}&limit=${limit}`, [query, category, low, revision]);
  return <div className="page">
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div><p className="section-title">Catalog & stock ledger</p><h1 className="page-title">Inventory</h1><p className="page-description">Every quantity change creates an attributable stock movement.</p></div>
      <Dialog open={createOpen} onOpenChange={setCreateOpen}><DialogTrigger asChild><Button><PackagePlus className="h-4 w-4" />New item</Button></DialogTrigger><InventoryForm onSaved={() => { setCreateOpen(false); inventory.reload(); }} /></Dialog>
    </div>
    <div className="mt-6 grid gap-3 sm:grid-cols-[1fr_200px_auto]">
      <div className="relative"><Search className="absolute left-3 top-3.5 h-4 w-4 text-zinc-400" /><Input className="pl-10" placeholder="SKU, barcode, name or brand…" value={query} onChange={(event) => setQuery(event.target.value)} /></div>
      <Select value={category} onChange={(event) => setCategory(event.target.value)}><option value="">All categories</option>{categoryOptions.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}</Select>
      <Button variant={low ? "default" : "outline"} onClick={() => setLow(!low)}><AlertTriangle className="h-4 w-4" />Low stock</Button>
    </div>
    <Card className="mt-4 overflow-hidden">{inventory.loading ? <div className="p-5"><Skeleton className="h-72" /></div> : inventory.error ? <div className="p-5"><ErrorState message={inventory.error.message} retry={inventory.reload} /></div> : inventory.items.length ? <>
      <div className="hidden md:block"><Table><thead><tr><Th>SKU</Th><Th>Item</Th><Th>Category</Th><Th>Cost</Th><Th>Price</Th><Th>Stock</Th></tr></thead><tbody>{inventory.items.map((item) => <tr key={item.id} className="cursor-pointer hover:bg-zinc-50" onClick={() => setSelected(item)}>
        <Td className="font-mono text-xs font-bold">{item.sku}</Td>
        <Td><div className="flex items-center gap-3"><FirstImageThumbnail entityType="inventory_item" entityId={item.id} /><div><div className="font-semibold">{item.name}</div><div className="text-xs text-zinc-500">{[item.brand, item.model].filter(Boolean).join(" · ") || item.barcode || "—"}</div></div></div></Td>
        <Td><Badge>{item.category.replaceAll("_", " ")}</Badge></Td>
        <Td className="font-mono text-xs">{money(item.costMinor, item.currency)}</Td>
        <Td className="font-mono text-xs font-bold">{money(item.salePriceMinor, item.currency)}</Td>
        <Td><div className="flex items-center gap-2"><span className="font-mono font-bold">{item.trackStock ? item.quantity : "—"}</span>{item.lowStock && <Badge tone="warning">Low</Badge>}{item.expired && <Badge tone="danger">Expired</Badge>}</div></Td>
      </tr>)}</tbody></Table></div>
      <div className="grid gap-3 p-3 sm:grid-cols-2 md:hidden">{inventory.items.map((item) => <button className="rounded-lg border p-4 text-left" key={item.id} onClick={() => setSelected(item)}>
        <div className="flex items-start justify-between"><div><div className="font-mono text-[11px] font-bold text-zinc-500">{item.sku}</div><div className="font-semibold">{item.name}</div></div><div className="flex flex-col items-end gap-1">{item.lowStock && <Badge tone="warning">Low</Badge>}{item.expired && <Badge tone="danger">Expired</Badge>}</div></div>
        <div className="mt-4 flex items-end justify-between"><div><div className="font-mono text-lg font-bold">{money(item.salePriceMinor, item.currency)}</div><div className="text-xs text-zinc-500">Stock: {item.trackStock ? item.quantity : "not tracked"}</div></div></div>
      </button>)}</div>
      <div className="px-3 pb-3"><Pager page={inventory.page} pageSize={inventory.pageSize} total={inventory.total} hasMore={inventory.hasMore} onPrevious={inventory.previous} onNext={inventory.next} /></div>
    </> : <EmptyState title="No inventory items" description="Add frames, lenses, contact lenses, accessories and clinic services." action={<Button onClick={() => setCreateOpen(true)}>Add first item</Button>} />}</Card>
    <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>{selected && <ProductDetail item={selected} onClose={() => setSelected(null)} onChanged={() => inventory.reload()} />}</Dialog>
  </div>;
}

const emptyForm = { sku: "", barcode: "", category: "frame", name: "", brand: "", model: "", supplierId: "", costMinor: 0, salePriceMinor: 0, currency: "HTG", quantity: 0, reorderLevel: 1, trackStock: true, unit: "unit", batchNumber: "", expirationDate: "", notes: "", attributes: {}, procedureCode: "", durationMinutes: 30, bookable: false };

function SupplierSelect({ value, onChange }: { value: string; onChange(id: string): void }) {
  const suppliers = useLoad(() => api.get<{ items: Supplier[] }>("/suppliers?limit=200"));
  return <Select value={value} onChange={(event) => onChange(event.target.value)}><option value="">No supplier</option>{suppliers.data?.items.map((s) => <option key={s.id} value={s.id}>{s.company}</option>)}</Select>;
}

function InventoryForm({ onSaved }: { onSaved(): void }) {
  const [form, setForm] = React.useState(emptyForm);
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => { event.preventDefault(); setSaving(true); try { await api.post("/inventory", form); toast.success("Inventory item created"); onSaved(); } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not create item"); } finally { setSaving(false); } };
  return <DialogContent className="max-w-2xl"><DialogHeader><DialogTitle>Add inventory item</DialogTitle><DialogDescription>Opening quantity is recorded as the first stock-ledger movement.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="SKU"><Input required value={form.sku} onChange={(event) => setForm({ ...form, sku: event.target.value })} /></Field>
        <Field label="Barcode"><Input value={form.barcode} onChange={(event) => setForm({ ...form, barcode: event.target.value })} /></Field>
        <Field label="Name"><Input required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></Field>
        <Field label="Category"><Select value={form.category} onChange={(event) => setForm({ ...form, category: event.target.value })}>{categoryOptions.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}</Select></Field>
        <Field label="Brand"><Input value={form.brand} onChange={(event) => setForm({ ...form, brand: event.target.value })} /></Field>
        <Field label="Model"><Input value={form.model} onChange={(event) => setForm({ ...form, model: event.target.value })} /></Field>
        <Field label="Supplier"><SupplierSelect value={form.supplierId} onChange={(supplierId) => setForm({ ...form, supplierId })} /></Field>
        <Field label="Unit"><Select value={form.unit} onChange={(event) => setForm({ ...form, unit: event.target.value })}><option value="unit">Unit</option><option value="pair">Pair</option><option value="box">Box</option><option value="bottle">Bottle</option><option value="pack">Pack</option></Select></Field>
        <Field label="Cost"><MoneyInput currency={form.currency} value={form.costMinor} onValueChange={(v) => setForm({ ...form, costMinor: v })} /></Field>
        <Field label="Sale price"><MoneyInput currency={form.currency} value={form.salePriceMinor} onValueChange={(v) => setForm({ ...form, salePriceMinor: v })} /></Field>
        <Field label="Opening quantity"><Input type="number" min={0} value={form.quantity} onChange={(event) => setForm({ ...form, quantity: Number(event.target.value) })} /></Field>
        <Field label="Reorder level"><Input type="number" min={0} value={form.reorderLevel} onChange={(event) => setForm({ ...form, reorderLevel: Number(event.target.value) })} /></Field>
        <Field label="Batch / lot number" hint="If applicable"><Input value={form.batchNumber} onChange={(event) => setForm({ ...form, batchNumber: event.target.value })} /></Field>
        <Field label="Expiration date" hint="If applicable"><Input type="date" value={form.expirationDate} onChange={(event) => setForm({ ...form, expirationDate: event.target.value })} /></Field>
      </div>
      <Field label="Billing procedure code" hint="Search CPT/HCPCS-style codes; carries onto every invoice line and the superbill."><CodeCombobox endpoint="/codes/procedures" placeholder="Search by code or description…" onSelect={(entry) => setForm({ ...form, procedureCode: entry.code })} /><Input className="mt-2 font-mono" placeholder="e.g. 92004" value={form.procedureCode} onChange={(event) => setForm({ ...form, procedureCode: event.target.value })} /></Field>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.trackStock} onChange={(event) => setForm({ ...form, trackStock: event.target.checked })} />Track stock quantity</label>
      {form.category === "service" && <div className="grid gap-3 rounded-md border bg-zinc-50 p-3 sm:grid-cols-2"><Field label="Duration (minutes)" hint="How long this service takes when it is booked."><Input type="number" min={0} max={1440} value={form.durationMinutes} onChange={(event) => setForm({ ...form, durationMinutes: Number(event.target.value) })} /></Field><label className="flex items-end gap-2 pb-2 text-sm"><input type="checkbox" checked={form.bookable} onChange={(event) => setForm({ ...form, bookable: event.target.checked })} />Offer as an appointment type</label></div>}
      <DialogFooter><Button type="submit" disabled={saving}>{saving ? "Creating…" : "Create item"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

type DetailTab = "details" | "stock" | "history" | "images";

function ProductDetail({ item, onClose, onChanged }: { item: InventoryItem; onClose(): void; onChanged(): void }) {
  const { user } = useAuth();
  const doctor = user?.role === "doctor";
  const [tab, setTab] = React.useState<DetailTab>("details");
  const detail = useLoad(() => api.get<InventoryItem>(`/inventory/${item.id}`), [item.id]);
  const current = detail.data ?? item;
  const refresh = () => { detail.reload(); onChanged(); };
  return <DialogContent className="max-w-3xl">
    <DialogHeader><DialogTitle className="flex flex-wrap items-center gap-2">{current.name}{current.archived && <Badge tone="neutral">Archived</Badge>}{current.lowStock && <Badge tone="warning">Low stock</Badge>}{current.expired && <Badge tone="danger">Expired</Badge>}</DialogTitle><DialogDescription className="font-mono text-xs">{current.sku}</DialogDescription></DialogHeader>
    <div className="flex flex-wrap gap-1 border-b pb-2">
      <TabButton active={tab === "details"} onClick={() => setTab("details")}>Details</TabButton>
      {current.trackStock && <TabButton active={tab === "stock"} onClick={() => setTab("stock")}><PackageSearch className="h-3.5 w-3.5" />Stock</TabButton>}
      {current.trackStock && <TabButton active={tab === "history"} onClick={() => setTab("history")}><History className="h-3.5 w-3.5" />History</TabButton>}
      <TabButton active={tab === "images"} onClick={() => setTab("images")}><ImageIcon className="h-3.5 w-3.5" />Images</TabButton>
    </div>
    {tab === "details" && <DetailsTab item={current} doctor={doctor} onChanged={refresh} onClose={onClose} />}
    {tab === "stock" && <StockTab item={current} onChanged={refresh} />}
    {tab === "history" && <HistoryTab itemId={current.id} />}
    {tab === "images" && <div className="pt-4"><ImageGallery entityType="inventory_item" entityId={current.id} canEdit emptyHint="No photographs yet." /></div>}
  </DialogContent>;
}

function TabButton({ active, onClick, children }: { active: boolean; onClick(): void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-semibold ${active ? "bg-black text-white" : "text-zinc-500 hover:bg-zinc-100"}`}>{children}</button>;
}

function DetailsTab({ item, doctor, onChanged, onClose }: { item: InventoryItem; doctor: boolean; onChanged(): void; onClose(): void }) {
  const [form, setForm] = React.useState({
    sku: item.sku, barcode: item.barcode, category: item.category, name: item.name, brand: item.brand, model: item.model,
    supplierId: item.supplierId, costMinor: item.costMinor, salePriceMinor: item.salePriceMinor, currency: item.currency,
    reorderLevel: item.reorderLevel, trackStock: item.trackStock, unit: item.unit, batchNumber: item.batchNumber,
    expirationDate: item.expirationDate, notes: item.notes, procedureCode: item.procedureCode, attributes: item.attributes,
    durationMinutes: item.durationMinutes, bookable: item.bookable,
  });
  const [saving, setSaving] = React.useState(false);
  const save = async (event: React.FormEvent) => {
    event.preventDefault(); setSaving(true);
    try { await api.put(`/inventory/${item.id}`, { ...form, version: item.version }); toast.success("Item updated"); onChanged(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not update item"); }
    finally { setSaving(false); }
  };
  const archive = async () => {
    if (!window.confirm(`Archive ${item.name}? It will no longer appear in the active catalogue.`)) return;
    try { await api.delete(`/inventory/${item.id}?version=${item.version}`); toast.success("Item archived"); onChanged(); onClose(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not archive item"); }
  };
  const reactivate = async () => { try { await api.post(`/inventory/${item.id}/reactivate`, { version: item.version }); toast.success("Item reactivated"); onChanged(); } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not reactivate item"); } };
  const remove = async () => {
    if (!window.confirm(`Permanently delete ${item.name}? This cannot be undone.`)) return;
    try { await api.post(`/inventory/${item.id}/permanent-delete?version=${item.version}`, {}); toast.success("Item deleted"); onChanged(); onClose(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Archive it instead: this item has history."); }
  };
  return <form className="grid gap-4 pt-4" onSubmit={save}>
    <div className="grid gap-4 sm:grid-cols-2">
      <Field label="SKU"><Input required value={form.sku} onChange={(e) => setForm({ ...form, sku: e.target.value })} /></Field>
      <Field label="Barcode"><Input value={form.barcode} onChange={(e) => setForm({ ...form, barcode: e.target.value })} /></Field>
      <Field label="Name"><Input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
      <Field label="Category"><Select value={form.category} onChange={(e) => setForm({ ...form, category: e.target.value })}>{categoryOptions.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}</Select></Field>
      <Field label="Brand"><Input value={form.brand} onChange={(e) => setForm({ ...form, brand: e.target.value })} /></Field>
      <Field label="Model"><Input value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} /></Field>
      <Field label="Supplier"><SupplierSelect value={form.supplierId} onChange={(supplierId) => setForm({ ...form, supplierId })} /></Field>
      <Field label="Unit"><Select value={form.unit} onChange={(e) => setForm({ ...form, unit: e.target.value })}><option value="unit">Unit</option><option value="pair">Pair</option><option value="box">Box</option><option value="bottle">Bottle</option><option value="pack">Pack</option></Select></Field>
      <Field label="Cost"><MoneyInput currency={form.currency} value={form.costMinor} onValueChange={(v) => setForm({ ...form, costMinor: v })} /></Field>
      <Field label="Sale price"><MoneyInput currency={form.currency} value={form.salePriceMinor} onValueChange={(v) => setForm({ ...form, salePriceMinor: v })} /></Field>
      <Field label="Reorder level"><Input type="number" min={0} value={form.reorderLevel} onChange={(e) => setForm({ ...form, reorderLevel: Number(e.target.value) })} /></Field>
      <Field label="Batch / lot number"><Input value={form.batchNumber} onChange={(e) => setForm({ ...form, batchNumber: e.target.value })} /></Field>
      <Field label="Expiration date"><Input type="date" value={form.expirationDate} onChange={(e) => setForm({ ...form, expirationDate: e.target.value })} /></Field>
    </div>
    <Field label="Notes"><Input value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></Field>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.trackStock} onChange={(e) => setForm({ ...form, trackStock: e.target.checked })} />Track stock quantity</label>
    <div className="flex flex-wrap items-center gap-2 border-t pt-4">
      {doctor && (item.archived
        ? <Button type="button" variant="outline" size="sm" onClick={reactivate}><RotateCcw className="h-3.5 w-3.5" />Reactivate</Button>
        : <Button type="button" variant="outline" size="sm" onClick={archive}><Archive className="h-3.5 w-3.5" />Archive</Button>)}
      {doctor && <Button type="button" variant="ghost" size="sm" className="text-red-700 hover:text-red-700" onClick={remove}><Trash2 className="h-3.5 w-3.5" />Delete</Button>}
      <Button type="submit" className="ml-auto" disabled={saving}>{saving ? "Saving…" : "Save changes"}</Button>
    </div>
  </form>;
}

const stockActions = [
  { type: "purchase", label: "Receive stock", hint: "Stock arriving from a supplier or manual restock." },
  { type: "adjustment", label: "Adjust", hint: "A correction not covered by the other reasons. Use a negative quantity to reduce stock." },
  { type: "damage", label: "Register damaged", hint: "Physically damaged and no longer sellable." },
  { type: "expired", label: "Register expired", hint: "Past its expiration date on the shelf." },
  { type: "loss", label: "Register lost", hint: "Missing/unaccounted for stock." },
] as const;

function StockTab({ item, onChanged }: { item: InventoryItem; onChanged(): void }) {
  const [action, setAction] = React.useState<(typeof stockActions)[number] | null>(null);
  return <div className="grid gap-4 pt-4">
    <div className="rounded-lg border bg-zinc-50 p-4 text-center"><div className="text-xs font-bold uppercase text-zinc-500">Current stock</div><div className="mt-1 font-mono text-3xl font-bold">{item.quantity} <span className="text-base font-normal text-zinc-500">{item.unit}</span></div></div>
    <div className="grid gap-2 sm:grid-cols-2">
      {stockActions.map((candidate) => <Button key={candidate.type} variant="outline" onClick={() => setAction(candidate)}>{candidate.label}</Button>)}
    </div>
    <Dialog open={Boolean(action)} onOpenChange={(open) => !open && setAction(null)}>{action && <StockActionForm item={item} action={action} onSaved={() => { setAction(null); onChanged(); }} />}</Dialog>
  </div>;
}

function StockActionForm({ item, action, onSaved }: { item: InventoryItem; action: (typeof stockActions)[number]; onSaved(): void }) {
  const negative = action.type === "damage" || action.type === "expired" || action.type === "loss";
  const [quantity, setQuantity] = React.useState(negative ? 1 : 0);
  const [reason, setReason] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const signedQuantity = negative ? -Math.abs(quantity) : quantity;
  const newQuantity = item.quantity + signedQuantity;
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setSaving(true);
    try { await api.post(`/inventory/${item.id}/movements`, { type: action.type, quantity: signedQuantity, reason, version: item.version }); toast.success("Stock movement recorded"); onSaved(); }
    catch (reason_) { toast.error(reason_ instanceof APIError ? reason_.body.message : "Could not record movement"); }
    finally { setSaving(false); }
  };
  return <DialogContent><DialogHeader><DialogTitle>{action.label}</DialogTitle><DialogDescription>{action.hint} Current stock: {item.quantity}.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <Field label={negative ? "Quantity" : "Quantity change"} hint={negative ? undefined : "Positive to add, negative to remove."}>
        <Input type="number" min={negative ? 1 : undefined} value={quantity} onChange={(event) => setQuantity(Number(event.target.value))} required />
      </Field>
      <Field label="Reason / notes"><Input value={reason} onChange={(event) => setReason(event.target.value)} required /></Field>
      <div className="rounded-md bg-zinc-50 p-3 text-sm">New stock: <strong className="font-mono">{newQuantity}</strong></div>
      <DialogFooter><Button type="submit" disabled={saving || quantity === 0 || newQuantity < 0}>{saving ? "Recording…" : "Record movement"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

function HistoryTab({ itemId }: { itemId: string }) {
  const movements = useLoad(() => api.get<{ items: StockMovement[] }>(`/inventory/${itemId}/movements`), [itemId]);
  if (movements.loading) return <div className="pt-4"><Skeleton className="h-64" /></div>;
  if (movements.error || !movements.data) return <div className="pt-4"><ErrorState message={movements.error?.message ?? "History unavailable"} retry={movements.reload} /></div>;
  if (!movements.data.items.length) return <div className="pt-4"><EmptyState title="No stock movements yet" description="Every quantity change will appear here." /></div>;
  return <div className="grid max-h-[50vh] gap-2 overflow-y-auto pt-4">{movements.data.items.map((movement) => <div key={movement.id} className="flex items-center justify-between gap-3 rounded-md border p-3 text-sm">
    <div><div className="font-semibold capitalize">{movement.type.replaceAll("_", " ")}</div><div className="text-xs text-zinc-500">{movement.reason} · {movement.user} · {dateTime(movement.createdAt)}</div></div>
    <div className="text-right"><div className={`font-mono font-bold ${movement.quantityChange < 0 ? "text-red-700" : "text-emerald-700"}`}>{movement.quantityChange > 0 ? "+" : ""}{movement.quantityChange}</div><div className="font-mono text-xs text-zinc-500">{movement.previousQuantity} → {movement.resultingQuantity}</div></div>
  </div>)}</div>;
}
