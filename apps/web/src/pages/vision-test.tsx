import * as React from "react";
import { toast } from "sonner";
import { Check, ChevronDown, ChevronUp, Monitor, Plus, Shuffle, X } from "lucide-react";
import { api, APIError } from "../api";
import { useLoad } from "../hooks";
import { useRealtime } from "../realtime";
import { cn } from "../lib";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Badge, EmptyState, ErrorState, Skeleton } from "../components/ui/data";
import { Field, Input, Select } from "../components/ui/input";
import { AmslerSurface, type AmslerMark } from "../components/vision-surfaces";
import { buildChartLine, metricFromLogMar, plateDigits, snellenFromLogMar, stepLogMar, type Optotype } from "../vision";

interface DisplayInfo { distanceMm: number; pixelsPerMm: number; calibratedAt: string; label: string }
interface Result { eye: string; correction: string; logMar: number; snellen: string; test: string; detail: string; recordedAt: string }
interface VisionState {
  mode: "blank" | "acuity" | "colour" | "amsler" | "fixation";
  eye: "OD" | "OS" | "OU";
  correction: "uncorrected" | "corrected" | "pinhole";
  logMar: number;
  optotype: Optotype;
  seed: number;
  singleLine: boolean;
  plate: number;
  display: DisplayInfo;
  results: Result[];
  amsler: Record<string, AmslerMark[]>;
}
interface Session { id: string; room: string; encounterId: string; patientName: string; state: VisionState; revision: number; updatedAt: string }

const optotypeLabels: Record<Optotype, string> = {
  sloan: "Sloan letters",
  numbers: "Numbers",
  landolt: "Landolt C",
  tumblingE: "Tumbling E",
};

export function VisionTestPage() {
  const { revision } = useRealtime();
  const sessions = useLoad(() => api.get<{ items: Session[] }>("/vision-tests"), [revision]);
  const [activeId, setActiveId] = React.useState<string | null>(null);
  const [room, setRoom] = React.useState("");
  const [encounterId, setEncounterId] = React.useState("");
  const encounters = useLoad(() => api.get<{ items: { id: string; encounterNumber: string; patientName: string; status: string }[] }>("/encounters?status=draft"), []);

  const open = async () => {
    if (!room.trim()) return;
    try {
      const created = await api.post<{ id: string }>("/vision-tests", { room: room.trim(), encounterId });
      setRoom("");
      setEncounterId("");
      setActiveId(created.id);
      sessions.reload();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Could not open the session");
    }
  };

  if (sessions.loading && !sessions.data) return <div className="page"><Skeleton className="h-96" /></div>;
  if (sessions.error) return <div className="page"><ErrorState message={sessions.error.message} retry={sessions.reload} /></div>;
  const items = sessions.data?.items ?? [];
  const active = items.find((session) => session.id === activeId) ?? null;

  return (
    <div className="page grid gap-5">
      <div>
        <p className="text-[11px] font-bold uppercase tracking-wide text-zinc-500">Vision testing</p>
        <h1 className="text-2xl font-bold">Digital acuity chart</h1>
        <p className="mt-1 text-sm text-zinc-500">
          Open the chart on the screen in the lane, then drive it from this device. The screen is calibrated on the
          screen itself, so a measurement is only as good as that calibration.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Open a session</CardTitle>
          <CardDescription>One session per lane. Link a consultation to write the result straight into its pre-test.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
          <Field label="Room or lane"><Input value={room} onChange={(event) => setRoom(event.target.value)} placeholder="Lane 1" /></Field>
          <Field label="Consultation (optional)">
            <Select value={encounterId} onChange={(event) => setEncounterId(event.target.value)}>
              <option value="">Not linked</option>
              {(encounters.data?.items ?? []).map((encounter) => (
                <option key={encounter.id} value={encounter.id}>{encounter.encounterNumber} · {encounter.patientName}</option>
              ))}
            </Select>
          </Field>
          <Button onClick={open} disabled={!room.trim()}><Plus className="h-4 w-4" />Open</Button>
        </CardContent>
      </Card>

      {items.length === 0 ? (
        <EmptyState title="No open session" description="Open a session for a lane, then load the display on that screen." />
      ) : (
        <div className="grid gap-3">
          {items.map((session) => (
            <SessionCard
              key={session.id}
              session={session}
              expanded={session.id === activeId}
              onToggle={() => setActiveId(session.id === activeId ? null : session.id)}
              onChanged={sessions.reload}
            />
          ))}
        </div>
      )}
      {active && <p className="text-xs text-zinc-400">Session {active.revision} · updated {new Date(active.updatedAt).toLocaleTimeString()}</p>}
    </div>
  );
}

