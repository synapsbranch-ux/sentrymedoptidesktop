import * as React from "react";
import { Building2, Download, FileText, Globe, Plus, Printer, ShieldCheck, Upload, WalletCards } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useAuth } from "../auth";
import { saveBlob } from "../download";
import { useLoad, usePagedList } from "../hooks";
import { dateTime, money } from "../lib";
import { useRealtime } from "../realtime";
import type { Invoice, InsuranceClaimDetail, InsuranceDocument, InsuranceDocumentStatus, Payer } from "../types";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "../components/ui/dialog";
import { Badge, EmptyState, ErrorState, Pager, Skeleton, Table, Td, Th } from "../components/ui/data";
import { Field, FieldGroup, Input, Select, Textarea } from "../components/ui/input";
import { MoneyInput } from "../components/ui/money-input";
import { PatientPicker } from "../components/patient-search";
import { ImageGallery } from "../components/image-gallery";
import { PrintHeader, triggerPrint } from "../components/print";

interface ClaimProposal { currency: string; exchangeRate: string; claimAmountMinor: number; payerPortionMinor: number; patientPortionMinor: number; alreadyClaimedMinor: number; policyFound: boolean; policy: { payerId: string; payerName: string; memberNumber: string; policyNumber: string; authorization: string; coveragePercent: number } }
interface Claim { id: string; patientName: string; patientId: string; medicalRecordNumber: string; payerName: string; payerId: string; invoiceNumber: string; invoiceId: string; authorization: string; memberNumber: string; policyNumber: string; currency: string; exchangeRate: string; claimAmountMinor: number; patientPortionMinor: number; payerPortionMinor: number; paidMinor: number; outstandingMinor: number; status: string; version: number; agingBucket: string }

const claimStatuses = ["draft", "submitted", "pending", "approved", "partially_paid", "paid", "rejected", "cancelled"];
const documentStatuses: InsuranceDocumentStatus[] = ["required", "requested", "received", "completed", "submitted", "rejected", "expired"];
const documentStatusTone = (status: string): "neutral" | "success" | "warning" | "danger" =>
  status === "completed" || status === "submitted" ? "success" : status === "rejected" || status === "expired" ? "danger" : status === "received" ? "neutral" : "warning";

async function downloadStoredFile(path: string, filename: string) {
  try {
    const { blob, filename: served } = await api.blob(path);
    const outcome = await saveBlob(blob, served || filename);
    if (outcome === "saved") toast.success("Downloaded");
  } catch (reason) {
    toast.error(reason instanceof Error ? reason.message : "Could not download the file");
  }
}

export function InsurancePage() {
  const [tab, setTab] = React.useState<"claims" | "providers">("claims");
  return (
    <div className="page">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="section-title">Third-party coverage</p>
          <h1 className="page-title">Insurance</h1>
          <p className="page-description">Providers, patient coverage, claims and insurer payments in one place.</p>
        </div>
      </div>
      <div className="mt-6 inline-flex rounded-lg border border-zinc-200 bg-zinc-50 p-1">
        <Button size="sm" variant={tab === "claims" ? "default" : "ghost"} onClick={() => setTab("claims")}><WalletCards className="h-4 w-4" />Claims</Button>
        <Button size="sm" variant={tab === "providers" ? "default" : "ghost"} onClick={() => setTab("providers")}><Building2 className="h-4 w-4" />Providers</Button>
      </div>
      {tab === "claims" ? <ClaimsTab /> : <ProvidersTab />}
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string | number }) {
  return <Card><CardContent className="p-5"><div className="text-xs font-bold uppercase text-zinc-500">{label}</div><div className="mt-2 font-mono text-2xl font-bold">{value}</div></CardContent></Card>;
}

// ---- Claims -----------------------------------------------------------------

