import * as React from "react";
import { CalendarClock, Clock3, Glasses, Maximize2, RefreshCw, Users } from "lucide-react";

interface DisplayQueueItem { code: string; patientLabel: string; stage: string; arrivedAt: string; doctor: string; priority: number }
interface DisplayAppointment { code: string; patientLabel: string; startsAt: string; status: string }
interface DisplayData {
  clinic: { name: string; timezone: string };
  settings: { announcement: string; showAppointments: boolean };
  queue: DisplayQueueItem[];
  appointments: DisplayAppointment[];
  logoUrl: string;
}

const stageLabels: Record<string, string> = {
  checked_in: "Checked in",
  waiting_nurse: "Waiting for nurse",
  pre_test: "Pre-test",
  waiting_doctor: "Waiting for doctor",
  in_consultation: "In consultation",
  checkout: "Checkout",
  completed: "Completed",
};

function formatTime(value: string) {
  return new Date(value).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export function PublicDisplayPage() {
  const [data, setData] = React.useState<DisplayData | null>(null);
  const [error, setError] = React.useState("");
  const [connected, setConnected] = React.useState(false);
  const [clock, setClock] = React.useState(new Date());
  const load = React.useCallback(async () => {
    try {
      const response = await fetch("/api/v1/public/display", { cache: "no-store" });
      const body = await response.json() as DisplayData | { message?: string };
      if (!response.ok) throw new Error("message" in body && body.message ? body.message : "Public display unavailable");
      setData(body as DisplayData);
      setError("");
      setConnected(true);
    } catch (reason) {
      setConnected(false);
      setError(reason instanceof Error ? reason.message : "Clinic server unavailable");
    }
  }, []);

  React.useEffect(() => {
    void load();
    const clockTimer = window.setInterval(() => setClock(new Date()), 1000);
    const fallback = window.setInterval(load, 10000);
    const stream = new EventSource("/api/v1/public/events");
    stream.addEventListener("connected", () => setConnected(true));
    stream.addEventListener("update", () => void load());
    stream.onerror = () => setConnected(false);
    return () => { window.clearInterval(clockTimer); window.clearInterval(fallback); stream.close(); };
  }, [load]);

  if (!data) {
    return <main className="grid min-h-dvh place-items-center bg-[var(--background)] p-6 text-[var(--foreground)]"><div className="max-w-md text-center"><Glasses className="mx-auto h-12 w-12" /><h1 className="mt-5 text-2xl font-bold">SentryMed Opti</h1><p className="mt-2 text-[var(--muted-foreground)]">{error || "Connecting to the clinic display…"}</p><button className="mt-6 inline-flex min-h-12 items-center gap-2 rounded-[var(--radius)] bg-[var(--primary)] px-5 font-semibold text-[var(--primary-foreground)]" onClick={() => void load()}><RefreshCw className="h-4 w-4" />Try again</button></div></main>;
  }

  const nowServing = data.queue.filter((item) => ["pre_test", "in_consultation", "checkout"].includes(item.stage));
  const waiting = data.queue.filter((item) => !["pre_test", "in_consultation", "checkout", "completed"].includes(item.stage));
  return <main className="min-h-dvh bg-zinc-950 p-4 text-white sm:p-6 lg:p-8">
    <header className="flex flex-wrap items-center justify-between gap-5 border-b border-white/15 pb-5">
      <div className="flex min-w-0 items-center gap-4">
        <div className="grid h-14 w-14 shrink-0 place-items-center overflow-hidden rounded-xl bg-white text-black">{data.logoUrl ? <img className="h-full w-full object-contain p-1" src={data.logoUrl} alt="" onError={(event) => { event.currentTarget.style.display = "none"; }} /> : <Glasses className="h-7 w-7" />}</div>
        <div><h1 className="truncate text-xl font-bold sm:text-2xl">{data.clinic.name || "SentryMed Opti"}</h1><p className="text-sm text-zinc-400">Patient flow · Live clinic display</p></div>
      </div>
      <div className="flex items-center gap-5"><div className="text-right"><div className="font-mono text-2xl font-bold sm:text-3xl">{clock.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</div><div className="text-xs text-zinc-400">{clock.toLocaleDateString([], { weekday: "long", month: "long", day: "numeric" })}</div></div><button aria-label="Enter fullscreen" className="grid h-12 w-12 place-items-center rounded-lg border border-white/20 hover:bg-white/10" onClick={() => void document.documentElement.requestFullscreen?.()}><Maximize2 className="h-5 w-5" /></button></div>
    </header>
    <div className="mt-6 grid min-w-0 gap-6 xl:grid-cols-[1.15fr_.85fr]">
      <section className="min-w-0 rounded-2xl border border-white/15 bg-white/[.04] p-5 sm:p-6"><div className="flex items-center justify-between gap-3"><div><p className="text-xs font-bold uppercase tracking-[.2em] text-emerald-400">Now serving</p><h2 className="mt-1 text-2xl font-bold">Current progress</h2></div><Users className="h-7 w-7 text-zinc-500" /></div><div className="mt-5 grid gap-3">{nowServing.length ? nowServing.map((item) => <div className="grid gap-3 rounded-xl bg-white p-4 text-zinc-950 sm:grid-cols-[1fr_auto] sm:items-center" key={item.code}><div><div className="font-mono text-2xl font-bold sm:text-3xl">{item.patientLabel}</div><div className="mt-1 text-sm text-zinc-500">{item.doctor ? `Assigned to ${item.doctor}` : "Clinic team"}</div></div><div className="rounded-full bg-zinc-950 px-4 py-2 text-center text-sm font-bold text-white">{stageLabels[item.stage] ?? item.stage}</div></div>) : <div className="grid min-h-40 place-items-center rounded-xl border border-dashed border-white/20 text-center text-zinc-400">No patient is being served right now.</div>}</div>
        <div className="mt-7 flex items-center justify-between"><h3 className="font-bold">Waiting queue</h3><span className="rounded-full bg-white/10 px-3 py-1 font-mono text-sm">{waiting.length}</span></div><div className="mt-3 grid gap-2 sm:grid-cols-2">{waiting.map((item, index) => <div className="flex items-center gap-4 rounded-xl border border-white/10 bg-black/20 p-4" key={item.code}><div className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-white/10 font-mono font-bold">{index + 1}</div><div className="min-w-0 flex-1"><div className="truncate font-mono text-lg font-bold">{item.patientLabel}</div><div className="mt-1 flex items-center gap-1 text-xs text-zinc-400"><Clock3 className="h-3 w-3" />Arrived {formatTime(item.arrivedAt)}</div></div><span className="text-xs font-semibold text-zinc-300">{stageLabels[item.stage] ?? item.stage}</span></div>)}</div>
      </section>
      <section className="min-w-0 rounded-2xl border border-white/15 bg-white/[.04] p-5 sm:p-6"><div className="flex items-center justify-between"><div><p className="text-xs font-bold uppercase tracking-[.2em] text-sky-400">Today</p><h2 className="mt-1 text-2xl font-bold">Upcoming appointments</h2></div><CalendarClock className="h-7 w-7 text-zinc-500" /></div><div className="mt-5 divide-y divide-white/10">{data.appointments.length ? data.appointments.slice(0, 12).map((item) => <div className="grid grid-cols-[5rem_1fr] gap-4 py-4" key={item.code}><div className="font-mono text-xl font-bold">{formatTime(item.startsAt)}</div><div className="min-w-0"><div className="truncate font-mono font-bold">{item.patientLabel}</div><div className="mt-1 text-xs capitalize text-zinc-400">{item.status}</div></div></div>) : <div className="grid min-h-40 place-items-center text-center text-zinc-400">No upcoming appointment to display.</div>}</div></section>
    </div>
    <footer className="mt-6 flex flex-wrap items-center justify-between gap-3 rounded-xl bg-white px-5 py-4 text-zinc-950"><p className="font-semibold">{data.settings.announcement}</p><div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider"><span className={`h-2.5 w-2.5 rounded-full ${connected ? "bg-emerald-500" : "bg-amber-500"}`} />{connected ? "Live" : "Reconnecting"}</div></footer>
  </main>;
}
