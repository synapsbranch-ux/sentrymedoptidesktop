import * as React from "react";
import { Check, Clock3, Phone, Play, Stethoscope, TriangleAlert, UserPlus, Wallet } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useDebouncedValue, useLoad } from "../hooks";
import { useRealtime } from "../realtime";
import type { QueueEntry } from "../types";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "./ui/dialog";
import { Badge, EmptyState, ErrorState, Skeleton } from "./ui/data";
import { Field, Input, Select } from "./ui/input";
import { patientSearchQuery, emptyPatientFilters, type PatientSearchPage, type PatientSearchResult } from "./patient-search";
import { stageAppearance, StatusPill } from "./status";

/**
 * B2: the screen the front desk keeps open all day. Everyone waiting, in arrival
 * order, with a live wait timer, why they are here and who they are seeing;
 * every stage change is a single click with no confirmation in between.
 */

/** Ordered clinical path. The next stage is always one step along it. */
export const queueStages = ["waiting_nurse", "pre_test", "waiting_doctor", "in_consultation", "checkout", "completed"] as const;
export type QueueStage = (typeof queueStages)[number];

export function nextStage(stage: string): QueueStage {
  const index = queueStages.indexOf(stage as QueueStage);
  return queueStages[Math.min(index < 0 ? 0 : index + 1, queueStages.length - 1)];
}

export function waitMinutes(since: string, now: number) {
  const arrived = new Date(since).getTime();
  if (Number.isNaN(arrived)) return 0;
  return Math.max(0, Math.floor((now - arrived) / 60000));
}

export function formatWait(minutes: number) {
  if (minutes < 60) return `${minutes} min`;
  return `${Math.floor(minutes / 60)} h ${(minutes % 60).toString().padStart(2, "0")}`;
}

/** Long waits are flagged by severity, always with a word as well as a colour. */
export function waitSeverity(minutes: number): { tone: "neutral" | "warning" | "danger"; label: string } {
  if (minutes >= 60) return { tone: "danger", label: "Waiting over an hour" };
  if (minutes >= 30) return { tone: "warning", label: "Long wait" };
  return { tone: "neutral", label: "Waiting" };
}

export function initials(name: string) {
  return name.split(" ").filter(Boolean).map((part) => part[0]).slice(0, 2).join("").toUpperCase();
}

export function WaitingRoomScreen({ onOpenPatient }: { onOpenPatient?(patientId: string): void }) {
  const { revision } = useRealtime();
  const queue = useLoad(() => api.get<{ items: QueueEntry[] }>("/queue"), [revision]);
  const [walkInOpen, setWalkInOpen] = React.useState(false);
  const [busy, setBusy] = React.useState("");
  // The wait timers count up on their own; the front desk never reloads the page.
  const [now, setNow] = React.useState(() => Date.now());
  React.useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 10000);
    return () => window.clearInterval(timer);
  }, []);
  // A safety net for the live stream: re-read the queue every minute even if no
  // SSE event arrives, so a stale board cannot go unnoticed.
  React.useEffect(() => {
    const timer = window.setInterval(() => queue.reload(), 60000);
    return () => window.clearInterval(timer);
  }, [queue.reload]);

  const advance = async (entry: QueueEntry, stage: QueueStage) => {
    setBusy(entry.id);
    try {
      await api.patch(`/queue/${entry.id}`, { stage, priority: entry.priority, assignedDoctorId: entry.assignedDoctorId, version: entry.version });
      queue.reload();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Queue update failed");
      queue.reload();
    } finally {
      setBusy("");
    }
  };

  const items = queue.data?.items ?? [];
  const waitingCount = items.filter((entry) => entry.stage !== "in_consultation" && entry.stage !== "checkout").length;

  return (
    <Card>
      <CardHeader className="flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <CardTitle>Waiting room</CardTitle>
          <CardDescription>
            {items.length === 0 ? "Nobody is waiting." : `${items.length} in the clinic · ${waitingCount} waiting to be seen. Timers update on their own.`}
          </CardDescription>
        </div>
        <Button onClick={() => setWalkInOpen(true)}><UserPlus className="h-4 w-4" />Walk-in</Button>
      </CardHeader>
      <CardContent>
        {queue.loading ? <Skeleton className="h-80" /> : queue.error ? <ErrorState message={queue.error.message} retry={queue.reload} /> : items.length === 0 ? (
          <EmptyState
            title="Waiting room is clear"
            description="Register a walk-in or check in a scheduled appointment."
            action={<Button onClick={() => setWalkInOpen(true)}><UserPlus className="h-4 w-4" />Walk-in</Button>}
          />
        ) : (
          <ol className="grid gap-3">
            {items.map((entry, position) => (
              <QueueCard
                key={entry.id}
                entry={entry}
                position={position + 1}
                now={now}
                busy={busy === entry.id}
                onAdvance={advance}
                onOpenPatient={onOpenPatient}
              />
            ))}
          </ol>
        )}
      </CardContent>
      <Dialog open={walkInOpen} onOpenChange={setWalkInOpen}>
        {walkInOpen && <WalkInForm onSaved={() => { setWalkInOpen(false); queue.reload(); }} />}
      </Dialog>
    </Card>
  );
}