function ClaimsTab() {
  const { revision } = useRealtime();
  const { user } = useAuth();
  const doctor = user?.role === "doctor";
  const payers = useLoad(() => api.get<{ items: Payer[] }>("/insurance/payers"), [revision]);
  const claims = usePagedList<Claim>((page, limit) => `/insurance/claims?page=${page}&limit=${limit}`, [revision]);
  const claimTotals = useLoad(() => api.get<{ openClaims: number; outstandingClaims: number; overNinetyDays: number }>("/insurance/claims/summary"), [revision]);
  const [newClaim, setNewClaim] = React.useState(false);
  const [selected, setSelected] = React.useState<string | null>(null);
  const reload = () => { payers.reload(); claims.reload(); claimTotals.reload(); };
  return <>
    <div className="mt-6 grid gap-4 sm:grid-cols-3">
      <Metric label="Open claims" value={claimTotals.data?.openClaims ?? 0} />
      <Metric label="Outstanding claims" value={claimTotals.data?.outstandingClaims ?? 0} />
      <Metric label="Over 90 days" value={claimTotals.data?.overNinetyDays ?? 0} />
    </div>
    <Card className="mt-6">
      <CardHeader className="flex-row items-center justify-between">
        <div><CardTitle>Claims and receivables</CardTitle><CardDescription>Amounts are entered from paper/email remittances and remain available offline.</CardDescription></div>
        <Button onClick={() => setNewClaim(true)}><Plus className="h-4 w-4" />New claim</Button>
      </CardHeader>
      {claims.loading ? <div className="p-5"><Skeleton className="h-72" /></div> : claims.error ? <div className="p-5"><ErrorState message={claims.error.message} retry={claims.reload} /></div> : claims.items.length ? <>
        <div className="hidden overflow-x-auto md:block">
          <Table>
            <thead><tr><Th>Patient</Th><Th>Insurer</Th><Th>Authorization</Th><Th>Payer portion</Th><Th>Paid</Th><Th>Outstanding</Th><Th>Aging</Th><Th>Status</Th></tr></thead>
            <tbody>{claims.items.map((c) => <tr key={c.id} className="cursor-pointer hover:bg-zinc-50" onClick={() => setSelected(c.id)}>
              <Td><b>{c.patientName}</b><div className="font-mono text-xs text-zinc-500">{c.medicalRecordNumber}</div></Td>
              <Td>{c.payerName}</Td>
              <Td className="font-mono text-xs">{c.authorization || "—"}</Td>
              <Td className="font-mono">{money(c.payerPortionMinor, c.currency)}</Td>
              <Td className="font-mono">{money(c.paidMinor, c.currency)}</Td>
              <Td className="font-mono font-bold">{money(c.outstandingMinor, c.currency)}</Td>
              <Td>{c.agingBucket}</Td>
              <Td><Badge tone={c.status === "paid" ? "success" : c.status === "rejected" || c.status === "cancelled" ? "danger" : "warning"}>{c.status.replaceAll("_", " ")}</Badge></Td>
            </tr>)}</tbody>
          </Table>
        </div>
        <div className="divide-y md:hidden">{claims.items.map((c) => <button className="w-full p-4 text-left" key={c.id} onClick={() => setSelected(c.id)}>
          <div className="flex justify-between gap-2"><b>{c.patientName}</b><Badge>{c.status}</Badge></div>
          <div className="mt-1 text-sm text-zinc-500">{c.payerName} · {c.authorization || "No authorization"}</div>
          <div className="mt-3 flex justify-between"><span className="text-sm">Outstanding · {c.agingBucket} days</span><b className="font-mono">{money(c.outstandingMinor, c.currency)}</b></div>
        </button>)}</div>
        <div className="px-4 pb-4"><Pager page={claims.page} pageSize={claims.pageSize} total={claims.total} hasMore={claims.hasMore} onPrevious={claims.previous} onNext={claims.next} /></div>
      </> : <EmptyState title="No insurance claims" description="Create a claim when a covered patient is billed." />}
    </Card>
    <Dialog open={newClaim} onOpenChange={setNewClaim}><ClaimForm payers={payers.data?.items ?? []} onSaved={() => { setNewClaim(false); reload(); }} /></Dialog>
    <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>{selected && <ClaimDetail id={selected} doctor={doctor} onSaved={reload} onClose={() => setSelected(null)} />}</Dialog>
  </>;
}

