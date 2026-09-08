import * as React from "react";
import { Building2, Plus, ShieldCheck, WalletCards } from "lucide-react";
import { toast } from "sonner";
import { api } from "../api";
import { useAuth } from "../auth";
import { useLoad } from "../hooks";
import { money } from "../lib";
import { useRealtime } from "../realtime";
import type { Invoice } from "../types";
import { Button } from "../components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../components/ui/dialog";
import {
  Badge,
  EmptyState,
  ErrorState,
  Skeleton,
  Table,
  Td,
  Th,
} from "../components/ui/data";
import { Field, Input, Select, Textarea } from "../components/ui/input";
import { PatientPicker } from "../components/patient-search";

interface Payer {
  id: string;
  name: string;
  contactName: string;
  phone: string;
  email: string;
  address: string;
  active: boolean;
  version: number;
}
interface Claim {
  id: string;
  patientName: string;
  patientId: string;
  medicalRecordNumber: string;
  payerName: string;
  payerId: string;
  invoiceNumber: string;
  invoiceId: string;
  authorization: string;
  memberNumber: string;
  policyNumber: string;
  currency: string;
  exchangeRate: string;
  claimAmountMinor: number;
  patientPortionMinor: number;
  payerPortionMinor: number;
  paidMinor: number;
  outstandingMinor: number;
  status: string;
  version: number;
  agingBucket: string;
}
const statuses = [
  "draft",
  "submitted",
  "pending",
  "approved",
  "partially_paid",
  "paid",
  "rejected",
  "cancelled",
];

