import * as React from "react";
import { CalendarClock, FileSignature, LogIn, LogOut, Plus, Printer, Users, Wallet } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useLoad, usePagedList } from "../hooks";
import { dateTime, money, todayInput } from "../lib";
import { PrintHeader, triggerPrint } from "../components/print";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "../components/ui/dialog";
import { Badge, EmptyState, ErrorState, Pager, Skeleton, Table, Td, Th } from "../components/ui/data";
import { Field, Input, Select, Textarea } from "../components/ui/input";

interface Position { id: string; title: string; defaultSalaryMinor: number; currency: string; active: boolean; staffCount: number }
interface Employee { id: string; employeeNumber: string; firstName: string; lastName: string; position: string; positionId: string; employmentType: string; status: string; startedOn: string; endedOn: string; payBasis: string; baseSalaryMinor: number; hourlyRateMinor: number; currency: string; phone: string; email: string; version: number }
interface AttendanceEntry { id: string; employeeId: string; employeeName: string; employeeNumber: string; workDate: string; clockIn: string; clockOut: string; minutesWorked: number; entryType: string; notes: string; openShift: boolean }
interface PayrollRun { id: string; runNumber: string; periodStart: string; periodEnd: string; status: string; currency: string; grossMinor: number; deductionsMinor: number; netMinor: number; payslips: number; version: number }
interface Payslip { id: string; employeeId: string; employeeNumber: string; employeeName: string; position: string; grossMinor: number; deductionsMinor: number; netMinor: number; minutesWorked: number; daysPresent: number; breakdown: { basis: string; note: string; deductions: { name: string; amountMinor: number }[] } }
interface PayrollDetail extends Omit<PayrollRun, "payslips"> { notes: string; payslips: Payslip[] }
interface DocumentTemplate { id: string; kind: string; title: string; body: string }
interface HRDocument { id: string; employeeId: string; employeeName: string; kind: string; title: string; body: string; generatedAt: string; generatedBy: string }

const statusTone = (status: string): "neutral" | "success" | "warning" | "danger" =>
  status === "active" || status === "paid" ? "success" : status === "terminated" || status === "cancelled" ? "danger" : status === "approved" ? "success" : "warning";
const hours = (minutes: number) => `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, "0")}m`;

export function HRPage() {
  const [tab, setTab] = React.useState<"staff" | "attendance" | "payroll" | "documents">("staff");
  return <div className="page">
    <div><p className="section-title">People</p><h1 className="page-title">Human resources</h1><p className="page-description">Staff records, hours worked, payroll and the paperwork that goes with them.</p></div>
    <div className="mt-6 flex gap-1 overflow-x-auto">
      {([["staff", "Staff", Users], ["attendance", "Attendance", CalendarClock], ["payroll", "Payroll", Wallet], ["documents", "Documents", FileSignature]] as const).map(([value, label, Icon]) => (
        <button key={value} onClick={() => setTab(value)} className={`flex min-h-10 items-center gap-2 whitespace-nowrap rounded-md px-3 text-sm font-semibold ${tab === value ? "bg-black text-white" : "text-zinc-500 hover:bg-zinc-100"}`}>
          <Icon className="h-4 w-4" />{label}
        </button>
      ))}
    </div>
    {tab === "staff" && <StaffTab />}
    {tab === "attendance" && <AttendanceTab />}
    {tab === "payroll" && <PayrollTab />}
    {tab === "documents" && <DocumentsTab />}
  </div>;
}

