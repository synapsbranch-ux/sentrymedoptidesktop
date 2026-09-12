import * as React from "react";
import {
  Banknote,
  CreditCard,
  FileText,
  Printer,
  Receipt,
  Wallet,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "../api";
import { useAuth } from "../auth";
import { useLoad, usePagedList } from "../hooks";
import { dateTime, money } from "../lib";
import { useRealtime } from "../realtime";
import { PrintHeader, triggerPrint } from "../components/print";
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
  Pager,
  Skeleton,
  Table,
  Td,
  Th,
} from "../components/ui/data";
import { Field, Input, Select, Textarea } from "../components/ui/input";
import { MoneyInput } from "../components/ui/money-input";
import type { InvoiceInsurance } from "../types";

interface InvoiceDetail extends Invoice, InvoiceInsurance {
  notes: string;
  items: {
    id: string;
    inventoryItemId: string;
    description: string;
    quantity: number;
    unitPriceMinor: number;
    lineTotalMinor: number;
  }[];
  payments: {
    id: string;
    receiptNumber: string;
    amountMinor: number;
    refundedMinor: number;
    currency: string;
    paymentMethod: string;
    receivedAt: string;
    receivedBy: string;
  }[];
  creditNotes: {
    creditNumber: string;
    amountMinor: number;
    reason: string;
    restocked: boolean;
    createdAt: string;
  }[];
}
export function BillingPage() {
  const { revision } = useRealtime();
  const [selectedID, setSelectedID] = React.useState<string | null>(null);
  const [registerOpen, setRegisterOpen] = React.useState(false);
  const invoices = usePagedList<Invoice>((page, limit) => `/invoices?page=${page}&limit=${limit}`, [revision]);
  const register = useLoad(
    () =>
      api.get<{
        open: boolean;
        id?: string;
        currency?: string;
        openingFloatMinor?: number;
        openedAt?: string;
        openedBy?: string;
      }>("/cash-register"),
    [revision],
  );
  React.useEffect(() => {
    const id = new URLSearchParams(location.search).get("id");
    if (id) setSelectedID(id);
  }, []);
  return (
    <div className="page">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="section-title">Invoices, receipts & register</p>
          <h1 className="page-title">Billing</h1>
          <p className="page-description">
            Payments are immutable records; balances are always derived.
          </p>
        </div>
        <div className="flex gap-2">
          {register.data?.open && register.data.id && <RegisterReportButton sessionId={register.data.id} />}
          <Button variant="outline" onClick={() => setRegisterOpen(true)}>
            <Wallet className="h-4 w-4" />
            {register.data?.open ? "Close register" : "Open register"}
          </Button>
        </div>
      </div>
      <SalesHistory />
      <Card className="mt-6 overflow-hidden">
        <CardHeader>
          <CardTitle>Invoices</CardTitle>
          <CardDescription>
            Issued, partial, paid and outstanding clinic balances
          </CardDescription>
        </CardHeader>
        {invoices.loading ? (
          <div className="p-5">
            <Skeleton className="h-72" />
          </div>
        ) : invoices.error ? (
          <div className="p-5">
            <ErrorState
              message={invoices.error.message}
              retry={invoices.reload}
            />
          </div>
        ) : invoices.items.length ? (
          <>
            <div className="hidden md:block">
              <Table>
                <thead>
                  <tr>
                    <Th>Invoice</Th>
                    <Th>Patient</Th>
                    <Th>Total</Th>
                    <Th>Paid</Th>
                    <Th>Balance</Th>
                    <Th>Status</Th>
                    <Th>Date</Th>
                  </tr>
                </thead>
                <tbody>
                  {invoices.items.map((item) => (
                    <tr
                      key={item.id}
                      onClick={() => setSelectedID(item.id)}
                      className="cursor-pointer hover:bg-zinc-50"
                    >
                      <Td className="font-mono text-xs font-bold">
                        {item.invoiceNumber}
                      </Td>
                      <Td>{item.patientName}</Td>
                      <Td className="font-mono font-bold">
                        {money(item.totalMinor, item.currency)}
                      </Td>
                      <Td className="font-mono">
                        {money(item.paidMinor, item.currency)}
                      </Td>
                      <Td className="font-mono">
                        {money(item.balanceMinor, item.currency)}
                      </Td>
                      <Td>
                        <Badge
                          tone={
                            item.status === "paid"
                              ? "success"
                              : item.status === "overdue"
                                ? "danger"
                                : "warning"
                          }
                        >
                          {item.status.replaceAll("_", " ")}
                        </Badge>
                      </Td>
                      <Td className="text-xs text-zinc-500">
                        {dateTime(item.createdAt)}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </div>
            <div className="divide-y md:hidden">
              {invoices.items.map((item) => (
                <button
                  key={item.id}
                  onClick={() => setSelectedID(item.id)}
                  className="w-full p-4 text-left"
                >
                  <div className="flex justify-between">
                    <span className="font-mono text-xs font-bold">
                      {item.invoiceNumber}
                    </span>
                    <Badge
                      tone={item.status === "paid" ? "success" : "warning"}
                    >
                      {item.status}
                    </Badge>
                  </div>
                  <div className="mt-2 font-semibold">{item.patientName}</div>
                  <div className="mt-1 flex justify-between text-sm">
                    <span className="text-zinc-500">Balance</span>
                    <span className="font-mono font-bold">
                      {money(item.balanceMinor, item.currency)}
                    </span>
                  </div>
                </button>
              ))}
            </div>
            <Pager page={invoices.page} pageSize={invoices.pageSize} total={invoices.total} hasMore={invoices.hasMore} onPrevious={invoices.previous} onNext={invoices.next} />
          </>
        ) : (
          <EmptyState
            title="No invoices"
            description="POS sales and manually issued invoices will appear here."
          />
        )}
      </Card>
      <Dialog
        open={Boolean(selectedID)}
        onOpenChange={(open) => !open && setSelectedID(null)}
      >
        {selectedID && (
          <InvoiceView
            id={selectedID}
            onChanged={() => {
              invoices.reload();
            }}
          />
        )}
      </Dialog>
      <Dialog open={registerOpen} onOpenChange={setRegisterOpen}>
        <RegisterForm
          current={register.data ?? { open: false }}
          onSaved={() => {
            setRegisterOpen(false);
            register.reload();
          }}
        />
      </Dialog>
    </div>
  );
}

function InvoiceView({ id, onChanged }: { id: string; onChanged(): void }) {
  const { user } = useAuth();
  const detail = useLoad(() => api.get<InvoiceDetail>(`/invoices/${id}`), [id]);
  const methods = useLoad(() =>
    api.get<{ items: { id: string; name: string }[] }>("/payment-methods"),
  );
  const register = useLoad(() =>
    api.get<{ open: boolean; id?: string }>("/cash-register"),
  );
  const [paying, setPaying] = React.useState(false);
  const [method, setMethod] = React.useState("pm_cash");
  const [amount, setAmount] = React.useState(0);
  const [refundPayment, setRefundPayment] = React.useState<
    InvoiceDetail["payments"][number] | null
  >(null);
  const [refundAmount, setRefundAmount] = React.useState(0);
  const [refundReason, setRefundReason] = React.useState("");
  const [restock, setRestock] = React.useState<string[]>([]);
  const [printingReceipt, setPrintingReceipt] = React.useState(false);
  const printThermalReceipt = async () => { setPrintingReceipt(true); try { await api.post(`/printers/default/print-invoice/${id}`, {}); toast.success("Thermal receipt printed"); } catch(reason) { toast.error(reason instanceof Error ? reason.message : "Thermal printer is unavailable"); } finally { setPrintingReceipt(false); } };
  const refund = async () => {
    if (!refundPayment) return;
    try {
      const result = await api.post<{ creditNumber: string }>(
        `/payments/${refundPayment.id}/refunds`,
        {
          amountMinor: refundAmount,
          reason: refundReason,
          restockItemIds: restock,
        },
      );
      toast.success(`Refund recorded · credit note ${result.creditNumber}`);
      setRefundPayment(null);
      setRefundReason("");
      setRestock([]);
      detail.reload();
      onChanged();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Refund failed");
    }
  };
  React.useEffect(() => {
    if (detail.data) setAmount(detail.data.balanceMinor);
  }, [detail.data]);
  const pay = async () => {
    if (!detail.data) return;
    setPaying(true);
    try {
      await api.post(`/invoices/${id}/payments`, {
        paymentMethodId: method,
        registerSessionId: register.data?.id ?? "",
        amountMinor: amount,
        currency: detail.data.currency,
        exchangeRate: detail.data.exchangeRate,
        reference: "",
        notes: "",
      });
      toast.success("Payment recorded and receipt created");
      detail.reload();
      onChanged();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Payment failed");
    } finally {
      setPaying(false);
    }
  };
  if (detail.loading)
    return (
      <DialogContent>
        <Skeleton className="h-96" />
      </DialogContent>
    );
  if (!detail.data || detail.error)
    return (
      <DialogContent>
        <ErrorState message={detail.error?.message ?? "Invoice not found"} />
      </DialogContent>
    );
  const invoice = detail.data;
  return (
    <DialogContent className="max-w-4xl">
      <DialogHeader className="no-print">
        <div className="flex items-center justify-between pr-8">
          <div>
            <div className="font-mono text-xs font-bold text-zinc-500">
              {invoice.invoiceNumber}
            </div>
            <DialogTitle>Invoice for {invoice.patientName}</DialogTitle>
          </div>
          <div className="flex flex-wrap gap-2"><Button disabled={printingReceipt} onClick={printThermalReceipt}><Receipt className="h-4 w-4" />{printingReceipt ? "Printing…" : "Print receipt"}</Button><Button variant="outline" onClick={triggerPrint}><Printer className="h-4 w-4" />Print / PDF</Button></div>
        </div>
      </DialogHeader>
      <div className="print-area document-print rounded-lg border p-5">
        <PrintHeader
          documentTitle="Invoice"
          number={invoice.invoiceNumber}
          date={dateTime(invoice.createdAt)}
        />
        <div className="mt-5 flex items-start justify-between">
          <div>
            <div className="text-xs font-bold uppercase text-zinc-500">
              Patient
            </div>
            <div className="mt-1 font-semibold">{invoice.patientName}</div>
          </div>
          <Badge tone={invoice.status === "paid" ? "success" : "warning"}>
            {invoice.status.replaceAll("_", " ")}
          </Badge>
        </div>
        <div className="mt-6 divide-y border-y">
          {invoice.items.map((item) => (
            <div
              key={item.id}
              className="grid grid-cols-[1fr_auto] gap-4 py-3 text-sm"
            >
              <div>
                <strong>{item.description}</strong>
                <div className="text-xs text-zinc-500">
                  {item.quantity} ×{" "}
                  {money(item.unitPriceMinor, invoice.currency)}
                </div>
              </div>
              <div className="font-mono font-bold">
                {money(item.lineTotalMinor, invoice.currency)}
              </div>
            </div>
          ))}
        </div>
        <div className="ml-auto mt-5 grid max-w-xs gap-2 text-sm">
          <div className="flex justify-between">
            <span>Total</span>
            <strong className="font-mono">
              {money(invoice.totalMinor, invoice.currency)}
            </strong>
          </div>
          <div className="flex justify-between">
            <span>Paid</span>
            <span className="font-mono">
              {money(invoice.paidMinor, invoice.currency)}
            </span>
          </div>
          <div className="flex justify-between border-t pt-2 text-lg">
            <strong>Balance</strong>
            <strong className="font-mono">
              {money(invoice.balanceMinor, invoice.currency)}
            </strong>
          </div>
        </div>
        {invoice.insuranceClaims.length > 0 && (
          <div className="mt-4 grid gap-3 rounded-lg border bg-zinc-50 p-4 sm:grid-cols-2">
            <div>
              <div className="text-xs font-bold uppercase text-zinc-500">
                Patient responsibility
              </div>
              <div className="mt-1 font-mono text-lg font-bold">
                {money(invoice.patientResponsibilityMinor, invoice.currency)}
              </div>
            </div>
            <div>
              <div className="text-xs font-bold uppercase text-zinc-500">
                Insurance
              </div>
              <div className="mt-1 flex items-baseline gap-2 font-mono text-lg font-bold">
                {money(invoice.insuranceReceivedMinor, invoice.currency)}
                <span className="text-xs font-normal text-zinc-500">
                  received of {money(invoice.insuranceExpectedMinor, invoice.currency)} expected
                </span>
              </div>
            </div>
            <div className="sm:col-span-2 space-y-1">
              {invoice.insuranceClaims.map((claim) => (
                <div key={claim.id} className="flex items-center justify-between text-xs text-zinc-600">
                  <span>{claim.payerName} · {claim.status.replaceAll("_", " ")}</span>
                  <span className="font-mono">
                    {money(claim.paidMinor, invoice.currency)} / {money(claim.payerPortionMinor, invoice.currency)}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
      {invoice.payments.length > 0 && (
        <Card className="mt-4 no-print">
          <CardHeader>
            <CardTitle>Payments & receipts</CardTitle>
          </CardHeader>
          <CardContent className="divide-y">
            {invoice.payments.map((payment) => (
              <div className="flex items-center gap-3 py-3" key={payment.id}>
                <Receipt className="h-4 w-4 text-zinc-400" />
                <div className="flex-1">
                  <div className="font-mono text-xs font-bold">
                    {payment.receiptNumber}
                  </div>
                  <div className="text-xs text-zinc-500">
                    {payment.paymentMethod} · {dateTime(payment.receivedAt)}
                  </div>
                </div>
                <div className="text-right">
                  <strong className="font-mono">
                    {money(payment.amountMinor, payment.currency)}
                  </strong>
                  {payment.refundedMinor > 0 && (
                    <div className="text-xs text-red-700">
                      −{money(payment.refundedMinor, payment.currency)}{" "}
                      refunded
                    </div>
                  )}
                </div>
                {user?.role === "doctor" &&
                  payment.amountMinor > payment.refundedMinor && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setRefundPayment(payment);
                        setRefundAmount(
                          payment.amountMinor - payment.refundedMinor,
                        );
                      }}
                    >
                      Refund
                    </Button>
                  )}
              </div>
            ))}
          </CardContent>
        </Card>
      )}
      {invoice.creditNotes.length > 0 && (
        <Card className="mt-4 no-print">
          <CardHeader>
            <CardTitle>Credit notes</CardTitle>
            <CardDescription>
              Immutable documents generated by confirmed refunds.
            </CardDescription>
          </CardHeader>
          <CardContent className="divide-y">
            {invoice.creditNotes.map((note) => (
              <div
                key={note.creditNumber}
                className="flex items-center gap-3 py-3"
              >
                <FileText className="h-4 w-4 text-zinc-400" />
                <div className="min-w-0 flex-1">
                  <div className="font-mono text-xs font-bold">
                    {note.creditNumber}
                  </div>
                  <div className="truncate text-xs text-zinc-500">
                    {note.reason} · {dateTime(note.createdAt)}
                    {note.restocked ? " · stock returned" : ""}
                  </div>
                </div>
                <b className="font-mono">
                  −{money(note.amountMinor, invoice.currency)}
                </b>
              </div>
            ))}
          </CardContent>
        </Card>
      )}
      {refundPayment && (
        <Card className="mt-4 border-black no-print">
          <CardHeader>
            <CardTitle>Refund & credit note</CardTitle>
            <CardDescription>
              Creates an immutable refund and optionally returns selected sold
              products to stock in the same transaction.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Refund amount">
                <MoneyInput
                  currency={refundPayment.currency}
                  value={refundAmount}
                  onValueChange={setRefundAmount}
                />
              </Field>
              <Field label="Reason">
                <Input
                  required
                  value={refundReason}
                  onChange={(e) => setRefundReason(e.target.value)}
                  placeholder="Return, cancellation, pricing correction…"
                />
              </Field>
            </div>
            <div>
              <div className="mb-2 text-xs font-bold uppercase text-zinc-500">
                Products physically returned
              </div>
              {invoice.items.filter((x) =>
                Boolean((x as { inventoryItemId?: string }).inventoryItemId),
              ).length ? (
                invoice.items
                  .filter((x) =>
                    Boolean(
                      (x as { inventoryItemId?: string }).inventoryItemId,
                    ),
                  )
                  .map((item) => (
                    <label
                      key={item.id}
                      className="flex min-h-11 items-center gap-3 border-t text-sm"
                    >
                      <input
                        type="checkbox"
                        checked={restock.includes(item.id)}
                        onChange={(e) =>
                          setRestock(
                            e.target.checked
                              ? [...restock, item.id]
                              : restock.filter((x) => x !== item.id),
                          )
                        }
                      />
                      <span>
                        {item.quantity} × {item.description}
                      </span>
                    </label>
                  ))
              ) : (
                <p className="text-sm text-zinc-500">
                  This invoice has no stock-linked products.
                </p>
              )}
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setRefundPayment(null)}>
                Cancel
              </Button>
              <Button
                disabled={
                  !refundReason.trim() ||
                  refundAmount <= 0 ||
                  refundAmount >
                    refundPayment.amountMinor - refundPayment.refundedMinor
                }
                onClick={refund}
              >
                Confirm refund & credit note
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {invoice.balanceMinor > 0 && (
        <Card className="mt-4 no-print">
          <CardHeader>
            <CardTitle>Record payment</CardTitle>
            <CardDescription>
              Partial payments create separate receipt records.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
            <Field label="Method">
              <Select
                value={method}
                onChange={(event) => setMethod(event.target.value)}
              >
                {methods.data?.items.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Amount">
              <MoneyInput
                currency={invoice.currency}
                value={amount}
                onValueChange={setAmount}
              />
            </Field>
            <Button
              disabled={paying || amount <= 0 || amount > invoice.balanceMinor}
              onClick={pay}
            >
              <CreditCard className="h-4 w-4" />
              {paying ? "Recording…" : "Record payment"}
            </Button>
          </CardContent>
        </Card>
      )}
    </DialogContent>
  );
}

function RegisterForm({
  current,
  onSaved,
}: {
  current: {
    open: boolean;
    id?: string;
    currency?: string;
    openingFloatMinor?: number;
    openedAt?: string;
    openedBy?: string;
  };
  onSaved(): void;
}) {
  const [amount, setAmount] = React.useState(0);
  const [notes, setNotes] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      if (current.open && current.id)
        await api.post(`/cash-register/${current.id}/close`, {
          countedCashMinor: amount,
          notes,
        });
      else
        await api.post("/cash-register/open", {
          currency: "HTG",
          openingFloatMinor: amount,
          notes,
        });
      toast.success(
        current.open ? "Cash register closed" : "Cash register opened",
      );
      onSaved();
    } catch (reason) {
      toast.error(
        reason instanceof Error ? reason.message : "Register action failed",
      );
    } finally {
      setSaving(false);
    }
  };
  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>
          {current.open ? "Close cash register" : "Open cash register"}
        </DialogTitle>
        <DialogDescription>
          {current.open
            ? `Opened by ${current.openedBy} with ${money(current.openingFloatMinor ?? 0, current.currency)}`
            : "Set the physical opening float before accepting cash."}
        </DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={submit}>
        <Field label={current.open ? "Counted cash" : "Opening float"}>
          <MoneyInput
            currency={current.currency ?? "HTG"}
            value={amount}
            onValueChange={setAmount}
          />
        </Field>
        <Field label="Notes">
          <Textarea
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
          />
        </Field>
        <DialogFooter>
          <Button type="submit" disabled={saving}>
            {current.open ? (
              <Banknote className="h-4 w-4" />
            ) : (
              <FileText className="h-4 w-4" />
            )}
            {saving
              ? "Saving…"
              : current.open
                ? "Close & generate Z report"
                : "Open register"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

interface SaleRow { id: string; invoiceNumber: string; patientName: string; status: string; currency: string; totalMinor: number; paidMinor: number; refundedMinor: number; balanceMinor: number; cashier: string; paymentMethods: string[]; createdAt: string }

/**
 * What the till has actually sold, filterable the way a cashier or a manager
 * asks the question — by day, by who was on the register, by how it was paid.
 * Every filter is applied on the server, so this stays usable at any volume.
 */
function SalesHistory() {
  const [from, setFrom] = React.useState("");
  const [to, setTo] = React.useState("");
  const [cashierId, setCashierId] = React.useState("");
  const [paymentMethodId, setPaymentMethodId] = React.useState("");
  const users = useLoad(() => api.get<{ items: { id: string; displayName: string }[] }>("/users"), []);
  const methods = useLoad(() => api.get<{ items: { id: string; name: string }[] }>("/payment-methods"), []);
  const sales = usePagedList<SaleRow>(
    (page, limit) => `/pos/sales?from=${from}&to=${to}&cashierId=${cashierId}&paymentMethodId=${paymentMethodId}&page=${page}&limit=${limit}`,
    [from, to, cashierId, paymentMethodId],
  );
  return <Card className="mt-6 overflow-hidden">
    <CardHeader><CardTitle>Sales history</CardTitle><CardDescription>Every sale the till has rung up, with what has been paid against it.</CardDescription></CardHeader>
    <CardContent>
      <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Field label="From"><Input type="date" value={from} onChange={(event) => setFrom(event.target.value)} /></Field>
        <Field label="To"><Input type="date" value={to} onChange={(event) => setTo(event.target.value)} /></Field>
        <Field label="Cashier"><Select value={cashierId} onChange={(event) => setCashierId(event.target.value)}><option value="">Anyone</option>{users.data?.items.map((user) => <option key={user.id} value={user.id}>{user.displayName}</option>)}</Select></Field>
        <Field label="Payment method"><Select value={paymentMethodId} onChange={(event) => setPaymentMethodId(event.target.value)}><option value="">Any method</option>{methods.data?.items.map((method) => <option key={method.id} value={method.id}>{method.name}</option>)}</Select></Field>
      </div>
      {sales.loading ? <Skeleton className="h-64" /> : sales.error ? <ErrorState message={sales.error.message} retry={sales.reload} /> : sales.items.length ? <>
        <Table>
          <thead><tr><Th>Sale</Th><Th>Customer</Th><Th>Cashier</Th><Th>Paid with</Th><Th>Total</Th><Th>Outstanding</Th><Th>When</Th></tr></thead>
          <tbody>{sales.items.map((sale) => <tr key={sale.id}>
            <Td className="font-mono text-xs font-bold">{sale.invoiceNumber}</Td>
            <Td>{sale.patientName}</Td>
            <Td>{sale.cashier}</Td>
            <Td>{sale.paymentMethods.length ? sale.paymentMethods.join(", ") : <Badge tone="warning">Unpaid</Badge>}</Td>
            <Td className="font-mono">{money(sale.totalMinor, sale.currency)}</Td>
            <Td className="font-mono font-bold">{money(sale.balanceMinor, sale.currency)}</Td>
            <Td className="text-xs text-zinc-500">{dateTime(sale.createdAt)}</Td>
          </tr>)}</tbody>
        </Table>
        <Pager page={sales.page} pageSize={sales.pageSize} total={sales.total} hasMore={sales.hasMore} onPrevious={sales.previous} onNext={sales.next} />
      </> : <EmptyState title="No sales in this range" description="Widen the dates or clear the filters." />}
    </CardContent>
  </Card>;
}

interface RegisterReport { currency: string; openingFloatMinor: number; openedBy: string; salesCount: number; takingsMinor: number; refundsMinor: number; netMinor: number; expectedCashMinor: number; byMethod: { method: string; payments: number; amountMinor: number; refundedMinor: number; netMinor: number }[] }

/** The end-of-day Z-report, readable before the drawer is counted. */
function RegisterReportButton({ sessionId }: { sessionId: string }) {
  const [open, setOpen] = React.useState(false);
  return <>
    <Button variant="outline" onClick={() => setOpen(true)}><Receipt className="h-4 w-4" />Z-report</Button>
    <Dialog open={open} onOpenChange={setOpen}>{open && <RegisterReportView sessionId={sessionId} />}</Dialog>
  </>;
}

function RegisterReportView({ sessionId }: { sessionId: string }) {
  const report = useLoad(() => api.get<RegisterReport>(`/cash-register/${sessionId}/report`), [sessionId]);
  return <DialogContent>
    <DialogHeader><DialogTitle>Register report</DialogTitle><DialogDescription>What this register has taken since it was opened, and what the drawer should hold if it were counted now.</DialogDescription></DialogHeader>
    {report.loading ? <Skeleton className="h-56" /> : report.error || !report.data ? <ErrorState message={report.error?.message ?? "Report unavailable"} retry={report.reload} /> : <div className="grid gap-4">
      <div className="grid grid-cols-2 gap-3 text-sm">
        {([["Opened by", report.data.openedBy], ["Sales", String(report.data.salesCount)], ["Opening float", money(report.data.openingFloatMinor, report.data.currency)], ["Taken", money(report.data.takingsMinor, report.data.currency)], ["Refunded", money(report.data.refundsMinor, report.data.currency)], ["Expected cash in drawer", money(report.data.expectedCashMinor, report.data.currency)]] as const).map(([label, value]) => (
          <div key={label}><div className="text-xs font-bold uppercase text-zinc-500">{label}</div><div className="mt-1 font-mono font-bold">{value}</div></div>
        ))}
      </div>
      <Table>
        <thead><tr><Th>Method</Th><Th>Payments</Th><Th>Taken</Th><Th>Refunded</Th><Th>Net</Th></tr></thead>
        <tbody>{report.data.byMethod.map((line) => <tr key={line.method}><Td>{line.method}</Td><Td className="font-mono">{line.payments}</Td><Td className="font-mono">{money(line.amountMinor, report.data!.currency)}</Td><Td className="font-mono">{money(line.refundedMinor, report.data!.currency)}</Td><Td className="font-mono font-bold">{money(line.netMinor, report.data!.currency)}</Td></tr>)}</tbody>
      </Table>
    </div>}
  </DialogContent>;
}