function SessionCard({ session, expanded, onToggle, onChanged }: { session: Session; expanded: boolean; onToggle(): void; onChanged(): void }) {
  const [busy, setBusy] = React.useState(false);
  const state = session.state;
  const calibrated = state.display.pixelsPerMm > 0 && state.display.distanceMm > 0;

  const push = async (changes: Partial<VisionState>) => {
    setBusy(true);
    try {
      await api.put(`/vision-tests/${session.id}/state`, { state: { ...state, ...changes } });
      onChanged();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Could not reach the display");
    } finally {
      setBusy(false);
    }
  };

  const record = async () => {
    if (!calibrated) return;
    const result: Result = {
      eye: state.eye,
      correction: state.correction,
      logMar: state.logMar,
      snellen: snellenFromLogMar(state.logMar),
      test: "acuity",
      detail: `${optotypeLabels[state.optotype]} at ${(state.display.distanceMm / 1000).toFixed(2)} m`,
      recordedAt: new Date().toISOString(),
    };
    await push({ results: [...state.results, result] });
    toast.success(`${state.eye} ${state.correction} ${result.snellen} recorded`);
  };

  const apply = async () => {
    setBusy(true);
    try {
      const outcome = await api.post<{ applied: string[] }>(`/vision-tests/${session.id}/apply`, {});
      toast.success(`Written to the pre-test: ${outcome.applied.join(", ")}`);
    } catch (reason) {
      if (reason instanceof APIError && reason.isConflict) toast.error("The pre-test changed while testing. Reopen it and try again.");
      else toast.error(reason instanceof Error ? reason.message : "Could not write to the pre-test");
    } finally {
      setBusy(false);
    }
  };

  const close = async () => {
    setBusy(true);
    try {
      await api.post(`/vision-tests/${session.id}/close`, {});
      onChanged();
    } finally {
      setBusy(false);
    }
  };

  const preview = buildChartLine(state.optotype, state.seed, state.logMar, state.singleLine ? 1 : 5);

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><Monitor className="h-4 w-4" />{session.room}</CardTitle>
            <CardDescription>
              {session.patientName ? `Linked to ${session.patientName}` : "Not linked to a consultation"}
              {" · "}
              {calibrated
                ? `${(state.display.distanceMm / 1000).toFixed(2)} m, ${state.display.pixelsPerMm.toFixed(2)} px/mm`
                : "screen not calibrated yet"}
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            {!calibrated && <Badge tone="warning">Awaiting screen</Badge>}
            <Button size="sm" variant="outline" onClick={onToggle}>{expanded ? "Hide" : "Drive"}</Button>
          </div>
        </div>
      </CardHeader>
      {expanded && (
        <CardContent className="grid gap-4">
          <div className="rounded-md border bg-zinc-50 p-3">
            <p className="text-[11px] font-bold uppercase tracking-wide text-zinc-500">Open on the lane screen</p>
            <p className="mt-1 break-all font-mono text-xs">{window.location.origin}/vision-display/{session.id}</p>
          </div>

          {!calibrated && (
            <p className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
              This screen has not reported a calibration yet. Open the link above on the lane screen and calibrate it
              there — results cannot be recorded until it has.
            </p>
          )}

          <div className="flex flex-wrap gap-1 rounded-md border p-1">
            {(["acuity", "colour", "amsler", "fixation", "blank"] as const).map((mode) => (
              <button
                key={mode}
                type="button"
                aria-pressed={state.mode === mode}
                disabled={busy}
                onClick={() => void push({ mode })}
                className={cn(
                  "min-h-11 flex-1 rounded px-3 text-sm font-semibold capitalize transition-colors sm:min-h-9",
                  state.mode === mode ? "bg-zinc-900 text-white" : "text-zinc-600 hover:bg-zinc-100",
                )}
              >
                {mode === "colour" ? "Colour" : mode}
              </button>
            ))}
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="Eye">
              <Select value={state.eye} onChange={(event) => void push({ eye: event.target.value as VisionState["eye"] })}>
                <option value="OD">OD — right</option>
                <option value="OS">OS — left</option>
                <option value="OU">OU — both</option>
              </Select>
            </Field>
            <Field label="Correction">
              <Select value={state.correction} onChange={(event) => void push({ correction: event.target.value as VisionState["correction"] })}>
                <option value="uncorrected">Uncorrected</option>
                <option value="corrected">With correction</option>
                <option value="pinhole">Pinhole</option>
              </Select>
            </Field>
            <Field label="Optotype">
              <Select disabled={state.mode !== "acuity"} value={state.optotype} onChange={(event) => void push({ optotype: event.target.value as Optotype })}>
                {(Object.keys(optotypeLabels) as Optotype[]).map((key) => <option key={key} value={key}>{optotypeLabels[key]}</option>)}
              </Select>
            </Field>
          </div>

          {state.mode === "acuity" && (
          <>
          <p className="-mt-1 text-xs text-zinc-500">
            Letters and digits are measured glyph by glyph and scaled to the exact height the acuity level calls for,
            but they use this screen&rsquo;s own font, whose stroke width is not a certified Sloan typeface. The Landolt C
            and the tumbling E are drawn to exact five-by-five proportions — prefer them when the result must be
            comparable with a printed chart.
          </p>
          <div className="rounded-lg border p-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-3xl font-bold">{snellenFromLogMar(state.logMar)}</div>
                <div className="font-mono text-xs text-zinc-500">
                  {metricFromLogMar(state.logMar)} · logMAR {state.logMar.toFixed(2)}
                </div>
              </div>
              <div className="flex gap-2">
                <Button size="icon" variant="outline" aria-label="Larger line" disabled={busy} onClick={() => void push({ logMar: stepLogMar(state.logMar, 1) })}><ChevronDown className="h-4 w-4" /></Button>
                <Button size="icon" variant="outline" aria-label="Smaller line" disabled={busy} onClick={() => void push({ logMar: stepLogMar(state.logMar, -1) })}><ChevronUp className="h-4 w-4" /></Button>
              </div>
            </div>
            <div className="mt-3 flex items-center gap-2 font-mono text-lg tracking-[0.3em]">
              {preview.map((entry, index) => (
                <span key={index} className={cn(state.optotype === "landolt" || state.optotype === "tumblingE" ? "inline-block" : undefined)} style={state.optotype === "landolt" || state.optotype === "tumblingE" ? { transform: `rotate(${entry.angle}deg)` } : undefined}>
                  {entry.glyph}
                </span>
              ))}
            </div>
            <p className="mt-1 text-[11px] text-zinc-400">What the patient is seeing right now.</p>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" disabled={busy} onClick={() => void push({ seed: Math.floor(Math.random() * 1_000_000) })}><Shuffle className="h-4 w-4" />New letters</Button>
            <Button size="sm" variant="outline" disabled={busy} onClick={() => void push({ singleLine: !state.singleLine })}>{state.singleLine ? "Show full line" : "Isolate one optotype"}</Button>
          </div>
          </>
          )}

          {state.mode === "colour" && <ColourPanel state={state} busy={busy} push={push} />}
          {state.mode === "amsler" && <AmslerPanel session={session} state={state} busy={busy} push={push} />}

          <div className="flex flex-wrap items-center gap-2 border-t pt-4">
            {state.mode === "acuity" && <Button size="sm" disabled={busy || !calibrated} onClick={record}><Check className="h-4 w-4" />Record {state.eye} {snellenFromLogMar(state.logMar)}</Button>}
            {session.encounterId && <Button size="sm" variant="outline" disabled={busy || state.results.length === 0} onClick={apply}>Write to pre-test</Button>}
            <Button size="sm" variant="ghost" className="ml-auto" disabled={busy} onClick={close}><X className="h-4 w-4" />Close session</Button>
          </div>

          {state.results.length > 0 && (
            <ul className="grid gap-1 text-sm">
              {state.results.map((result, index) => (
                <li key={index} className="flex items-center gap-3 rounded-md border p-2">
                  <Badge>{result.eye}</Badge>
                  <span className="capitalize text-zinc-500">{result.correction}</span>
                  <strong className="font-mono">{result.snellen}</strong>
                  <span className="ml-auto text-xs text-zinc-400">{new Date(result.recordedAt).toLocaleTimeString()}</span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      )}
    </Card>
  );
}


/**
 * The examiner needs the answer before the patient speaks, and the patient must never
 * see it — so the number lives here, on the phone, and never on the lane screen.
 */
function ColourPanel({ state, busy, push }: { state: VisionState; busy: boolean; push(changes: Partial<VisionState>): Promise<void> }) {
  const answer = plateDigits(state.seed, state.plate);
  const record = async (correct: boolean) => {
    const result: Result = {
      eye: state.eye,
      correction: state.correction,
      logMar: 0,
      snellen: "",
      test: "colour",
      detail: `Plate ${state.plate}: expected ${answer}, ${correct ? "read correctly" : "misread or not seen"}`,
      recordedAt: new Date().toISOString(),
    };
    await push({ results: [...state.results, result] });
    toast.success(`Plate ${state.plate} recorded`);
  };
  return (
    <div className="grid gap-3 rounded-lg border p-4">
      <p className="rounded-md border border-zinc-200 bg-zinc-50 p-2 text-xs text-zinc-600">
        Generated screening plates, not the Ishihara test. They flag a likely red-green deficiency for referral; they do
        not classify or grade one, and a normal result does not rule out a subtle defect.
      </p>
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="text-[11px] font-bold uppercase tracking-wide text-zinc-500">Plate {state.plate} shows</div>
          <div className="font-mono text-4xl font-bold">{answer}</div>
        </div>
        <div className="flex gap-2">
          <Button size="icon" variant="outline" aria-label="Previous plate" disabled={busy || state.plate <= 1} onClick={() => void push({ plate: state.plate - 1 })}><ChevronDown className="h-4 w-4" /></Button>
          <Button size="icon" variant="outline" aria-label="Next plate" disabled={busy || state.plate >= 24} onClick={() => void push({ plate: state.plate + 1 })}><ChevronUp className="h-4 w-4" /></Button>
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={busy} onClick={() => void record(true)}><Check className="h-4 w-4" />Read correctly</Button>
        <Button size="sm" variant="outline" disabled={busy} onClick={() => void record(false)}>Misread</Button>
        <Button size="sm" variant="ghost" disabled={busy} onClick={() => void push({ seed: Math.floor(Math.random() * 1_000_000), plate: 1 })}>New plate set</Button>
      </div>
    </div>
  );
}

/**
 * The patient traces on the lane screen; the doctor reviews it here and decides whether
 * it enters the record, which is also why the marks are not written to the chart as the
 * patient makes them.
 */
function AmslerPanel({ session, state, busy, push }: { session: Session; state: VisionState; busy: boolean; push(changes: Partial<VisionState>): Promise<void> }) {
  const [saving, setSaving] = React.useState(false);
  const eye = state.eye === "OU" ? "OD" : state.eye;
  const marks = state.amsler?.[eye] ?? [];

  const saveToRecord = async () => {
    setSaving(true);
    try {
      const charts = await api.get<{ charts: Record<string, Record<string, { version: number }>> }>(`/encounters/${session.encounterId}/eye-diagrams`);
      const version = charts.charts.amsler?.[eye]?.version ?? 0;
      await api.put(`/encounters/${session.encounterId}/eye-diagrams/amsler/${eye}`, {
        annotations: marks,
        notes: marks.length === 0 ? "Amsler grid reported as normal." : `Patient marked ${marks.length} distorted areas.`,
        version,
      });
      toast.success(`Amsler ${eye} saved to the consultation`);
    } catch (reason) {
      if (reason instanceof APIError && reason.isConflict) toast.error("This Amsler chart changed since it was opened. Reopen the consultation and try again.");
      else toast.error(reason instanceof Error ? reason.message : "Could not save the Amsler grid");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="grid gap-3 rounded-lg border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="text-[11px] font-bold uppercase tracking-wide text-zinc-500">Patient tracing · {eye}</div>
          <div className="text-sm text-zinc-600">{marks.length === 0 ? "Nothing marked yet" : `${marks.length} areas marked`}</div>
        </div>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" disabled={busy || marks.length === 0} onClick={() => void push({ amsler: { ...state.amsler, [eye]: [] } })}>Clear</Button>
          {session.encounterId && <Button size="sm" disabled={saving || busy} onClick={saveToRecord}>{saving ? "Saving…" : "Save to record"}</Button>}
        </div>
      </div>
      <div className="justify-self-center">
        <AmslerSurface marks={marks} sizePx={260} readOnly inverted={false} />
      </div>
      {!session.encounterId && <p className="text-xs text-zinc-500">Link this session to a consultation to save the grid into the chart.</p>}
    </div>
  );
}