export function InsurancePage() {
  const { revision } = useRealtime();
  const { user } = useAuth();
  const doctor = user?.role === "doctor";
  const payers = useLoad(
    () => api.get<{ items: Payer[] }>("/insurance/payers"),
    [revision],
  );
  const claims = useLoad(
    () => api.get<{ items: Claim[] }>("/insurance/claims"),
    [revision],
  );
  const [newPayer, setNewPayer] = React.useState(false);
  const [newClaim, setNewClaim] = React.useState(false);
  const [selected, setSelected] = React.useState<Claim | null>(null);
  const reload = () => {
    payers.reload();
    claims.reload();
  };
  return (
    <div className="page">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="section-title">Third-party coverage</p>
          <h1 className="page-title">Insurance</h1>
          <p className="page-description">
            Manual local workflow for authorizations, claims, insurer payments
            and receivables.
          </p>
        </div>
        <div className="flex gap-2">
          {doctor && (
            <Button variant="outline" onClick={() => setNewPayer(true)}>
              <Building2 className="h-4 w-4" />
              Add insurer
            </Button>
          )}
          <Button onClick={() => setNewClaim(true)}>
            <Plus className="h-4 w-4" />
            New claim
          </Button>
        </div>
      </div>
      <div className="mt-6 grid gap-4 sm:grid-cols-3">
        <Metric
          label="Open claims"
          value={
            claims.data?.items.filter(
              (x) => !["paid", "rejected", "cancelled"].includes(x.status),
            ).length ?? 0
          }
        />
        <Metric
          label="Outstanding claims"
          value={
            claims.data?.items.filter((x) => x.outstandingMinor > 0).length ?? 0
          }
        />
        <Metric
          label="Over 90 days"
          value={
            claims.data?.items.filter(
              (x) => x.agingBucket === "90+" && x.outstandingMinor > 0,
            ).length ?? 0
          }
        />
      </div>
      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Claims and receivables</CardTitle>
          <CardDescription>
            Amounts are entered from paper/email remittances and remain
            available offline.
          </CardDescription>
        </CardHeader>
        {claims.loading ? (
          <div className="p-5">
            <Skeleton className="h-72" />
          </div>
        ) : claims.error ? (
          <div className="p-5">
            <ErrorState message={claims.error.message} retry={claims.reload} />
          </div>
        ) : claims.data?.items.length ? (
          <>
            <div className="hidden overflow-x-auto md:block">
              <Table>
                <thead>
                  <tr>
                    <Th>Patient</Th>
                    <Th>Insurer</Th>
                    <Th>Authorization</Th>
                    <Th>Payer portion</Th>
                    <Th>Paid</Th>
                    <Th>Outstanding</Th>
                    <Th>Aging</Th>
                    <Th>Status</Th>
                  </tr>
                </thead>
                <tbody>
                  {claims.data.items.map((c) => (
                    <tr
                      key={c.id}
                      className="cursor-pointer hover:bg-zinc-50"
                      onClick={() => setSelected(c)}
                    >
                      <Td>
                        <b>{c.patientName}</b>
                        <div className="font-mono text-xs text-zinc-500">
                          {c.medicalRecordNumber}
                        </div>
                      </Td>
                      <Td>{c.payerName}</Td>
                      <Td className="font-mono text-xs">
                        {c.authorization || "—"}
                      </Td>
                      <Td className="font-mono">
                        {money(c.payerPortionMinor, c.currency)}
                      </Td>
                      <Td className="font-mono">
                        {money(c.paidMinor, c.currency)}
                      </Td>
                      <Td className="font-mono font-bold">
                        {money(c.outstandingMinor, c.currency)}
                      </Td>
                      <Td>{c.agingBucket}</Td>
                      <Td>
                        <Badge
                          tone={
                            c.status === "paid"
                              ? "success"
                              : c.status === "rejected"
                                ? "danger"
                                : "warning"
                          }
                        >
                          {c.status.replaceAll("_", " ")}
                        </Badge>
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </div>
            <div className="divide-y md:hidden">
              {claims.data.items.map((c) => (
                <button
                  className="w-full p-4 text-left"
                  key={c.id}
                  onClick={() => setSelected(c)}
                >
                  <div className="flex justify-between gap-2">
                    <b>{c.patientName}</b>
                    <Badge>{c.status}</Badge>
                  </div>
                  <div className="mt-1 text-sm text-zinc-500">
                    {c.payerName} · {c.authorization || "No authorization"}
                  </div>
                  <div className="mt-3 flex justify-between">
                    <span className="text-sm">
                      Outstanding · {c.agingBucket} days
                    </span>
                    <b className="font-mono">
                      {money(c.outstandingMinor, c.currency)}
                    </b>
                  </div>
                </button>
              ))}
            </div>
          </>
        ) : (
          <EmptyState
            title="No insurance claims"
            description="Create a claim when a covered patient is billed."
          />
        )}
      </Card>
      <Dialog open={newPayer} onOpenChange={setNewPayer}>
        <PayerForm
          onSaved={() => {
            setNewPayer(false);
            reload();
          }}
        />
      </Dialog>
      <Dialog open={newClaim} onOpenChange={setNewClaim}>
        <ClaimForm
          payers={payers.data?.items ?? []}
          onSaved={() => {
            setNewClaim(false);
            reload();
          }}
        />
      </Dialog>
      <Dialog open={!!selected} onOpenChange={(v) => !v && setSelected(null)}>
        {selected && (
          <ClaimAction
            claim={selected}
            doctor={doctor}
            onSaved={() => {
              setSelected(null);
              reload();
            }}
          />
        )}
      </Dialog>
    </div>
  );
}
function Metric({ label, value }: { label: string; value: string | number }) {
  return (
    <Card>
      <CardContent className="p-5">
        <div className="text-xs font-bold uppercase text-zinc-500">{label}</div>
        <div className="mt-2 font-mono text-2xl font-bold">{value}</div>
      </CardContent>
    </Card>
  );
}
function PayerForm({ onSaved }: { onSaved(): void }) {
  const [f, setF] = React.useState({
    name: "",
    contactName: "",
    phone: "",
    email: "",
    address: "",
  });
  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.post("/insurance/payers", f);
      toast.success("Insurer created");
      onSaved();
    } catch (x) {
      toast.error(x instanceof Error ? x.message : "Could not create insurer");
    }
  };
  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Add insurer</DialogTitle>
        <DialogDescription>
          Contact information used for manual claim follow-up.
        </DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={save}>
        <Field label="Company name">
          <Input
            required
            value={f.name}
            onChange={(e) => setF({ ...f, name: e.target.value })}
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Contact">
            <Input
              value={f.contactName}
              onChange={(e) => setF({ ...f, contactName: e.target.value })}
            />
          </Field>
          <Field label="Phone">
            <Input
              value={f.phone}
              onChange={(e) => setF({ ...f, phone: e.target.value })}
            />
          </Field>
          <Field label="Email">
            <Input
              type="email"
              value={f.email}
              onChange={(e) => setF({ ...f, email: e.target.value })}
            />
          </Field>
        </div>
        <Field label="Address">
          <Textarea
            value={f.address}
            onChange={(e) => setF({ ...f, address: e.target.value })}
          />
        </Field>
        <DialogFooter>
          <Button>
            <ShieldCheck className="h-4 w-4" />
            Save insurer
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}
function ClaimForm({ payers, onSaved }: { payers: Payer[]; onSaved(): void }) {
  const invoices = useLoad(() => api.get<{ items: Invoice[] }>("/invoices"));
  const [f, setF] = React.useState({
    patientId: "",
    payerId: "",
    invoiceId: "",
    authorization: "",
    memberNumber: "",
    policyNumber: "",
    currency: "HTG",
    exchangeRate: "1",
    claimAmountMinor: 0,
    patientPortionMinor: 0,
    payerPortionMinor: 0,
  });
  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.post("/insurance/claims", f);
      toast.success("Insurance claim created");
      onSaved();
    } catch (x) {
      toast.error(x instanceof Error ? x.message : "Could not create claim");
    }
  };
  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>New manual claim</DialogTitle>
        <DialogDescription>
          Patient and insurer portions must equal the total.
        </DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={save}>
        <Field label="Patient">
          <PatientPicker
            required
            value={f.patientId}
            onChange={(patientId) => setF({ ...f, patientId })}
          />
        </Field>
        <Field label="Insurer">
          <Select
            required
            value={f.payerId}
            onChange={(e) => setF({ ...f, payerId: e.target.value })}
          >
            <option value="">Select…</option>
            {payers
              .filter((p) => p.active)
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
          </Select>
        </Field>
        <Field label="Related invoice (optional)">
          <Select
            value={f.invoiceId}
            onChange={(e) => setF({ ...f, invoiceId: e.target.value })}
          >
            <option value="">None</option>
            {invoices.data?.items
              .filter((i) => !f.patientId || i.patientId === f.patientId)
              .map((i) => (
                <option key={i.id} value={i.id}>
                  {i.invoiceNumber} — {money(i.totalMinor, i.currency)}
                </option>
              ))}
          </Select>
        </Field>
        <Field label="Policy authorization / reference">
          <Input
            value={f.authorization}
            onChange={(e) => setF({ ...f, authorization: e.target.value })}
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Member number">
            <Input
              value={f.memberNumber}
              onChange={(e) => setF({ ...f, memberNumber: e.target.value })}
            />
          </Field>
          <Field label="Policy number">
            <Input
              value={f.policyNumber}
              onChange={(e) => setF({ ...f, policyNumber: e.target.value })}
            />
          </Field>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Currency">
            <Select
              value={f.currency}
              onChange={(e) =>
                setF({
                  ...f,
                  currency: e.target.value,
                  exchangeRate: e.target.value === "HTG" ? "1" : f.exchangeRate,
                })
              }
            >
              <option value="HTG">HTG</option>
              <option value="USD">USD</option>
            </Select>
          </Field>
          <Field label="Historical exchange rate">
            <Input
              value={f.exchangeRate}
              onChange={(e) => setF({ ...f, exchangeRate: e.target.value })}
            />
          </Field>
        </div>
        <div className="grid gap-4 sm:grid-cols-3">
          <MoneyInput
            label="Claim total"
            value={f.claimAmountMinor}
            set={(v) => setF({ ...f, claimAmountMinor: v })}
          />
          <MoneyInput
            label="Patient portion"
            value={f.patientPortionMinor}
            set={(v) => setF({ ...f, patientPortionMinor: v })}
          />
          <MoneyInput
            label="Insurer portion"
            value={f.payerPortionMinor}
            set={(v) => setF({ ...f, payerPortionMinor: v })}
          />
        </div>
        <DialogFooter>
          <Button>Create claim</Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}
