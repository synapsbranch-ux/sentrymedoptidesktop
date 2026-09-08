import * as React from "react";
import { toast } from "sonner";
import { Mic, MicOff, Save, Square, Trash2 } from "lucide-react";
import { api, APIError } from "../api";
import { useAuth } from "../auth";
import { useLoad } from "../hooks";
import { useRealtime } from "../realtime";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Badge, EmptyState, Skeleton } from "./ui/data";
import { Textarea } from "./ui/input";
import { inspectMicrophoneEnvironment, requestMicrophone, type MicrophoneProblem } from "../microphone";

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

const candidateMimeTypes = ["audio/webm;codecs=opus", "audio/webm", "audio/ogg;codecs=opus", "audio/mp4"];

function pickMimeType(): string | null {
  if (typeof MediaRecorder === "undefined") return null;
  return candidateMimeTypes.find((type) => MediaRecorder.isTypeSupported(type)) ?? null;
}

function formatDuration(totalSeconds: number) {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
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

export function ConsultationRecorder({ encounterId, canRecord }: { encounterId: string; canRecord: boolean }) {
  const { user } = useAuth();
  const { revision } = useRealtime();
  const recordings = useLoad(() => api.get<{ items: Recording[] }>(`/encounters/${encounterId}/recordings`), [encounterId, revision]);
  const [consent, setConsent] = React.useState(false);
  const [recording, setRecording] = React.useState(false);
  const [elapsed, setElapsed] = React.useState(0);
  const [uploading, setUploading] = React.useState(false);
  const [problem, setProblem] = React.useState<MicrophoneProblem | null>(null);
  const mediaRecorderRef = React.useRef<MediaRecorder | null>(null);
  const chunksRef = React.useRef<Blob[]>([]);
  const streamRef = React.useRef<MediaStream | null>(null);
  const timerRef = React.useRef<number | null>(null);
  // `recorder.onstop` is assigned once, so it would otherwise close over the
  // elapsed value from the render that started the recording — always zero.
  const elapsedRef = React.useRef(0);

  // Report an environment that cannot record before staff confirm consent and
  // press a button that was never going to work.
  const environmentProblem = React.useMemo(() => (canRecord ? inspectMicrophoneEnvironment() : null), [canRecord]);
  const blocking = problem ?? environmentProblem;

  React.useEffect(() => () => {
    if (timerRef.current) window.clearInterval(timerRef.current);
    streamRef.current?.getTracks().forEach((track) => track.stop());
  }, []);

  const startRecording = async () => {
    const mimeType = pickMimeType();
    if (!mimeType) {
      setProblem({ reason: "unsupported_browser", message: "This browser cannot record audio (MediaRecorder is unavailable).", guidance: "Use Chrome, Edge or Safari, kept up to date, or record from the SentryMed desktop application." });
      return;
    }
    const result = await requestMicrophone();
    if ("problem" in result) {
      setProblem(result.problem);
      return;
    }
    setProblem(null);
    streamRef.current = result.stream;
    const recorder = new MediaRecorder(result.stream, { mimeType });
    chunksRef.current = [];
    recorder.ondataavailable = (event) => { if (event.data.size > 0) chunksRef.current.push(event.data); };
    recorder.onstop = () => {
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
      void uploadRecording(mimeType, elapsedRef.current);
    };
    mediaRecorderRef.current = recorder;
    // A one-second timeslice keeps completed chunks in hand, so an interrupted
    // recording still has everything captured up to the interruption.
    recorder.start(1000);
    setRecording(true);
    setElapsed(0);
    elapsedRef.current = 0;
    timerRef.current = window.setInterval(() => { elapsedRef.current += 1; setElapsed(elapsedRef.current); }, 1000);
  };

  const stopRecording = () => {
    // The tracks are stopped in `onstop`; stopping them here can truncate the
    // final chunk before the recorder has flushed it.
    mediaRecorderRef.current?.stop();
    if (timerRef.current) { window.clearInterval(timerRef.current); timerRef.current = null; }
    setRecording(false);
  };

  const uploadRecording = async (mimeType: string, durationSeconds: number) => {
    const blob = new Blob(chunksRef.current, { type: mimeType });
    if (blob.size === 0) {
      toast.error("The recording was empty and was not saved.");
      return;
    }
    setUploading(true);
    try {
      const form = new FormData();
      form.append("consentConfirmed", "true");
      form.append("durationSeconds", String(durationSeconds));
      const extension = mimeType.includes("mp4") ? "m4a" : mimeType.includes("ogg") ? "ogg" : "webm";
      form.append("audio", blob, `consultation.${extension}`);
      await api.post(`/encounters/${encounterId}/recordings`, form);
      toast.success("Recording saved. Transcription will appear here shortly.");
      setConsent(false);
      recordings.reload();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Could not save the recording.");
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className="grid gap-4">
      {canRecord && (
        <Card>
          <CardHeader>
            <CardTitle>Record this consultation</CardTitle>
            <CardDescription>Audio stays on this computer. Nothing is sent to the internet or a cloud service.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            {!recording ? (
              <>
                {blocking && <MicrophoneNotice problem={blocking} />}
                <label className="flex min-h-11 items-start gap-3 rounded-md border p-3 text-sm">
                  <input type="checkbox" className="mt-0.5" checked={consent} onChange={(event) => setConsent(event.target.checked)} />
                  <span>The patient has been informed that this consultation may be recorded for clinical documentation, and consents.</span>
                </label>
                <Button disabled={!consent || uploading || Boolean(environmentProblem)} onClick={startRecording}>
                  <Mic className="h-4 w-4" />
                  {uploading ? "Saving previous recording…" : "Start recording"}
                </Button>
              </>
            ) : (
              <div className="flex items-center gap-4">
                <span className="flex items-center gap-2 font-mono text-sm font-bold text-red-700">
                  <span className="h-2.5 w-2.5 animate-pulse rounded-full bg-red-600" />
                  {formatDuration(elapsed)}
                </span>
                <Button variant="outline" onClick={stopRecording}>
                  <Square className="h-4 w-4" />
                  Stop and save
                </Button>
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