function StaffTab() {
  const [creating, setCreating] = React.useState(false);
  const [editing, setEditing] = React.useState<Employee | null>(null);
  const [status, setStatus] = React.useState("");
  const positions = useLoad(() => api.get<{ items: Position[] }>("/hr/positions"), []);
  const employees = usePagedList<Employee>((page, limit) => `/hr/employees?status=${status}&page=${page}&limit=${limit}`, [status]);
  return <div className="grid gap-4">
    <div className="mt-4 flex flex-wrap items-end justify-between gap-3">
      <div className="w-56"><Field label="Status"><Select value={status} onChange={(event) => setStatus(event.target.value)}><option value="">Everyone</option><option value="active">Active</option><option value="on_leave">On leave</option><option value="suspended">Suspended</option><option value="terminated">Left</option></Select></Field></div>
      <div className="flex gap-2">
        <PositionsDialog positions={positions.data?.items ?? []} onSaved={positions.reload} />
        <Dialog open={creating} onOpenChange={setCreating}>
          <DialogTrigger asChild><Button><Plus className="h-4 w-4" />New employee</Button></DialogTrigger>
          {creating && <EmployeeForm positions={positions.data?.items ?? []} onSaved={() => { setCreating(false); employees.reload(); }} />}
        </Dialog>
      </div>
    </div>
    <Card>
      <CardHeader><CardTitle>Staff</CardTitle><CardDescription>Employees and contractors, with the pay terms payroll computes from.</CardDescription></CardHeader>
      <CardContent>
        {employees.loading ? <Skeleton className="h-64" /> : employees.error ? <ErrorState message={employees.error.message} retry={employees.reload} /> : employees.items.length ? <>
          <Table>
            <thead><tr><Th>Number</Th><Th>Name</Th><Th>Position</Th><Th>Engagement</Th><Th>Pay</Th><Th>Status</Th><Th>{""}</Th></tr></thead>
            <tbody>{employees.items.map((employee) => <tr key={employee.id}>
              <Td className="font-mono text-xs font-bold">{employee.employeeNumber}</Td>
              <Td><strong>{employee.firstName} {employee.lastName}</strong><div className="text-xs text-zinc-500">{employee.phone || employee.email || "—"}</div></Td>
              <Td>{employee.position || "—"}</Td>
              <Td className="capitalize">{employee.employmentType}</Td>
              <Td className="font-mono text-xs">{employee.payBasis === "hourly" ? `${money(employee.hourlyRateMinor, employee.currency)}/h` : `${money(employee.baseSalaryMinor, employee.currency)} ${employee.payBasis}`}</Td>
              <Td><Badge tone={statusTone(employee.status)}>{employee.status.replaceAll("_", " ")}</Badge></Td>
              <Td><Button size="sm" variant="outline" onClick={() => setEditing(employee)}>Edit</Button></Td>
            </tr>)}</tbody>
          </Table>
          <Pager page={employees.page} pageSize={employees.pageSize} total={employees.total} hasMore={employees.hasMore} onPrevious={employees.previous} onNext={employees.next} />
        </> : <EmptyState title="No staff records" description="Add the clinic's employees and contractors to track hours, pay and contracts." action={<Button onClick={() => setCreating(true)}>Add the first employee</Button>} />}
      </CardContent>
    </Card>
    <Dialog open={Boolean(editing)} onOpenChange={(open) => !open && setEditing(null)}>
      {editing && <EmployeeForm employee={editing} positions={positions.data?.items ?? []} onSaved={() => { setEditing(null); employees.reload(); }} />}
    </Dialog>
  </div>;
}