function MoneyInput({
  label,
  value,
  set,
}: {
  label: string;
  value: number;
  set(v: number): void;
}) {
  return (
    <Field label={`${label} (minor units)`}>
      <Input
        type="number"
        min={0}
        value={value}
        onChange={(e) => set(Number(e.target.value))}
      />
    </Field>
  );
}
function ClaimAction({
  claim,
  doctor,
  onSaved,
}: {
  claim: Claim;
  doctor: boolean;
  onSaved(): void;
}) {
  const [status, setStatus] = React.useState(claim.status);
  const [amount, setAmount] = React.useState(claim.outstandingMinor);
  const [reference, setReference] = React.useState("");
  const act = async (kind: "status" | "payment") => {
    try {
      if (kind === "status")
        await api.patch(`/insurance/claims/${claim.id}/status`, {
          status,
          version: claim.version,
        });
      else
        await api.post(`/insurance/claims/${claim.id}/payments`, {
          amountMinor: amount,
          paymentDate: new Date().toISOString().slice(0, 10),
          reference,
          notes: "",
        });
      toast.success(
        kind === "status" ? "Claim status updated" : "Insurer payment recorded",
      );
      onSaved();
    } catch (x) {
      toast.error(x instanceof Error ? x.message : "Action failed");
    }
  };
  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>
          {claim.patientName} · {claim.payerName}
        </DialogTitle>
        <DialogDescription>
          {claim.invoiceNumber || "No linked invoice"} · authorization{" "}
          {claim.authorization || "not entered"}
          {claim.memberNumber && ` · member ${claim.memberNumber}`}
          {claim.policyNumber && ` · policy ${claim.policyNumber}`}
        </DialogDescription>
      </DialogHeader>
      <div className="grid grid-cols-2 gap-3">
        <Metric
          label="Payer portion"
          value={money(claim.payerPortionMinor, claim.currency)}
        />
        <Metric
          label="Outstanding"
          value={money(claim.outstandingMinor, claim.currency)}
        />
      </div>
      {doctor ? (
        <div className="grid gap-5">
          <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
            <Field label="Workflow status">
              <Select
                value={status}
                onChange={(e) => setStatus(e.target.value)}
              >
                {statuses.map((s) => (
                  <option key={s}>{s}</option>
                ))}
              </Select>
            </Field>
            <Button variant="outline" onClick={() => act("status")}>
              Update status
            </Button>
          </div>
          {claim.outstandingMinor > 0 && (
            <div className="grid gap-3 border-t pt-5 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
              <MoneyInput
                label="Insurer payment"
                value={amount}
                set={setAmount}
              />
              <Field label="Cheque/transfer reference">
                <Input
                  value={reference}
                  onChange={(e) => setReference(e.target.value)}
                />
              </Field>
              <Button
                disabled={amount <= 0 || amount > claim.outstandingMinor}
                onClick={() => act("payment")}
              >
                <WalletCards className="h-4 w-4" />
                Record
              </Button>
            </div>
          )}
        </div>
      ) : (
        <p className="text-sm text-zinc-500">
          A doctor must approve statuses and record insurer remittances.
        </p>
      )}
    </DialogContent>
  );
}