function QueueCard({ entry, position, now, busy, onAdvance, onOpenPatient }: {
  entry: QueueEntry;
  position: number;
  now: number;
  busy: boolean;
  onAdvance(entry: QueueEntry, stage: QueueStage): void;
  onOpenPatient?(patientId: string): void;
}) {
  const minutes = waitMinutes(entry.arrivedAt, now);
  const severity = waitSeverity(minutes);
  const stage = stageAppearance(entry.stage);
  const following = nextStage(entry.stage);
  const walkIn = !entry.appointmentId;
  return (
    <li className={`rounded-lg border p-4 ${severity.tone === "danger" ? "border-red-300 bg-red-50/60" : severity.tone === "warning" ? "border-amber-300 bg-amber-50/50" : "border-zinc-200"}`}>
      <div className="flex items-start gap-3">
        <div className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-zinc-100 text-xs font-bold" aria-hidden>{initials(entry.patientName)}</div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs font-bold text-zinc-500">#{position}</span>
            <button className="truncate font-semibold underline-offset-2 hover:underline" onClick={() => onOpenPatient?.(entry.patientId)}>{entry.patientName}</button>
            {entry.priority > 0 && <Badge tone="danger">{entry.priority > 1 ? "Urgent" : "Priority"}</Badge>}
            {walkIn && <Badge>Walk-in</Badge>}
            {entry.source === "kiosk" && <Badge>Kiosk</Badge>}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-zinc-600">
            <span className="font-mono">{entry.medicalRecordNumber}</span>
            {entry.phone && <span className="flex items-center gap-1"><Phone aria-hidden className="h-3 w-3" />{entry.phone}</span>}
            <span className="flex items-center gap-1"><Stethoscope aria-hidden className="h-3 w-3" />{entry.assignedDoctorName || "No practitioner assigned"}</span>
          </div>
          <p className="mt-1 text-sm">{entry.visitReason || <span className="text-zinc-400">No reason recorded</span>}</p>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1.5">
          <StatusPill appearance={stage} />
          <span className={`flex items-center gap-1 font-mono text-xs font-bold ${severity.tone === "danger" ? "text-red-700" : severity.tone === "warning" ? "text-amber-700" : "text-zinc-500"}`}>
            {severity.tone !== "neutral" ? <TriangleAlert aria-hidden className="h-3 w-3" /> : <Clock3 aria-hidden className="h-3 w-3" />}
            {formatWait(minutes)}
          </span>
          <span className="sr-only">{severity.label}</span>
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button size="sm" disabled={busy} onClick={() => onAdvance(entry, following)}>
          <Play aria-hidden className="h-3 w-3" />
          {stageLabels[following]}
        </Button>
        {entry.stage !== "in_consultation" && following !== "in_consultation" && (
          <Button size="sm" variant="outline" disabled={busy} onClick={() => onAdvance(entry, "in_consultation")}>
            <Stethoscope aria-hidden className="h-3 w-3" />In consultation
          </Button>
        )}
        {entry.stage !== "checkout" && following !== "checkout" && (
          <Button size="sm" variant="outline" disabled={busy} onClick={() => onAdvance(entry, "checkout")}>
            <Wallet aria-hidden className="h-3 w-3" />Checkout
          </Button>
        )}
        {following !== "completed" && (
          <Button size="sm" variant="outline" disabled={busy} onClick={() => onAdvance(entry, "completed")}>
            <Check aria-hidden className="h-3 w-3" />Completed
          </Button>
        )}
      </div>
    </li>
  );
}

