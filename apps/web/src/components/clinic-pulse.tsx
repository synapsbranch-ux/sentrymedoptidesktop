import { Activity, CalendarX2, Clock4, Timer } from "lucide-react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Badge, EmptyState, ErrorState, Skeleton } from "./ui/data";

interface StageMetric { stage: string; samples: number; averageMinutes: number; currentCount: number }
interface HeatmapCell { weekday: number; hour: number; arrivals: number; averageCycleMinutes: number }
interface NoShowSlot { weekday: number; hour: number; total: number; noShows: number; rate: number }
interface NoShowPatient { patientId: string; medicalRecordNumber: string; patientName: string; total: number; noShows: number; rate: number }
interface OperationsAnalytics {
  periodDays: number;
  stages: StageMetric[];
  heatmap: HeatmapCell[];
  duration: { matchedAppointments: number; plannedAverageMinutes: number; actualAverageMinutes: number; varianceAverageMinutes: number; withinTolerancePercent: number };
  noShowSlots: NoShowSlot[];
  noShowPatients: NoShowPatient[];
}

const weekdays = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const stageNames: Record<string, string> = { checked_in: "Checked in", waiting_nurse: "Waiting for nurse", pre_test: "Pre-test", waiting_doctor: "Waiting for doctor", in_consultation: "Consultation", checkout: "Checkout" };