function ClaimForm({ payers, onSaved }: { payers: Payer[]; onSaved(): void }) {
  const invoices = useLoad(() => api.get<{ items: Invoice[] }>("/invoices"));
  const [f, setF] = React.useState({
    patientId: "", payerId: "", invoiceId: "", authorization: "", memberNumber: "", policyNumber: "",
    currency: "HTG", exchangeRate: "1", claimAmountMinor: 0, patientPortionMinor: 0, payerPortionMinor: 0, notes: "",
  });
  const [saving, setSaving] = React.useState(false);
  const [proposal, setProposal] = React.useState<ClaimProposal | null>(null);
  React.useEffect(() => {
    if (!f.patientId || !f.invoiceId) { setProposal(null); return; }
    let active = true;
    api.get<ClaimProposal>(`/insurance/claims/proposal?patientId=${encodeURIComponent(f.patientId)}&invoiceId=${encodeURIComponent(f.invoiceId)}`)
      .then((next) => {
        if (!active) return;
        setProposal(next);
        setF((current) => ({ ...current, currency: next.currency, exchangeRate: next.exchangeRate,
          claimAmountMinor: next.claimAmountMinor, payerPortionMinor: next.payerPortionMinor, patientPortionMinor: next.patientPortionMinor,
          memberNumber: current.memberNumber || next.policy.memberNumber, policyNumber: current.policyNumber || next.policy.policyNumber,
          authorization: current.authorization || next.policy.authorization, payerId: current.payerId || next.policy.payerId }));
      }).catch(() => { if (active) setProposal(null); });
    return () => { active = false; };
  }, [f.patientId, f.invoiceId]);
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setSaving(true);
    try { await api.post("/insurance/claims", f); toast.success("Insurance claim created"); onSaved(); }
    catch (x) { toast.error(x instanceof APIError ? x.body.message : "Could not create claim"); }
    finally { setSaving(false); }
  };
  return <DialogContent>
    <DialogHeader><DialogTitle>New manual claim</DialogTitle><DialogDescription>Patient and insurer portions must equal the total. Figures are proposed from the patient's policy and stay editable.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={save}>
      <FieldGroup label="Patient"><PatientPicker required value={f.patientId} onChange={(patientId) => setF({ ...f, patientId })} /></FieldGroup>
      <Field label="Insurer"><Select required value={f.payerId} onChange={(e) => setF({ ...f, payerId: e.target.value })}><option value="">Select…</option>{payers.filter((p) => p.active).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</Select></Field>
      <Field label="Related invoice (optional)"><Select value={f.invoiceId} onChange={(e) => setF({ ...f, invoiceId: e.target.value })}><option value="">None</option>{invoices.data?.items.filter((i) => !f.patientId || i.patientId === f.patientId).map((i) => <option key={i.id} value={i.id}>{i.invoiceNumber} — {money(i.totalMinor, i.currency)}</option>)}</Select></Field>
      {proposal && (proposal.policyFound
        ? <div className="rounded-md border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-900"><strong>{proposal.policy.payerName}</strong> covers {proposal.policy.coveragePercent}% of this bill.{proposal.alreadyClaimedMinor > 0 && <> {money(proposal.alreadyClaimedMinor, proposal.currency)} has already been claimed, so {money(proposal.claimAmountMinor, proposal.currency)} remains.</>}<div className="mt-1 text-xs">The figures below are filled from the policy and stay editable.</div></div>
        : <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">This patient has no insurance policy on file, so nothing can be proposed. Record their policy on the patient screen, or enter the split by hand.</div>)}
      <Field label="Policy authorization / reference"><Input value={f.authorization} onChange={(e) => setF({ ...f, authorization: e.target.value })} /></Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Member number"><Input value={f.memberNumber} onChange={(e) => setF({ ...f, memberNumber: e.target.value })} /></Field>
        <Field label="Policy number"><Input value={f.policyNumber} onChange={(e) => setF({ ...f, policyNumber: e.target.value })} /></Field>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Claim total"><MoneyInput currency={f.currency} value={f.claimAmountMinor} onValueChange={(v) => setF({ ...f, claimAmountMinor: v })} /></Field>
        <Field label="Patient portion"><MoneyInput currency={f.currency} value={f.patientPortionMinor} onValueChange={(v) => setF({ ...f, patientPortionMinor: v })} /></Field>
        <Field label="Insurer portion"><MoneyInput currency={f.currency} value={f.payerPortionMinor} onValueChange={(v) => setF({ ...f, payerPortionMinor: v })} /></Field>
      </div>
      <Field label="Notes"><Textarea value={f.notes} onChange={(e) => setF({ ...f, notes: e.target.value })} /></Field>
      <DialogFooter><Button disabled={saving || f.claimAmountMinor !== f.patientPortionMinor + f.payerPortionMinor}>{saving ? "Creating…" : "Create claim"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

function ClaimDetail({ id, doctor, onSaved, onClose }: { id: string; doctor: boolean; onSaved(): void; onClose(): void }) {
  const claim = useLoad(() => api.get<InsuranceClaimDetail>(`/insurance/claims/${id}`), [id]);
  const [status, setStatus] = React.useState("");
  const [approvedAmount, setApprovedAmount] = React.useState(0);
  const [amount, setAmount] = React.useState(0);
  const [reference, setReference] = React.useState("");
  const [uploading, setUploading] = React.useState(false);
  React.useEffect(() => { if (claim.data) { setStatus(claim.data.status); setApprovedAmount(claim.data.approvedAmountMinor ?? claim.data.payerPortionMinor); setAmount(claim.data.outstandingMinor); } }, [claim.data]);
  const changeStatus = async () => {
    if (!claim.data) return;
    try {
      const body: Record<string, unknown> = { status, version: claim.data.version };
      if (status === "approved") body.approvedAmountMinor = approvedAmount;
      await api.patch(`/insurance/claims/${id}/status`, body);
      toast.success("Claim status updated"); claim.reload(); onSaved();
    } catch (x) { toast.error(x instanceof APIError ? x.body.message : "Action failed"); }
  };
  const recordPayment = async () => {
    if (!claim.data) return;
    try {
      await api.post(`/insurance/claims/${id}/payments`, { amountMinor: amount, paymentDate: new Date().toISOString().slice(0, 10), reference, notes: "" });
      toast.success("Insurer payment recorded"); claim.reload(); onSaved();
    } catch (x) { toast.error(x instanceof APIError ? x.body.message : "Action failed"); }
  };
  const upload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]; if (!file) return; setUploading(true);
    try {
      const created = await api.post<{ id: string }>("/insurance/documents", { claimId: id, patientId: claim.data?.patientId, documentType: "supporting_document", status: "received" });
      const body = new FormData(); body.set("file", file);
      await api.post(`/insurance/documents/${created.id}/upload`, body);
      toast.success("Document attached"); claim.reload();
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "Upload failed"); }
    finally { setUploading(false); event.target.value = ""; }
  };
  if (claim.loading) return <DialogContent><Skeleton className="h-96" /></DialogContent>;
  if (claim.error || !claim.data) return <DialogContent><ErrorState message={claim.error?.message ?? "Claim unavailable"} /></DialogContent>;
  const value = claim.data;
  return <DialogContent className="max-w-3xl">
    <DialogHeader className="no-print">
      <div className="flex items-center justify-between pr-8">
        <div><DialogTitle>{value.patientName} · {value.payerName}</DialogTitle><DialogDescription>{value.invoiceNumber || "No linked invoice"} · authorization {value.authorization || "not entered"}{value.memberNumber && ` · member ${value.memberNumber}`}{value.policyNumber && ` · policy ${value.policyNumber}`}</DialogDescription></div>
        <Button variant="outline" onClick={triggerPrint}><Printer className="h-4 w-4" />Print / PDF</Button>
      </div>
    </DialogHeader>
    <div className="print-area document-print rounded-lg border p-5">
      <PrintHeader documentTitle="Insurance claim" number={value.id.slice(0, 8).toUpperCase()} date={dateTime(value.createdAt)} />
      <div className="mt-5 grid grid-cols-2 gap-4 text-sm">
        <div><div className="text-xs font-bold uppercase text-zinc-500">Patient</div><div className="mt-1">{value.patientName} · {value.medicalRecordNumber}</div></div>
        <div><div className="text-xs font-bold uppercase text-zinc-500">Insurer</div><div className="mt-1">{value.payerName}</div></div>
        <div><div className="text-xs font-bold uppercase text-zinc-500">Policy / member</div><div className="mt-1">{value.policyNumber || "—"} / {value.memberNumber || "—"}</div></div>
        <div><div className="text-xs font-bold uppercase text-zinc-500">Authorization</div><div className="mt-1">{value.authorization || "—"}</div></div>
        <div><div className="text-xs font-bold uppercase text-zinc-500">Invoice</div><div className="mt-1">{value.invoiceNumber || "—"}</div></div>
        <div><div className="text-xs font-bold uppercase text-zinc-500">Status</div><div className="mt-1 capitalize">{value.status.replaceAll("_", " ")}</div></div>
      </div>
      <div className="mt-5 grid grid-cols-3 gap-3 border-t pt-4 text-sm">
        <div><div className="text-xs text-zinc-500">Claim amount</div><div className="font-mono font-bold">{money(value.claimAmountMinor, value.currency)}</div></div>
        <div><div className="text-xs text-zinc-500">Patient responsibility</div><div className="font-mono font-bold">{money(value.patientPortionMinor, value.currency)}</div></div>
        <div><div className="text-xs text-zinc-500">Insurance responsibility</div><div className="font-mono font-bold">{money(value.payerPortionMinor, value.currency)}</div></div>
      </div>
      {value.approvedAmountMinor !== null && <div className="mt-3 text-sm">Approved: <strong className="font-mono">{money(value.approvedAmountMinor, value.currency)}</strong>{value.partiallyApproved && <Badge tone="warning" className="ml-2">Partially approved</Badge>}</div>}
      <div className="mt-3 text-sm">Paid so far: <strong className="font-mono">{money(value.paidMinor, value.currency)}</strong> · Outstanding: <strong className="font-mono">{money(value.outstandingMinor, value.currency)}</strong></div>
      {value.notes && <div className="mt-4 border-t pt-3 text-sm"><strong>Notes</strong><p className="mt-1 whitespace-pre-wrap">{value.notes}</p></div>}
      <div className="mt-10 flex justify-end"><div className="w-56 border-t pt-2 text-center text-xs text-zinc-500">Authorized signature</div></div>
    </div>
    <Card className="no-print">
      <CardHeader><CardTitle>Supporting documents</CardTitle></CardHeader>
      <CardContent className="grid gap-3">
        {value.documents.length ? value.documents.map((doc) => <div key={doc.id} className="flex items-center justify-between gap-3 rounded-md border p-3 text-sm">
          <div className="min-w-0 flex-1"><div className="truncate font-semibold">{doc.documentType.replaceAll("_", " ")}</div><div className="text-xs text-zinc-500">{doc.displayName || "No file yet"}</div></div>
          <Badge tone={documentStatusTone(doc.status)}>{doc.status.replaceAll("_", " ")}</Badge>
          {doc.hasFile && <Button size="sm" variant="outline" onClick={() => downloadStoredFile(`/insurance/documents/${doc.id}/content?download=true`, doc.displayName || "document")}><Download className="h-3.5 w-3.5" />Download</Button>}
        </div>) : <p className="text-sm text-zinc-500">No supporting documents attached yet.</p>}
        <label className="flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-md border border-dashed p-3 text-sm font-semibold hover:bg-zinc-50"><Upload className="h-4 w-4" />{uploading ? "Uploading…" : "Attach a supporting document"}<input className="sr-only" type="file" accept=".pdf,.jpg,.jpeg,.png,.webp" onChange={upload} disabled={uploading} /></label>
      </CardContent>
    </Card>
    <div className="grid grid-cols-2 gap-3 no-print">
      <Metric label="Payer portion" value={money(value.payerPortionMinor, value.currency)} />
      <Metric label="Outstanding" value={money(value.outstandingMinor, value.currency)} />
    </div>
    {doctor ? <div className="grid gap-5 no-print">
      <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
        <Field label="Workflow status"><Select value={status} onChange={(e) => setStatus(e.target.value)}>{claimStatuses.map((s) => <option key={s}>{s}</option>)}</Select></Field>
        <Button variant="outline" onClick={changeStatus}>Update status</Button>
      </div>
      {status === "approved" && <Field label="Approved amount" hint="Leave equal to the insurer portion for a full approval, or lower for a partial approval."><MoneyInput currency={value.currency} value={approvedAmount} onValueChange={setApprovedAmount} /></Field>}
      {value.outstandingMinor > 0 && (value.status === "approved" || value.status === "partially_paid") && <div className="grid gap-3 border-t pt-5 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
        <Field label="Insurer payment"><MoneyInput currency={value.currency} value={amount} onValueChange={setAmount} /></Field>
        <Field label="Cheque/transfer reference"><Input value={reference} onChange={(e) => setReference(e.target.value)} /></Field>
        <Button disabled={amount <= 0 || amount > value.outstandingMinor} onClick={recordPayment}><WalletCards className="h-4 w-4" />Record</Button>
      </div>}
    </div> : <p className="text-sm text-zinc-500 no-print">A doctor must approve statuses and record insurer remittances.</p>}
    <DialogFooter className="no-print"><Button variant="ghost" onClick={onClose}>Close</Button></DialogFooter>
  </DialogContent>;
}

// ---- Providers ---------------------------------------------------------------

function ProvidersTab() {
  const { revision } = useRealtime();
  const payers = useLoad(() => api.get<{ items: Payer[] }>("/insurance/payers"), [revision]);
  const [creating, setCreating] = React.useState(false);
  const [selected, setSelected] = React.useState<string | null>(null);
  return <>
    <div className="mt-6 flex justify-end"><Button onClick={() => setCreating(true)}><Plus className="h-4 w-4" />Add provider</Button></div>
    <Card className="mt-4 overflow-hidden">
      {payers.loading ? <div className="p-5"><Skeleton className="h-72" /></div> : payers.error ? <div className="p-5"><ErrorState message={payers.error.message} retry={payers.reload} /></div> : payers.data?.items.length ? <div className="grid gap-3 p-3 sm:grid-cols-2 lg:grid-cols-3">
        {payers.data.items.map((payer) => <button key={payer.id} className="rounded-lg border p-4 text-left hover:border-black" onClick={() => setSelected(payer.id)}>
          <div className="flex items-start justify-between gap-2"><div className="font-semibold">{payer.name}</div><Badge tone={payer.active ? "success" : "neutral"}>{payer.active ? "Active" : "Inactive"}</Badge></div>
          <div className="mt-2 text-xs text-zinc-500">{payer.contactName || payer.phone || "No contact on file"}</div>
          <div className="mt-3 font-mono text-sm font-bold">{payer.defaultCoveragePercent}% default coverage</div>
        </button>)}
      </div> : <EmptyState title="No insurance providers" description="Add the insurers this clinic files claims with." action={<Button onClick={() => setCreating(true)}>Add first provider</Button>} />}
    </Card>
    <Dialog open={creating} onOpenChange={setCreating}><PayerForm onSaved={() => { setCreating(false); payers.reload(); }} /></Dialog>
    <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>{selected && <PayerDetail id={selected} onClose={() => setSelected(null)} onChanged={payers.reload} />}</Dialog>
  </>;
}

const emptyPayer = { name: "", contactName: "", phone: "", email: "", address: "", website: "", notes: "", acceptedCoverage: "", billingInfo: "", claimInstructions: "", defaultCoveragePercent: 0, active: true };

function PayerForm({ payer, onSaved }: { payer?: Payer; onSaved(): void }) {
  const [f, setF] = React.useState(payer ? { ...emptyPayer, ...payer } : emptyPayer);
  const [saving, setSaving] = React.useState(false);
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setSaving(true);
    try {
      if (payer) await api.put(`/insurance/payers/${payer.id}`, { ...f, version: payer.version });
      else await api.post("/insurance/payers", f);
      toast.success(payer ? "Provider updated" : "Provider created"); onSaved();
    } catch (x) { toast.error(x instanceof APIError ? x.body.message : "Could not save the provider"); }
    finally { setSaving(false); }
  };
  return <DialogContent className="max-w-2xl">
    <DialogHeader><DialogTitle>{payer ? "Edit provider" : "Add insurance provider"}</DialogTitle><DialogDescription>What staff need to know before submitting a claim to this insurer.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={save}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Company name"><Input required value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Contact person"><Input value={f.contactName} onChange={(e) => setF({ ...f, contactName: e.target.value })} /></Field>
        <Field label="Phone"><Input value={f.phone} onChange={(e) => setF({ ...f, phone: e.target.value })} /></Field>
        <Field label="Email"><Input type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} /></Field>
        <Field label="Website"><Input value={f.website} onChange={(e) => setF({ ...f, website: e.target.value })} /></Field>
        <Field label="Default coverage (%)" hint="Used to propose a claim split when a policy has none of its own."><Input type="number" min={0} max={100} value={f.defaultCoveragePercent} onChange={(e) => setF({ ...f, defaultCoveragePercent: Number(e.target.value) })} /></Field>
      </div>
      <Field label="Address"><Textarea value={f.address} onChange={(e) => setF({ ...f, address: e.target.value })} /></Field>
      <Field label="Accepted coverage / services"><Textarea value={f.acceptedCoverage} onChange={(e) => setF({ ...f, acceptedCoverage: e.target.value })} /></Field>
      <Field label="Billing information"><Textarea value={f.billingInfo} onChange={(e) => setF({ ...f, billingInfo: e.target.value })} /></Field>
      <Field label="Claim instructions"><Textarea value={f.claimInstructions} onChange={(e) => setF({ ...f, claimInstructions: e.target.value })} /></Field>
      <Field label="Notes"><Textarea value={f.notes} onChange={(e) => setF({ ...f, notes: e.target.value })} /></Field>
      {payer && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={f.active} onChange={(e) => setF({ ...f, active: e.target.checked })} />Active</label>}
      <DialogFooter><Button disabled={saving}><ShieldCheck className="h-4 w-4" />{saving ? "Saving…" : "Save provider"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

function PayerDetail({ id, onClose, onChanged }: { id: string; onClose(): void; onChanged(): void }) {
  const payer = useLoad(() => api.get<Payer>(`/insurance/payers/${id}`), [id]);
  const documents = useLoad(() => api.get<{ items: InsuranceDocument[] }>(`/insurance/documents?payerId=${id}`), [id]);
  const [editing, setEditing] = React.useState(false);
  const [addingForm, setAddingForm] = React.useState(false);
  if (payer.loading) return <DialogContent><Skeleton className="h-80" /></DialogContent>;
  if (payer.error || !payer.data) return <DialogContent><ErrorState message={payer.error?.message ?? "Provider unavailable"} /></DialogContent>;
  const value = payer.data;
  return <DialogContent className="max-w-2xl">
    <DialogHeader><div className="flex items-center justify-between pr-8"><DialogTitle>{value.name}</DialogTitle><Button size="sm" variant="outline" onClick={() => setEditing(true)}>Edit</Button></div><DialogDescription>{value.website && <a className="underline" href={value.website} target="_blank" rel="noreferrer"><Globe className="mr-1 inline h-3 w-3" />{value.website}</a>}</DialogDescription></DialogHeader>
    <div className="grid gap-4">
      <ImageGallery entityType="payer" entityId={value.id} canEdit emptyHint="No logo or photos yet." />
      <div className="grid gap-3 sm:grid-cols-2 text-sm">
        <Info label="Contact" value={value.contactName} /><Info label="Phone" value={value.phone} /><Info label="Email" value={value.email} /><Info label="Default coverage" value={`${value.defaultCoveragePercent}%`} />
      </div>
      {value.acceptedCoverage && <Info label="Accepted coverage / services" value={value.acceptedCoverage} block />}
      {value.billingInfo && <Info label="Billing information" value={value.billingInfo} block />}
      {value.claimInstructions && <Info label="Claim instructions" value={value.claimInstructions} block />}
      {value.notes && <Info label="Notes" value={value.notes} block />}
      <Card>
        <CardHeader className="flex-row items-center justify-between"><CardTitle>Required forms</CardTitle><Button size="sm" variant="outline" onClick={() => setAddingForm(true)}><Plus className="h-3.5 w-3.5" />Add required form</Button></CardHeader>
        <CardContent className="grid gap-2">
          {documents.data?.items.length ? documents.data.items.map((doc) => <RequiredFormRow key={doc.id} document={doc} onChanged={documents.reload} />) : <p className="text-sm text-zinc-500">No forms recorded for this provider yet.</p>}
        </CardContent>
      </Card>
    </div>
    <Dialog open={editing} onOpenChange={setEditing}><PayerForm payer={value} onSaved={() => { setEditing(false); payer.reload(); onChanged(); }} /></Dialog>
    <Dialog open={addingForm} onOpenChange={setAddingForm}><RequiredFormForm payerId={value.id} onSaved={() => { setAddingForm(false); documents.reload(); }} /></Dialog>
    <DialogFooter><Button variant="ghost" onClick={onClose}>Close</Button></DialogFooter>
  </DialogContent>;
}

function Info({ label, value, block }: { label: string; value?: string; block?: boolean }) {
  return <div className={block ? "" : undefined}><div className="text-xs font-semibold text-zinc-500">{label}</div><div className={`mt-1 ${block ? "whitespace-pre-wrap" : ""}`}>{value || "—"}</div></div>;
}

function RequiredFormForm({ payerId, onSaved }: { payerId: string; onSaved(): void }) {
  const [documentType, setDocumentType] = React.useState("");
  const [notes, setNotes] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setSaving(true);
    try { await api.post("/insurance/documents", { payerId, documentType, notes, status: "required" }); toast.success("Required form recorded"); onSaved(); }
    catch (x) { toast.error(x instanceof APIError ? x.body.message : "Could not record the form"); }
    finally { setSaving(false); }
  };
  return <DialogContent><DialogHeader><DialogTitle>Add required form</DialogTitle><DialogDescription>Record a form this insurer requires before it can be uploaded.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={save}>
      <Field label="Form / document type"><Input required placeholder="e.g. Claim form, referral, authorization" value={documentType} onChange={(e) => setDocumentType(e.target.value)} /></Field>
      <Field label="Notes"><Textarea value={notes} onChange={(e) => setNotes(e.target.value)} /></Field>
      <DialogFooter><Button disabled={saving}>{saving ? "Saving…" : "Record requirement"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

function RequiredFormRow({ document, onChanged }: { document: InsuranceDocument; onChanged(): void }) {
  const [uploading, setUploading] = React.useState(false);
  const upload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]; if (!file) return; setUploading(true);
    try { const body = new FormData(); body.set("file", file); await api.post(`/insurance/documents/${document.id}/upload`, body); toast.success("Form uploaded"); onChanged(); }
    catch (reason) { toast.error(reason instanceof Error ? reason.message : "Upload failed"); }
    finally { setUploading(false); event.target.value = ""; }
  };
  const archive = async () => {
    if (!window.confirm(`Remove the "${document.documentType}" requirement?`)) return;
    try { await api.delete(`/insurance/documents/${document.id}?version=${document.version}`); toast.success("Requirement removed"); onChanged(); }
    catch (reason) { toast.error(reason instanceof Error ? reason.message : "Could not remove the requirement"); }
  };
  return <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3 text-sm">
    <div className="min-w-0 flex-1"><div className="truncate font-semibold">{document.documentType.replaceAll("_", " ")}</div>{document.notes && <div className="text-xs text-zinc-500">{document.notes}</div>}</div>
    <Badge tone={documentStatusTone(document.status)}>{document.status.replaceAll("_", " ")}</Badge>
    <div className="flex gap-2">
      {document.hasFile ? <Button size="sm" variant="outline" onClick={() => downloadStoredFile(`/insurance/documents/${document.id}/content?download=true`, document.displayName || document.documentType)}><Download className="h-3.5 w-3.5" />Download</Button>
        : <label className="flex cursor-pointer items-center gap-1 rounded-md border px-3 py-1.5 text-xs font-semibold hover:bg-zinc-50"><Upload className="h-3.5 w-3.5" />{uploading ? "Uploading…" : "Upload"}<input className="sr-only" type="file" accept=".pdf,.jpg,.jpeg,.png,.webp" onChange={upload} disabled={uploading} /></label>}
      <Button size="sm" variant="ghost" onClick={archive}><FileText className="h-3.5 w-3.5" />Remove</Button>
    </div>
  </div>;
}