function PositionsDialog({ positions, onSaved }: { positions: Position[]; onSaved(): void }) {
  const [open, setOpen] = React.useState(false);
  const [title, setTitle] = React.useState("");
  const [salary, setSalary] = React.useState(0);
  const [saving, setSaving] = React.useState(false);
  const add = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try { await api.post("/hr/positions", { title, defaultSalaryMinor: salary, currency: "HTG" }); toast.success("Position added"); setTitle(""); setSalary(0); onSaved(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not add the position"); }
    finally { setSaving(false); }
  };
  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger asChild><Button variant="outline">Positions</Button></DialogTrigger>
    <DialogContent>
      <DialogHeader><DialogTitle>Positions</DialogTitle><DialogDescription>A position carries the default pay so a new hire does not need it retyped. Every employee keeps their own agreed figure.</DialogDescription></DialogHeader>
      <form className="grid gap-3 sm:grid-cols-[1fr_150px_auto] sm:items-end" onSubmit={add}>
        <Field label="Title"><Input required value={title} onChange={(event) => setTitle(event.target.value)} /></Field>
        <Field label="Default salary"><Input type="number" min={0} value={salary} onChange={(event) => setSalary(Number(event.target.value))} /></Field>
        <Button type="submit" disabled={saving || !title.trim()}>Add</Button>
      </form>
      {positions.length ? <Table><thead><tr><Th>Title</Th><Th>Default salary</Th><Th>Staff</Th></tr></thead><tbody>{positions.map((position) => <tr key={position.id}><Td>{position.title}</Td><Td className="font-mono">{money(position.defaultSalaryMinor, position.currency)}</Td><Td className="font-mono">{position.staffCount}</Td></tr>)}</tbody></Table> : <p className="text-sm text-zinc-500">No positions yet.</p>}
    </DialogContent>
  </Dialog>;
}

function EmployeeForm({ employee, positions, onSaved }: { employee?: Employee; positions: Position[]; onSaved(): void }) {
  const [form, setForm] = React.useState({
    firstName: employee?.firstName ?? "", lastName: employee?.lastName ?? "", positionId: employee?.positionId ?? "",
    employmentType: employee?.employmentType ?? "employee", status: employee?.status ?? "active",
    startedOn: employee?.startedOn ?? todayInput(), endedOn: employee?.endedOn ?? "",
    payBasis: employee?.payBasis ?? "monthly", baseSalaryMinor: employee?.baseSalaryMinor ?? 0, hourlyRateMinor: employee?.hourlyRateMinor ?? 0,
    currency: employee?.currency ?? "HTG", phone: employee?.phone ?? "", email: employee?.email ?? "", address: "", nationalId: "", bankAccount: "", notes: "",
  });
  const [deductions, setDeductions] = React.useState<{ name: string; type: string; value: number }[]>([]);
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      const body = { ...form, deductions };
      if (employee) await api.put(`/hr/employees/${employee.id}`, { ...body, version: employee.version });
      else await api.post("/hr/employees", body);
      toast.success(employee ? "Employee updated" : "Employee added");
      onSaved();
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not save the employee record"); }
    finally { setSaving(false); }
  };
  const usePositionDefault = (positionId: string) => {
    const position = positions.find((candidate) => candidate.id === positionId);
    setForm({ ...form, positionId, baseSalaryMinor: position && form.baseSalaryMinor === 0 ? position.defaultSalaryMinor : form.baseSalaryMinor });
  };
  return <DialogContent className="max-w-3xl">
    <DialogHeader><DialogTitle>{employee ? `${employee.firstName} ${employee.lastName}` : "New employee"}</DialogTitle><DialogDescription>Pay terms here are what payroll computes from: a salaried employee is paid their agreed figure, an hourly one is paid the hours attendance records.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="First name"><Input required value={form.firstName} onChange={(event) => setForm({ ...form, firstName: event.target.value })} /></Field>
        <Field label="Last name"><Input required value={form.lastName} onChange={(event) => setForm({ ...form, lastName: event.target.value })} /></Field>
        <Field label="Position"><Select value={form.positionId} onChange={(event) => usePositionDefault(event.target.value)}><option value="">Not set</option>{positions.map((position) => <option key={position.id} value={position.id}>{position.title}</option>)}</Select></Field>
        <Field label="Engagement"><Select value={form.employmentType} onChange={(event) => setForm({ ...form, employmentType: event.target.value })}><option value="employee">Employee</option><option value="contractor">Contractor</option></Select></Field>
        <Field label="Status"><Select value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}><option value="active">Active</option><option value="on_leave">On leave</option><option value="suspended">Suspended</option><option value="terminated">Left</option></Select></Field>
        <Field label="Pay basis"><Select value={form.payBasis} onChange={(event) => setForm({ ...form, payBasis: event.target.value })}><option value="monthly">Monthly</option><option value="biweekly">Every two weeks</option><option value="weekly">Weekly</option><option value="hourly">Hourly</option></Select></Field>
        {form.payBasis === "hourly"
          ? <Field label="Hourly rate (minor units)"><Input type="number" min={0} value={form.hourlyRateMinor} onChange={(event) => setForm({ ...form, hourlyRateMinor: Number(event.target.value) })} /></Field>
          : <Field label="Salary per period (minor units)"><Input type="number" min={0} value={form.baseSalaryMinor} onChange={(event) => setForm({ ...form, baseSalaryMinor: Number(event.target.value) })} /></Field>}
        <Field label="Started on"><Input type="date" value={form.startedOn} onChange={(event) => setForm({ ...form, startedOn: event.target.value })} /></Field>
        <Field label="Ended on"><Input type="date" value={form.endedOn} onChange={(event) => setForm({ ...form, endedOn: event.target.value })} /></Field>
        <Field label="Phone"><Input value={form.phone} onChange={(event) => setForm({ ...form, phone: event.target.value })} /></Field>
        <Field label="Email"><Input type="email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} /></Field>
        <Field label="Address"><Input value={form.address} onChange={(event) => setForm({ ...form, address: event.target.value })} /></Field>
        <Field label="National ID"><Input value={form.nationalId} onChange={(event) => setForm({ ...form, nationalId: event.target.value })} /></Field>
        <Field label="Bank account"><Input value={form.bankAccount} onChange={(event) => setForm({ ...form, bankAccount: event.target.value })} /></Field>
      </div>
      <fieldset className="grid gap-2 rounded-md border p-3">
        <legend className="px-1 text-xs font-bold uppercase">Standing deductions</legend>
        {deductions.map((deduction, index) => (
          <div className="grid grid-cols-[1fr_120px_120px_auto] items-end gap-2" key={index}>
            <Field label="Name"><Input value={deduction.name} onChange={(event) => setDeductions(deductions.map((item, position) => position === index ? { ...item, name: event.target.value } : item))} /></Field>
            <Field label="Type"><Select value={deduction.type} onChange={(event) => setDeductions(deductions.map((item, position) => position === index ? { ...item, type: event.target.value } : item))}><option value="fixed">Fixed</option><option value="percent">Percent</option></Select></Field>
            <Field label="Value"><Input type="number" min={0} value={deduction.value} onChange={(event) => setDeductions(deductions.map((item, position) => position === index ? { ...item, value: Number(event.target.value) } : item))} /></Field>
            <Button type="button" size="icon" variant="ghost" aria-label="Remove deduction" onClick={() => setDeductions(deductions.filter((_, position) => position !== index))}>×</Button>
          </div>
        ))}
        <div><Button type="button" size="sm" variant="outline" onClick={() => setDeductions([...deductions, { name: "", type: "percent", value: 0 }])}><Plus className="h-3 w-3" />Add deduction</Button></div>
      </fieldset>
      <Field label="Notes"><Textarea value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} /></Field>
      <DialogFooter><Button type="submit" disabled={saving}>{saving ? "Saving…" : employee ? "Save changes" : "Add employee"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

function AttendanceTab() {
  const [from, setFrom] = React.useState(new Date(Date.now() - 14 * 86400000).toISOString().slice(0, 10));
  const [to, setTo] = React.useState(todayInput());
  const [employeeId, setEmployeeId] = React.useState("");
  const employees = useLoad(() => api.get<{ items: Employee[] }>("/hr/employees?status=active&limit=200"), []);
  const attendance = usePagedList<AttendanceEntry>((page, limit) => `/hr/attendance?from=${from}&to=${to}&employeeId=${employeeId}&page=${page}&limit=${limit}`, [from, to, employeeId]);
  const act = async (path: string, body: Record<string, unknown>, success: string) => {
    try { await api.post(path, body); toast.success(success); attendance.reload(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not record attendance"); }
  };
  return <div className="mt-4 grid gap-4">
    <Card>
      <CardHeader><CardTitle>Clock in and out</CardTitle><CardDescription>A clocked shift derives its hours from the two times, so nobody types a total. A day can also be entered afterwards from the book.</CardDescription></CardHeader>
      <CardContent className="grid gap-3 sm:grid-cols-[1fr_auto_auto_auto] sm:items-end">
        <Field label="Employee"><Select value={employeeId} onChange={(event) => setEmployeeId(event.target.value)}><option value="">Choose…</option>{employees.data?.items.map((employee) => <option key={employee.id} value={employee.id}>{employee.firstName} {employee.lastName}</option>)}</Select></Field>
        <Button disabled={!employeeId} onClick={() => act("/hr/attendance", { employeeId, entryType: "clocked" }, "Clocked in")}><LogIn className="h-4 w-4" />Clock in</Button>
        <Button variant="outline" disabled={!employeeId} onClick={() => act("/hr/attendance/clock-out", { employeeId }, "Clocked out")}><LogOut className="h-4 w-4" />Clock out</Button>
        <ManualEntry employeeId={employeeId} onSaved={attendance.reload} />
      </CardContent>
    </Card>
    <Card>
      <CardHeader>
        <CardTitle>Attendance register</CardTitle>
        <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Field label="From"><Input type="date" value={from} onChange={(event) => setFrom(event.target.value)} /></Field>
          <Field label="To"><Input type="date" value={to} onChange={(event) => setTo(event.target.value)} /></Field>
        </div>
      </CardHeader>
      <CardContent>
        {attendance.loading ? <Skeleton className="h-64" /> : attendance.items.length ? <>
          <Table>
            <thead><tr><Th>Date</Th><Th>Employee</Th><Th>In</Th><Th>Out</Th><Th>Worked</Th><Th>Type</Th></tr></thead>
            <tbody>{attendance.items.map((entry) => <tr key={entry.id}>
              <Td>{entry.workDate}</Td>
              <Td>{entry.employeeName}</Td>
              <Td className="text-xs">{entry.clockIn ? dateTime(entry.clockIn) : "—"}</Td>
              <Td className="text-xs">{entry.clockOut ? dateTime(entry.clockOut) : entry.openShift ? <Badge tone="warning">Still in</Badge> : "—"}</Td>
              <Td className="font-mono">{hours(entry.minutesWorked)}</Td>
              <Td className="capitalize">{entry.entryType}</Td>
            </tr>)}</tbody>
          </Table>
          <Pager page={attendance.page} pageSize={attendance.pageSize} total={attendance.total} hasMore={attendance.hasMore} onPrevious={attendance.previous} onNext={attendance.next} />
        </> : <EmptyState title="No attendance in this range" description="Clock somebody in, or enter a day from the book." />}
      </CardContent>
    </Card>
  </div>;
}

function ManualEntry({ employeeId, onSaved }: { employeeId: string; onSaved(): void }) {
  const [open, setOpen] = React.useState(false);
  const [form, setForm] = React.useState({ workDate: todayInput(), minutesWorked: 480, entryType: "manual", notes: "" });
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try { await api.post("/hr/attendance", { employeeId, ...form }); toast.success("Day recorded"); setOpen(false); onSaved(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not record the day"); }
    finally { setSaving(false); }
  };
  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger asChild><Button variant="outline" disabled={!employeeId}>Enter a day</Button></DialogTrigger>
    <DialogContent>
      <DialogHeader><DialogTitle>Enter a day</DialogTitle><DialogDescription>For hours recorded on paper, and for leave, absence and holidays — payroll reads all of them from the same register.</DialogDescription></DialogHeader>
      <form className="grid gap-4" onSubmit={submit}>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Date"><Input type="date" value={form.workDate} onChange={(event) => setForm({ ...form, workDate: event.target.value })} /></Field>
          <Field label="Type"><Select value={form.entryType} onChange={(event) => setForm({ ...form, entryType: event.target.value })}><option value="manual">Worked</option><option value="leave">Leave</option><option value="absent">Absent</option><option value="holiday">Holiday</option></Select></Field>
          <Field label="Minutes worked"><Input type="number" min={0} max={1440} value={form.minutesWorked} onChange={(event) => setForm({ ...form, minutesWorked: Number(event.target.value) })} /></Field>
        </div>
        <Field label="Notes"><Input value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} /></Field>
        <DialogFooter><Button type="submit" disabled={saving}>{saving ? "Recording…" : "Record day"}</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}

function PayrollTab() {
  const [creating, setCreating] = React.useState(false);
  const [opened, setOpened] = React.useState<string | null>(null);
  const runs = usePagedList<PayrollRun>((page, limit) => `/hr/payroll-runs?page=${page}&limit=${limit}`, []);
  return <div className="mt-4 grid gap-4">
    <div className="flex justify-end">
      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogTrigger asChild><Button><Plus className="h-4 w-4" />Prepare payroll</Button></DialogTrigger>
        {creating && <PayrollRunForm onSaved={(id) => { setCreating(false); runs.reload(); setOpened(id); }} />}
      </Dialog>
    </div>
    <Card>
      <CardHeader><CardTitle>Payroll runs</CardTitle><CardDescription>A run is prepared as a draft, approved, then marked paid. A paid run is frozen so the payslips people were paid on stay as they were.</CardDescription></CardHeader>
      <CardContent>
        {runs.loading ? <Skeleton className="h-64" /> : runs.items.length ? <>
          <Table>
            <thead><tr><Th>Run</Th><Th>Period</Th><Th>Payslips</Th><Th>Gross</Th><Th>Deductions</Th><Th>Net</Th><Th>Status</Th><Th>{""}</Th></tr></thead>
            <tbody>{runs.items.map((run) => <tr key={run.id}>
              <Td className="font-mono text-xs font-bold">{run.runNumber}</Td>
              <Td>{run.periodStart} → {run.periodEnd}</Td>
              <Td className="font-mono">{run.payslips}</Td>
              <Td className="font-mono">{money(run.grossMinor, run.currency)}</Td>
              <Td className="font-mono">{money(run.deductionsMinor, run.currency)}</Td>
              <Td className="font-mono font-bold">{money(run.netMinor, run.currency)}</Td>
              <Td><Badge tone={statusTone(run.status)}>{run.status}</Badge></Td>
              <Td><Button size="sm" variant="outline" onClick={() => setOpened(run.id)}>Open</Button></Td>
            </tr>)}</tbody>
          </Table>
          <Pager page={runs.page} pageSize={runs.pageSize} total={runs.total} hasMore={runs.hasMore} onPrevious={runs.previous} onNext={runs.next} />
        </> : <EmptyState title="No payroll runs" description="Prepare a run for a period; it computes a payslip for every active employee from their pay terms and recorded hours." />}
      </CardContent>
    </Card>
    <Dialog open={Boolean(opened)} onOpenChange={(open) => !open && setOpened(null)}>
      {opened && <PayrollRunDetail id={opened} onChanged={runs.reload} />}
    </Dialog>
  </div>;
}

function PayrollRunForm({ onSaved }: { onSaved(id: string): void }) {
  const firstOfMonth = new Date();
  const [periodStart, setPeriodStart] = React.useState(new Date(firstOfMonth.getFullYear(), firstOfMonth.getMonth(), 1).toISOString().slice(0, 10));
  const [periodEnd, setPeriodEnd] = React.useState(todayInput());
  const [notes, setNotes] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      const run = await api.post<{ id: string; runNumber: string; payslips: number }>("/hr/payroll-runs", { periodStart, periodEnd, notes });
      toast.success(`${run.runNumber} prepared with ${run.payslips} payslip(s)`);
      onSaved(run.id);
    } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not prepare the payroll run"); }
    finally { setSaving(false); }
  };
  return <DialogContent>
    <DialogHeader><DialogTitle>Prepare payroll</DialogTitle><DialogDescription>Every active employee gets a payslip: salaried staff at their agreed figure, hourly staff for the hours the attendance register holds for this period.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Period start"><Input type="date" required value={periodStart} onChange={(event) => setPeriodStart(event.target.value)} /></Field>
        <Field label="Period end"><Input type="date" required value={periodEnd} onChange={(event) => setPeriodEnd(event.target.value)} /></Field>
      </div>
      <Field label="Notes"><Input value={notes} onChange={(event) => setNotes(event.target.value)} /></Field>
      <DialogFooter><Button type="submit" disabled={saving}>{saving ? "Preparing…" : "Prepare run"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

function PayrollRunDetail({ id, onChanged }: { id: string; onChanged(): void }) {
  const run = useLoad(() => api.get<PayrollDetail>(`/hr/payroll-runs/${id}`), [id]);
  const [printing, setPrinting] = React.useState<Payslip | null>(null);
  const move = async (status: string) => {
    if (!run.data) return;
    try { await api.patch(`/hr/payroll-runs/${id}/status`, { status, version: run.data.version }); toast.success(`Run marked ${status}`); run.reload(); onChanged(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not update the run"); }
  };
  if (run.loading) return <DialogContent><Skeleton className="h-72" /></DialogContent>;
  if (run.error || !run.data) return <DialogContent><ErrorState message={run.error?.message ?? "Run unavailable"} retry={run.reload} /></DialogContent>;
  const detail = run.data;
  return <DialogContent className="max-h-[92vh] max-w-4xl overflow-y-auto">
    <DialogHeader>
      <DialogTitle>{detail.runNumber}</DialogTitle>
      <DialogDescription>{detail.periodStart} → {detail.periodEnd} · {money(detail.netMinor, detail.currency)} net across {detail.payslips.length} payslip(s)</DialogDescription>
    </DialogHeader>
    <div className="flex flex-wrap gap-2">
      {detail.status === "draft" && <Button size="sm" onClick={() => move("approved")}>Approve run</Button>}
      {detail.status === "approved" && <Button size="sm" onClick={() => move("paid")}><Wallet className="h-3.5 w-3.5" />Mark paid</Button>}
      {(detail.status === "draft" || detail.status === "approved") && <Button size="sm" variant="ghost" onClick={() => move("cancelled")}>Cancel run</Button>}
      <Badge tone={statusTone(detail.status)}>{detail.status}</Badge>
    </div>
    <Table>
      <thead><tr><Th>Employee</Th><Th>Basis</Th><Th>Worked</Th><Th>Gross</Th><Th>Deductions</Th><Th>Net</Th><Th>{""}</Th></tr></thead>
      <tbody>{detail.payslips.map((slip) => <tr key={slip.id}>
        <Td><strong>{slip.employeeName}</strong><div className="font-mono text-[11px] text-zinc-500">{slip.employeeNumber}</div></Td>
        <Td className="text-xs">{slip.breakdown?.note}</Td>
        <Td className="font-mono">{hours(slip.minutesWorked)}</Td>
        <Td className="font-mono">{money(slip.grossMinor, detail.currency)}</Td>
        <Td className="font-mono">{money(slip.deductionsMinor, detail.currency)}</Td>
        <Td className="font-mono font-bold">{money(slip.netMinor, detail.currency)}</Td>
        <Td><Button size="sm" variant="outline" onClick={() => setPrinting(slip)}><Printer className="h-3 w-3" />Payslip</Button></Td>
      </tr>)}</tbody>
    </Table>
    <Dialog open={Boolean(printing)} onOpenChange={(open) => !open && setPrinting(null)}>
      {printing && <PayslipPrint slip={printing} run={detail} />}
    </Dialog>
  </DialogContent>;
}

function PayslipPrint({ slip, run }: { slip: Payslip; run: PayrollDetail }) {
  return <DialogContent className="max-h-[92vh] max-w-2xl overflow-y-auto">
    <DialogHeader className="no-print"><DialogTitle>Payslip · {slip.employeeName}</DialogTitle><DialogDescription>Prints on the clinic's document paper, not the till roll.</DialogDescription></DialogHeader>
    <article className="print-area document-print space-y-5 bg-white p-3 text-black sm:p-8">
      <PrintHeader documentTitle="Payslip" number={run.runNumber} date={`${run.periodStart} — ${run.periodEnd}`} />
      <div className="grid grid-cols-2 gap-4 border-y border-zinc-300 py-4 text-sm">
        <div><div className="text-xs font-bold uppercase text-zinc-500">Employee</div><div className="mt-1 font-bold">{slip.employeeName}</div><div className="font-mono text-xs">{slip.employeeNumber}</div></div>
        <div><div className="text-xs font-bold uppercase text-zinc-500">Position</div><div className="mt-1">{slip.position || "—"}</div></div>
        <div><div className="text-xs font-bold uppercase text-zinc-500">Basis</div><div className="mt-1">{slip.breakdown?.note}</div></div>
        <div><div className="text-xs font-bold uppercase text-zinc-500">Days present</div><div className="mt-1 font-mono">{slip.daysPresent} · {hours(slip.minutesWorked)}</div></div>
      </div>
      <table className="w-full text-left text-sm">
        <tbody>
          <tr className="border-b"><td className="py-2 font-semibold">Gross pay</td><td className="py-2 text-right font-mono">{money(slip.grossMinor, run.currency)}</td></tr>
          {(slip.breakdown?.deductions ?? []).map((deduction) => <tr className="border-b" key={deduction.name}><td className="py-2 pl-4 text-zinc-600">{deduction.name}</td><td className="py-2 text-right font-mono">-{money(deduction.amountMinor, run.currency)}</td></tr>)}
          <tr className="border-b-2 border-black"><td className="py-2 font-bold">Net pay</td><td className="py-2 text-right font-mono text-lg font-bold">{money(slip.netMinor, run.currency)}</td></tr>
        </tbody>
      </table>
      <div className="mt-10 flex justify-between gap-8 text-xs">
        <div className="w-56 border-t border-black pt-2">Employer</div>
        <div className="w-56 border-t border-black pt-2">{slip.employeeName}</div>
      </div>
    </article>
    <div className="no-print flex justify-end"><Button onClick={triggerPrint}><Printer className="h-4 w-4" />Print payslip</Button></div>
  </DialogContent>;
}

function DocumentsTab() {
  const [generating, setGenerating] = React.useState(false);
  const [reading, setReading] = React.useState<HRDocument | null>(null);
  const documents = useLoad(() => api.get<{ items: HRDocument[] }>("/hr/documents"), []);
  return <div className="mt-4 grid gap-4">
    <div className="flex justify-end">
      <Dialog open={generating} onOpenChange={setGenerating}>
        <DialogTrigger asChild><Button><FileSignature className="h-4 w-4" />Generate document</Button></DialogTrigger>
        {generating && <GenerateDocumentForm onSaved={() => { setGenerating(false); documents.reload(); }} />}
      </Dialog>
    </div>
    <Card>
      <CardHeader><CardTitle>Staff documents</CardTitle><CardDescription>Contracts and letters are stored as generated, so editing a template never changes a document somebody has already been given.</CardDescription></CardHeader>
      <CardContent>
        {documents.loading ? <Skeleton className="h-64" /> : documents.data?.items.length ? <Table>
          <thead><tr><Th>Document</Th><Th>Employee</Th><Th>Kind</Th><Th>Generated</Th><Th>{""}</Th></tr></thead>
          <tbody>{documents.data.items.map((document) => <tr key={document.id}>
            <Td><strong>{document.title}</strong></Td>
            <Td>{document.employeeName}</Td>
            <Td className="capitalize">{document.kind}</Td>
            <Td className="text-xs text-zinc-500">{dateTime(document.generatedAt)} · {document.generatedBy}</Td>
            <Td><Button size="sm" variant="outline" onClick={() => setReading(document)}><Printer className="h-3 w-3" />Open</Button></Td>
          </tr>)}</tbody>
        </Table> : <EmptyState title="No staff documents" description="Generate a contract or letter from a template; it is filled from the employee's own record." />}
      </CardContent>
    </Card>
    <Dialog open={Boolean(reading)} onOpenChange={(open) => !open && setReading(null)}>
      {reading && <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto">
        <DialogHeader className="no-print"><DialogTitle>{reading.title}</DialogTitle><DialogDescription>Generated {dateTime(reading.generatedAt)} by {reading.generatedBy}.</DialogDescription></DialogHeader>
        <article className="print-area document-print space-y-5 bg-white p-3 text-black sm:p-8">
          <PrintHeader documentTitle={reading.kind} number={reading.employeeName} date={dateTime(reading.generatedAt)} />
          <pre className="whitespace-pre-wrap font-sans text-sm leading-relaxed">{reading.body}</pre>
        </article>
        <div className="no-print flex justify-end"><Button onClick={triggerPrint}><Printer className="h-4 w-4" />Print</Button></div>
      </DialogContent>}
    </Dialog>
  </div>;
}

function GenerateDocumentForm({ onSaved }: { onSaved(): void }) {
  const employees = useLoad(() => api.get<{ items: Employee[] }>("/hr/employees?limit=200"), []);
  const templates = useLoad(() => api.get<{ items: DocumentTemplate[] }>("/hr/document-templates"), []);
  const [employeeId, setEmployeeId] = React.useState("");
  const [templateId, setTemplateId] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try { await api.post("/hr/documents", { employeeId, templateId }); toast.success("Document generated"); onSaved(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not generate the document"); }
    finally { setSaving(false); }
  };
  return <DialogContent>
    <DialogHeader><DialogTitle>Generate a document</DialogTitle><DialogDescription>The template is filled from the employee's record — name, position, start date, pay and the clinic's own details.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <Field label="Employee"><Select required value={employeeId} onChange={(event) => setEmployeeId(event.target.value)}><option value="">Choose…</option>{employees.data?.items.map((employee) => <option key={employee.id} value={employee.id}>{employee.firstName} {employee.lastName}</option>)}</Select></Field>
      <Field label="Template"><Select required value={templateId} onChange={(event) => setTemplateId(event.target.value)}><option value="">Choose…</option>{templates.data?.items.map((template) => <option key={template.id} value={template.id}>{template.title}</option>)}</Select></Field>
      <p className="rounded-md bg-zinc-50 p-3 text-xs text-zinc-600">The bundled contract template is a starting point, not legal advice. Have it reviewed against the employment law that applies to this clinic before it is signed.</p>
      <DialogFooter><Button type="submit" disabled={saving || !employeeId || !templateId}>{saving ? "Generating…" : "Generate"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}