export function ClinicPulse({ revision }: { revision: number }) {
  const analytics = useLoad(() => api.get<OperationsAnalytics>("/operations/analytics?days=28"), [revision]);
  if (analytics.loading) return <section className="mt-6"><Skeleton className="h-96" /></section>;
  if (analytics.error) return <section className="mt-6"><ErrorState message={analytics.error.message} retry={analytics.reload} /></section>;
  const data = analytics.data!;
  const stageMax = Math.max(1, ...data.stages.map((item) => item.averageMinutes));
  const heatMax = Math.max(1, ...data.heatmap.map((item) => item.averageCycleMinutes));
  const heat = new Map(data.heatmap.map((item) => [`${item.weekday}-${item.hour}`, item]));
  return <section className="mt-6">
    <div className="mb-3 flex flex-wrap items-end justify-between gap-3"><div><p className="section-title">Clinic pulse</p><h2 className="text-xl font-bold tracking-tight">Flow and reliability</h2><p className="mt-1 text-sm text-zinc-500">Rolling {data.periodDays}-day operational view</p></div><Badge tone="neutral">Live from recorded activity</Badge></div>
    <div className="grid gap-4 xl:grid-cols-2">
      <Card><CardHeader><CardTitle><span className="flex items-center gap-2"><Activity className="h-4 w-4" />Bottlenecks by stage</span></CardTitle><CardDescription>Average completed time in each patient-flow stage</CardDescription></CardHeader><CardContent>{data.stages.length ? <div className="space-y-4">{data.stages.map((item) => <div key={item.stage}><div className="mb-1 flex items-center justify-between gap-3 text-xs"><span className="font-semibold">{stageNames[item.stage] ?? item.stage.replaceAll("_", " ")}</span><span className="font-mono text-zinc-500">{item.averageMinutes.toFixed(1)} min · {item.samples} visits{item.currentCount ? ` · ${item.currentCount} now` : ""}</span></div><div className="h-2 overflow-hidden rounded-full bg-zinc-100"><div className="h-full rounded-full bg-zinc-900" style={{ width: `${Math.max(2, item.averageMinutes / stageMax * 100)}%` }} /></div></div>)}</div> : <EmptyState title="No completed flow yet" description="Stage timings appear after patients move through the queue." />}</CardContent></Card>
      <Card><CardHeader><CardTitle><span className="flex items-center gap-2"><Timer className="h-4 w-4" />Planned vs actual duration</span></CardTitle><CardDescription>Actual time uses the recorded consultation stage</CardDescription></CardHeader><CardContent>{data.duration.matchedAppointments ? <><div className="grid grid-cols-2 gap-3"><Metric label="Planned average" value={`${data.duration.plannedAverageMinutes.toFixed(1)} min`} /><Metric label="Actual average" value={`${data.duration.actualAverageMinutes.toFixed(1)} min`} /><Metric label="Average variance" value={`${data.duration.varianceAverageMinutes > 0 ? "+" : ""}${data.duration.varianceAverageMinutes.toFixed(1)} min`} /><Metric label="Within ±10 min" value={`${data.duration.withinTolerancePercent.toFixed(1)}%`} /></div><p className="mt-3 text-xs text-zinc-500">Based on {data.duration.matchedAppointments} matched appointment{data.duration.matchedAppointments === 1 ? "" : "s"}.</p></> : <EmptyState title="No matched durations" description="Complete queued appointments to compare scheduled and consultation time." />}</CardContent></Card>
      <Card className="xl:col-span-2"><CardHeader><CardTitle><span className="flex items-center gap-2"><Clock4 className="h-4 w-4" />Day × hour heatmap</span></CardTitle><CardDescription>Average total clinic cycle by arrival slot; darker cells indicate longer cycles</CardDescription></CardHeader><CardContent><div className="overflow-x-auto"><div className="min-w-[820px]"><div className="grid grid-cols-[3rem_repeat(24,minmax(1.5rem,1fr))] gap-1 text-center text-[9px] text-zinc-500"><span />{Array.from({ length: 24 }, (_, hour) => <span key={hour}>{hour % 3 === 0 ? `${String(hour).padStart(2, "0")}h` : ""}</span>)}{weekdays.map((day, weekday) => <div className="contents" key={day}><span className="self-center text-left text-xs font-semibold">{day}</span>{Array.from({ length: 24 }, (_, hour) => { const item = heat.get(`${weekday}-${hour}`); const intensity = item ? .12 + item.averageCycleMinutes / heatMax * .78 : .035; return <div key={hour} className="aspect-square rounded-sm border border-zinc-200" style={{ backgroundColor: `rgba(24,24,27,${intensity})` }} title={item ? `${day} ${String(hour).padStart(2, "0")}:00 · ${item.averageCycleMinutes.toFixed(1)} min · ${item.arrivals} arrivals` : `${day} ${String(hour).padStart(2, "0")}:00 · no data`} />; })}</div>)}</div></div></div></CardContent></Card>
      <Card><CardHeader><CardTitle><span className="flex items-center gap-2"><CalendarX2 className="h-4 w-4" />No-show by time slot</span></CardTitle><CardDescription>Completed and no-show appointments only</CardDescription></CardHeader><CardContent>{data.noShowSlots.length ? <div className="divide-y">{data.noShowSlots.slice(0, 8).map((item) => <div key={`${item.weekday}-${item.hour}`} className="flex items-center justify-between gap-3 py-3"><div><p className="text-sm font-semibold">{weekdays[item.weekday]} at {String(item.hour).padStart(2, "0")}:00</p><p className="text-xs text-zinc-500">{item.noShows} of {item.total} appointments</p></div><Badge tone={item.rate >= 25 ? "warning" : "neutral"}>{item.rate.toFixed(1)}%</Badge></div>)}</div> : <EmptyState title="No no-shows recorded" description="Rates appear when a past appointment is marked no-show." />}</CardContent></Card>
      <Card><CardHeader><CardTitle>Patients with no-shows</CardTitle><CardDescription>Follow-up signal, ordered by occurrence count</CardDescription></CardHeader><CardContent>{data.noShowPatients.length ? <div className="divide-y">{data.noShowPatients.slice(0, 8).map((item) => <div key={item.patientId} className="flex items-center justify-between gap-3 py-3"><div className="min-w-0"><p className="truncate text-sm font-semibold">{item.patientName}</p><p className="font-mono text-[11px] text-zinc-500">{item.medicalRecordNumber} · {item.noShows}/{item.total}</p></div><Badge tone={item.rate >= 25 ? "warning" : "neutral"}>{item.rate.toFixed(1)}%</Badge></div>)}</div> : <EmptyState title="No patient no-shows" description="Patients appear here when a past appointment is marked no-show." />}</CardContent></Card>
    </div>
  </section>;
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div className="rounded-lg border bg-zinc-50 p-4"><p className="text-xs font-semibold text-zinc-500">{label}</p><p className="mt-2 font-mono text-xl font-bold">{value}</p></div>;
}
