import * as React from "react";
import { toast } from "sonner";
import { Mic, MicOff, Save, Square, Trash2 } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { useLoad } from "../hooks";
import { useRealtime } from "../realtime";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Badge, EmptyState, Skeleton } from "./ui/data";
import { Textarea } from "./ui/input";
import { inspectMicrophoneEnvironment, type MicrophoneProblem } from "../microphone";
import { formatDuration, useRecording } from "./recording";

interface Recording {
  id: string;
  mediaType: string;
  sizeBytes: number;
  durationSeconds: number;
  consentConfirmed: boolean;
  transcriptStatus: "pending" | "processing" | "done" | "failed" | "unavailable";
  transcriptText: string;
  transcriptEdited: boolean;
  transcriptError: string;
  createdBy: string;
  createdAt: string;
}

const statusTone: Record<Recording["transcriptStatus"], "success" | "warning" | "danger" | "neutral"> = {
  pending: "neutral",
  processing: "warning",
  done: "success",
  failed: "danger",
  unavailable: "neutral",
};

const statusLabel: Record<Recording["transcriptStatus"], string> = {
  pending: "Queued",
  processing: "Transcribing…",
  done: "Transcribed",
  failed: "Transcription failed",
  unavailable: "No transcription engine configured",
};

export function ConsultationRecorder({ canRecord }: { canRecord: boolean }) {
  const { user } = useAuth();
  const { revision } = useRealtime();
  const {
    encounterId, recording, paused, uploading, elapsed, problem, recovered, retentionDays,
    start, stop, saveRecovered, discardRecovered, onSaved,
  } = useRecording();
  const recordings = useLoad(() => api.get<{ items: Recording[] }>(`/encounters/${encounterId}/recordings`), [encounterId, revision]);
  const [consent, setConsent] = React.useState(false);

  // The widget can stop a recording from anywhere on the consultation screen, so
  // the list refreshes when the provider reports a saved upload, not on a click.
  React.useEffect(() => onSaved(() => { setConsent(false); recordings.reload(); }), [onSaved, recordings.reload]);

  const environmentProblem = canRecord ? inspectMicrophoneEnvironment() : null;
  const blocking = problem ?? environmentProblem;

  return (
    <div className="grid gap-4">
      {canRecord && (
        <Card>
          <CardHeader>
            <CardTitle>Record this consultation</CardTitle>
            <CardDescription>Audio stays on this computer. Nothing is sent to the internet or a cloud service.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            {recovered && (
              <div role="alert" className="grid gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
                <strong>A recording from this consultation was interrupted.</strong>
                <p>{formatDuration(recovered.durationSeconds)} of audio is still on this device. Nothing has been lost.</p>
                <p className="text-xs">Audio waiting to be recovered is kept on this device for {retentionDays} days and then deleted, so it is not left on a shared tablet.</p>
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" disabled={uploading} onClick={() => void saveRecovered()}><Save className="h-3.5 w-3.5" />{uploading ? "Saving…" : "Save it to the chart"}</Button>
                  <Button size="sm" variant="outline" disabled={uploading} onClick={() => void discardRecovered()}>Discard</Button>
                </div>
              </div>
            )}
            {!recording ? (
              <>
                {blocking && <MicrophoneNotice problem={blocking} />}
                <label className="flex min-h-11 items-start gap-3 rounded-md border p-3 text-sm">
                  <input type="checkbox" className="mt-0.5" checked={consent} onChange={(event) => setConsent(event.target.checked)} />
                  <span>The patient has been informed that this consultation may be recorded for clinical documentation, and consents.</span>
                </label>
                <Button disabled={!consent || uploading || Boolean(environmentProblem)} onClick={() => void start()}>
                  <Mic className="h-4 w-4" />
                  {uploading ? "Saving previous recording…" : "Start recording"}
                </Button>
              </>
            ) : (
              <div className="flex flex-wrap items-center gap-4">
                <span className="flex items-center gap-2 font-mono text-sm font-bold text-red-700">
                  <span className={`h-2.5 w-2.5 rounded-full bg-red-600 ${paused ? "" : "animate-pulse"}`} />
                  {paused ? "Paused" : "Recording"} {formatDuration(elapsed)}
                </span>
                <Button variant="outline" onClick={stop}>
                  <Square className="h-4 w-4" />
                  Stop and save
                </Button>
                <p className="w-full text-xs text-zinc-500">The floating widget keeps these controls to hand anywhere on this screen.</p>
              </div>
            )}
          </CardContent>
        </Card>
      )}
      <Card>
        <CardHeader>
          <CardTitle>Recordings & transcripts</CardTitle>
          <CardDescription>Every transcript is a draft — review and correct it before relying on it clinically.</CardDescription>
        </CardHeader>
        <CardContent>
          {recordings.loading ? (
            <Skeleton className="h-40" />
          ) : recordings.data?.items.length ? (
            <div className="grid gap-4">
              {recordings.data.items.map((item) => (
                <RecordingRow key={item.id} recording={item} canDelete={user?.role === "doctor"} onChanged={recordings.reload} />
              ))}
            </div>
          ) : (
            <EmptyState title="No recordings yet" description="Recordings made during this consultation will appear here." />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function MicrophoneNotice({ problem }: { problem: MicrophoneProblem }) {
  return (
    <div role="alert" className="grid gap-1.5 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
      <div className="flex items-center gap-2 font-semibold">
        <MicOff className="h-4 w-4 shrink-0" />
        Recording is not available on this device yet
      </div>
      <p>{problem.message}</p>
      <p className="text-xs opacity-90">{problem.guidance}</p>
    </div>
  );
}

function RecordingRow({ recording, canDelete, onChanged }: { recording: Recording; canDelete: boolean; onChanged(): void }) {
  const [text, setText] = React.useState(recording.transcriptText);
  const [saving, setSaving] = React.useState(false);
  React.useEffect(() => setText(recording.transcriptText), [recording.transcriptText]);

  const saveCorrection = async () => {
    setSaving(true);
    try {
      await api.put(`/recordings/${recording.id}/transcript`, { text });
      toast.success("Transcript correction saved");
      onChanged();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Could not save the correction");
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    try {
      await api.delete(`/recordings/${recording.id}`);
      toast.success("Recording deleted");
      onChanged();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Could not delete the recording");
    }
  };

  return (
    <div className="rounded-lg border p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm text-zinc-500">
          {new Date(recording.createdAt).toLocaleString()} · {formatDuration(recording.durationSeconds)} · {recording.createdBy}
        </div>
        <div className="flex items-center gap-2">
          <Badge tone={statusTone[recording.transcriptStatus]}>{statusLabel[recording.transcriptStatus]}</Badge>
          {canDelete && (
            <Button aria-label="Delete recording" size="icon" variant="ghost" onClick={remove}>
              <Trash2 className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>
      <audio className="mt-3 w-full" controls src={`/api/v1/recordings/${recording.id}/audio`} />
      {recording.transcriptStatus === "done" && (
        <div className="mt-3 grid gap-2">
          <Textarea rows={4} value={text} onChange={(event) => setText(event.target.value)} />
          <div className="flex items-center justify-between">
            {recording.transcriptEdited && <span className="text-xs text-zinc-400">Edited by a clinician</span>}
            <Button className="ml-auto" size="sm" disabled={saving || text === recording.transcriptText} onClick={saveCorrection}>
              <Save className="h-3.5 w-3.5" />
              {saving ? "Saving…" : "Save correction"}
            </Button>
          </div>
        </div>
      )}
      {recording.transcriptStatus === "failed" && recording.transcriptError && (
        <p className="mt-3 rounded-md bg-red-50 p-3 text-xs text-red-700">{recording.transcriptError}</p>
      )}
      {recording.transcriptStatus === "unavailable" && (
        <p className="mt-3 text-xs text-zinc-400">No local transcription engine is configured. Set one up in System → Clinic.</p>
      )}
    </div>
  );
}