const stageLabels: Record<QueueStage, string> = {
  waiting_nurse: "Waiting for nurse",
  pre_test: "Pre-test",
  waiting_doctor: "Waiting for doctor",
  in_consultation: "In consultation",
  checkout: "Checkout",
  completed: "Completed",
};

/**
 * Name, phone and reason, with a live search over existing records so the same
 * person is not registered twice. Selecting a match queues that record instead.
 */
export function WalkInForm({ onSaved }: { onSaved(): void }) {
  const [form, setForm] = React.useState({ firstName: "", lastName: "", phone: "", reason: "", priority: 0 });
  const [saving, setSaving] = React.useState(false);
  const [duplicate, setDuplicate] = React.useState<{ patientId: string; medicalRecordNumber: string } | null>(null);
  const lookup = `${form.firstName} ${form.lastName} ${form.phone}`.trim();
  const debounced = useDebouncedValue(lookup, 300);
  const matches = useLoad(
    () => (debounced.replace(/\s+/g, "").length >= 3
      ? api.get<PatientSearchPage>(patientSearchQuery(debounced, emptyPatientFilters, 1, 5))
      : Promise.resolve({ items: [], page: 1, limit: 5, total: 0, hasMore: false } as PatientSearchPage)),
    [debounced],
  );

  const queuePatient = async (body: Record<string, unknown>) => {
    setSaving(true);
    setDuplicate(null);
    try {
      await api.post("/queue/walk-in", body);
      toast.success("Added to the waiting room");
      onSaved();
    } catch (reason) {
      if (reason instanceof APIError && reason.body.code === "POSSIBLE_DUPLICATE_PATIENT") {
        setDuplicate(reason.body.details as unknown as { patientId: string; medicalRecordNumber: string });
      } else {
        toast.error(reason instanceof APIError ? reason.body.message : "Could not add the walk-in");
      }
    } finally {
      setSaving(false);
    }
  };

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    void queuePatient(form);
  };

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Walk-in</DialogTitle>
        <DialogDescription>Name, phone and reason. Existing records appear as you type — pick one to avoid a duplicate.</DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={submit}>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="First name"><Input autoFocus required value={form.firstName} onChange={(event) => setForm({ ...form, firstName: event.target.value })} /></Field>
          <Field label="Last name"><Input required value={form.lastName} onChange={(event) => setForm({ ...form, lastName: event.target.value })} /></Field>
        </div>
        <Field label="Phone"><Input inputMode="tel" value={form.phone} onChange={(event) => setForm({ ...form, phone: event.target.value })} /></Field>
        <Field label="Reason for visit"><Input value={form.reason} onChange={(event) => setForm({ ...form, reason: event.target.value })} /></Field>
        <Field label="Priority">
          <Select value={form.priority} onChange={(event) => setForm({ ...form, priority: Number(event.target.value) })}>
            <option value={0}>Normal</option><option value={1}>Priority</option><option value={2}>Urgent</option>
          </Select>
        </Field>
        {matches.data && matches.data.items.length > 0 && (
          <div className="rounded-md border border-amber-300 bg-amber-50 p-3">
            <p className="text-xs font-semibold text-amber-900">Already registered? Select the record instead of creating a second one.</p>
            <div className="mt-2 grid gap-1">
              {matches.data.items.map((match: PatientSearchResult) => (
                <button
                  key={match.id}
                  type="button"
                  className="rounded-md bg-white/70 px-2 py-1.5 text-left text-sm hover:bg-white"
                  disabled={saving}
                  onClick={() => void queuePatient({ patientId: match.id, reason: form.reason, priority: form.priority })}
                >
                  <span className="font-semibold">{match.firstName} {match.lastName}</span>
                  <span className="ml-2 font-mono text-xs text-zinc-500">{match.medicalRecordNumber} · {match.dateOfBirth || "—"} · {match.phone || "—"}</span>
                </button>
              ))}
            </div>
          </div>
        )}
        {duplicate && (
          <div role="alert" className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
            <strong>This patient already has a record ({duplicate.medicalRecordNumber}).</strong>
            <Button className="mt-2" size="sm" disabled={saving} onClick={() => void queuePatient({ patientId: duplicate.patientId, reason: form.reason, priority: form.priority })}>
              Add that record to the waiting room
            </Button>
          </div>
        )}
        <DialogFooter>
          <Button type="submit" disabled={saving}>{saving ? "Adding…" : "Add to waiting room"}</Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}
