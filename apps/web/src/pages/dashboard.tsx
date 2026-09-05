import { AlertTriangle, CalendarDays, CheckCircle2, Clock3, FlaskConical, PackageMinus, Receipt, UserPlus, Users } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { useLoad } from "../hooks";
import { money } from "../lib";
import { useRealtime } from "../realtime";
import type { Appointment, QueueEntry } from "../types";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Badge, EmptyState, ErrorState, Skeleton } from "../components/ui/data";

interface DashboardData { role: string; today: Record<string, number>; finance?: { baseCurrency: string; todayRevenueMinor: number; monthRevenueMinor: number; outstandingMinor: number; monthExpensesMinor: number } }
export function DashboardPage() {
  const { user } = useAuth(); const { revision } = useRealtime();
  const dashboard = useLoad(() => api.get<DashboardData>("/dashboard"), [revision]);
  const queue = useLoad(() => api.get<{ items: QueueEntry[] }>("/queue"), [revision]);
  const appointments = useLoad(() => api.get<{ items: Appointment[] }>(`/appointments?date=${new Date().toISOString().slice(0, 10)}`), [revision]);
  if (dashboard.error) return <div className="page"><ErrorState message={dashboard.error.message} retry={dashboard.reload} /></div>;
  const metrics = [
    ["Appointments", dashboard.data?.today.appointmentsToday, CalendarDays], ["Waiting", dashboard.data?.today.patientsWaiting, Clock3],
    ["Consultations", dashboard.data?.today.consultationsCompleted, CheckCircle2], ["New patients", dashboard.data?.today.newPatients, UserPlus],
    ["Pending lab", dashboard.data?.today.pendingLabOrders, FlaskConical], ["Ready glasses", dashboard.data?.today.readyGlasses, Users],
    ["Low stock", dashboard.data?.today.lowStockItems, PackageMinus], ["Unpaid invoices", dashboard.data?.today.unpaidInvoices, Receipt],
  ] as const;
  return <div className="page"><div className="flex flex-wrap items-end justify-between gap-4"><div><p className="section-title">Today</p><h1 className="page-title">Good day, {user?.displayName.split(" ")[0]}</h1><p className="page-description">Here is the clinic's live operating picture.</p></div><Badge tone="success">Clinic server online</Badge></div>
    <section className="mt-6 grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-8">{metrics.map(([label, value, Icon]) => <Card key={label} className="min-w-0"><CardContent className="p-4"><div className="flex items-center justify-between"><Icon className="h-4 w-4 text-zinc-400" />{label === "Low stock" && Number(value) > 0 && <AlertTriangle className="h-4 w-4 text-amber-600" />}</div>{dashboard.loading ? <Skeleton className="mt-5 h-8 w-16" /> : <div className="stat-value mt-4">{value ?? 0}</div>}<div className="mt-1 truncate text-[11px] font-semibold text-zinc-500">{label}</div></CardContent></Card>)}</section>
    {dashboard.data?.finance && <section className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{[["Revenue today", dashboard.data.finance.todayRevenueMinor], ["Revenue this month", dashboard.data.finance.monthRevenueMinor], ["Outstanding", dashboard.data.finance.outstandingMinor], ["Expenses this month", dashboard.data.finance.monthExpensesMinor]].map(([label, value]) => <Card key={String(label)}><CardContent className="p-5"><p className="text-xs font-semibold text-zinc-500">{label} · {dashboard.data?.finance?.baseCurrency}</p><p className="mt-2 font-mono text-2xl font-bold tracking-tight">{money(Number(value),dashboard.data?.finance?.baseCurrency)}</p></CardContent></Card>)}</section>}
    <section className="mt-6 grid gap-4 xl:grid-cols-2"><Card><CardHeader><CardTitle>Waiting room</CardTitle><CardDescription>Live queue, oldest arrival first</CardDescription></CardHeader><CardContent>{queue.loading ? <Skeleton className="h-48" /> : queue.data?.items.length ? <div className="divide-y">{queue.data.items.slice(0, 8).map((item) => <div key={item.id} className="flex items-center gap-3 py-3"><div className="grid h-9 w-9 place-items-center rounded-full bg-zinc-100 text-xs font-bold">{item.patientName.split(" ").map((part) => part[0]).slice(0, 2).join("")}</div><div className="min-w-0 flex-1"><div className="truncate text-sm font-semibold">{item.patientName}</div><div className="font-mono text-[11px] text-zinc-500">{item.medicalRecordNumber}</div></div><Badge tone={item.stage === "waiting_doctor" ? "warning" : "neutral"}>{item.stage.replaceAll("_", " ")}</Badge></div>)}</div> : <EmptyState title="Waiting room is clear" description="Checked-in and walk-in patients will appear here." />}</CardContent></Card>
      <Card><CardHeader><CardTitle>Upcoming appointments</CardTitle><CardDescription>Today's scheduled patient flow</CardDescription></CardHeader><CardContent>{appointments.loading ? <Skeleton className="h-48" /> : appointments.data?.items.length ? <div className="divide-y">{appointments.data.items.slice(0, 8).map((item) => <div key={item.id} className="flex items-center gap-3 py-3"><div className="w-14 font-mono text-xs font-bold">{new Date(item.startsAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</div><div className="min-w-0 flex-1"><div className="truncate text-sm font-semibold">{item.patientName}</div><div className="truncate text-xs text-zinc-500">{item.reason || item.type}</div></div><Badge>{item.status.replaceAll("_", " ")}</Badge></div>)}</div> : <EmptyState title="No appointments today" description="Scheduled appointments will appear here." />}</CardContent></Card></section>
  </div>;
}
