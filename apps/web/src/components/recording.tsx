import * as React from "react";
import { createPortal } from "react-dom";
import { toast } from "sonner";
import { ChevronDown, ChevronUp, GripVertical, Mic, Pause, Play, Square } from "lucide-react";
import { api, APIError } from "../api";
import { clearRecovered, loadRecovered, persistChunk, purgeExpiredRecordings, recordingRetentionMs, type RecoveredRecording } from "../recording-store";
import { requestMicrophone, type MicrophoneProblem } from "../microphone";
import { Button } from "./ui/button";

/**
 * D5: the recording lives above the consultation screen, not inside one tab of
 * it. The state sits in this provider so that switching tabs, scrolling or
 * opening another panel cannot interrupt or hide a recording in progress, and
 * the widget is portalled to the body so the consultation dialog's own transform
 * cannot clip it.
 */

const candidateMimeTypes = ["audio/webm;codecs=opus", "audio/webm", "audio/ogg;codecs=opus", "audio/mp4"];

export function pickMimeType(): string | null {
  if (typeof MediaRecorder === "undefined") return null;
  return candidateMimeTypes.find((type) => MediaRecorder.isTypeSupported(type)) ?? null;
}

export function formatDuration(totalSeconds: number) {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = Math.max(0, Math.floor(totalSeconds % 60));
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

export function extensionFor(mimeType: string) {
  return mimeType.includes("mp4") ? "m4a" : mimeType.includes("ogg") ? "ogg" : "webm";
}

interface RecordingValue {
  encounterId: string;
  patientName: string;
  recording: boolean;
  paused: boolean;
  uploading: boolean;
  elapsed: number;
  problem: MicrophoneProblem | null;
  canPause: boolean;
  recovered: RecoveredRecording | null;
  retentionDays: number;
  start(): Promise<void>;
  pause(): void;
  resume(): void;
  stop(): void;
  saveRecovered(): Promise<void>;
  discardRecovered(): Promise<void>;
  onSaved(listener: () => void): () => void;
}

const RecordingContext = React.createContext<RecordingValue | null>(null);

export function useRecording() {
  const value = React.useContext(RecordingContext);
  if (!value) throw new Error("useRecording must be used inside RecordingProvider");
  return value;
}

export function RecordingProvider({ encounterId, patientName, children }: { encounterId: string; patientName: string; children: React.ReactNode }) {
  const [recording, setRecording] = React.useState(false);
  const [paused, setPaused] = React.useState(false);
  const [uploading, setUploading] = React.useState(false);
  const [elapsed, setElapsed] = React.useState(0);
  const [problem, setProblem] = React.useState<MicrophoneProblem | null>(null);
  const [canPause, setCanPause] = React.useState(false);
  const [recovered, setRecovered] = React.useState<RecoveredRecording | null>(null);

  const recorderRef = React.useRef<MediaRecorder | null>(null);
  const streamRef = React.useRef<MediaStream | null>(null);
  const chunksRef = React.useRef<Blob[]>([]);
  const timerRef = React.useRef<number | null>(null);
  const elapsedRef = React.useRef(0);
  const listenersRef = React.useRef(new Set<() => void>());

  const onSaved = React.useCallback((listener: () => void) => {
    listenersRef.current.add(listener);
    return () => { listenersRef.current.delete(listener); };
  }, []);
  const announceSaved = React.useCallback(() => { listenersRef.current.forEach((listener) => listener()); }, []);

  // Audio captured before an interruption is offered back the next time this
  // consultation is opened.
  React.useEffect(() => {
    let active = true;
    // Opening any consultation also sweeps audio nobody came back for, so a
    // recording is not left on a shared tablet because its own consultation was
    // never reopened.
    void purgeExpiredRecordings()
      .then(() => loadRecovered(encounterId))
      .then((found) => { if (active) setRecovered(found); });
    return () => { active = false; };
  }, [encounterId]);

  const stopTimer = React.useCallback(() => {
    if (timerRef.current) { window.clearInterval(timerRef.current); timerRef.current = null; }
  }, []);

  const upload = React.useCallback(async (blob: Blob, mimeType: string, durationSeconds: number) => {
    if (blob.size === 0) {
      toast.error("The recording was empty and was not saved.");
      await clearRecovered(encounterId);
      setRecovered(null);
      return;
    }
    setUploading(true);
    try {
      const form = new FormData();
      form.append("consentConfirmed", "true");
      form.append("durationSeconds", String(Math.round(durationSeconds)));
      form.append("audio", blob, `consultation.${extensionFor(mimeType)}`);
      await api.post(`/encounters/${encounterId}/recordings`, form);
      await clearRecovered(encounterId);
      setRecovered(null);
      toast.success("Recording saved. Transcription will appear here shortly.");
      announceSaved();
    } catch (reason) {
      // The local copy is deliberately kept so the audio can be retried.
      toast.error(reason instanceof APIError ? reason.body.message : "Could not save the recording. The audio is kept on this device and can be recovered.");
      void loadRecovered(encounterId).then(setRecovered);
    } finally {
      setUploading(false);
    }
  }, [encounterId, announceSaved]);

  const start = React.useCallback(async () => {
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
    await clearRecovered(encounterId);
    setRecovered(null);
    recorder.ondataavailable = (event) => {
      if (event.data.size === 0) return;
      chunksRef.current.push(event.data);
      void persistChunk(encounterId, event.data, mimeType, elapsedRef.current);
    };
    recorder.onstop = () => {
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
      void upload(new Blob(chunksRef.current, { type: mimeType }), mimeType, elapsedRef.current);
    };
    recorderRef.current = recorder;
    setCanPause(typeof recorder.pause === "function" && typeof recorder.resume === "function");
    // One-second chunks: an interruption can then lose at most the last second.
    recorder.start(1000);
    elapsedRef.current = 0;
    setElapsed(0);
    setPaused(false);
    setRecording(true);
    timerRef.current = window.setInterval(() => { elapsedRef.current += 1; setElapsed(elapsedRef.current); }, 1000);
  }, [encounterId, upload]);

  const pause = React.useCallback(() => {
    recorderRef.current?.pause();
    stopTimer();
    setPaused(true);
  }, [stopTimer]);

  const resume = React.useCallback(() => {
    recorderRef.current?.resume();
    setPaused(false);
    timerRef.current = window.setInterval(() => { elapsedRef.current += 1; setElapsed(elapsedRef.current); }, 1000);
  }, []);

  const stop = React.useCallback(() => {
    recorderRef.current?.stop();
    stopTimer();
    setRecording(false);
    setPaused(false);
  }, [stopTimer]);

  const saveRecovered = React.useCallback(async () => {
    if (!recovered) return;
    await upload(new Blob(recovered.chunks, { type: recovered.mimeType }), recovered.mimeType, recovered.durationSeconds);
  }, [recovered, upload]);

  const discardRecovered = React.useCallback(async () => {
    await clearRecovered(encounterId);
    setRecovered(null);
  }, [encounterId]);

  // Leaving with a recording running would discard the audio held in memory.
  React.useEffect(() => {
    if (!recording) return;
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ""; };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [recording]);

  React.useEffect(() => () => {
    if (timerRef.current) window.clearInterval(timerRef.current);
    streamRef.current?.getTracks().forEach((track) => track.stop());
  }, []);

  const value = React.useMemo<RecordingValue>(() => ({
    encounterId, patientName, recording, paused, uploading, elapsed, problem, canPause, recovered,
    retentionDays: Math.round(recordingRetentionMs / 86_400_000),
    start, pause, resume, stop, saveRecovered, discardRecovered, onSaved,
  }), [encounterId, patientName, recording, paused, uploading, elapsed, problem, canPause, recovered, start, pause, resume, stop, saveRecovered, discardRecovered, onSaved]);

  return <RecordingContext.Provider value={value}>{children}</RecordingContext.Provider>;
}

export const widgetWidth = 240;
export const widgetHeaderHeight = 64;

/** Keeps the widget inside the viewport, so it can never be dragged out of reach. */
export function clampWidgetPosition(x: number, y: number, viewport: { width: number; height: number }) {
  return {
    x: Math.min(Math.max(0, x), Math.max(0, viewport.width - widgetWidth)),
    y: Math.min(Math.max(0, y), Math.max(0, viewport.height - widgetHeaderHeight)),
  };
}

/**
 * The widget itself. It is portalled to the document body because the
 * consultation dialog is centred with a CSS transform, which would otherwise
 * become the containing block for a fixed child and pin the widget inside the
 * scrolling panel.
 */
export function FloatingRecorder() {
  const { recording, paused, elapsed, patientName, canPause, pause, resume, stop } = useRecording();
  const [collapsed, setCollapsed] = React.useState(false);
  const [position, setPosition] = React.useState({ x: 16, y: 16 });
  const dragging = React.useRef<{ offsetX: number; offsetY: number } | null>(null);

  const onPointerDown = (event: React.PointerEvent<HTMLButtonElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId);
    dragging.current = { offsetX: event.clientX - position.x, offsetY: event.clientY - position.y };
  };
  const onPointerMove = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (!dragging.current) return;
    setPosition(clampWidgetPosition(
      event.clientX - dragging.current.offsetX,
      event.clientY - dragging.current.offsetY,
      { width: window.innerWidth, height: window.innerHeight },
    ));
  };
  const onPointerUp = () => { dragging.current = null; };

  if (!recording || typeof document === "undefined") return null;

  return createPortal(
    <section
      role="status"
      aria-label="Consultation recording in progress"
      className="fixed z-[60] w-60 rounded-lg border border-red-300 bg-white shadow-xl"
      style={{ left: position.x, top: position.y }}
    >
      <div className="flex items-center gap-1 border-b border-red-200 bg-red-50 px-2 py-1.5">
        <button
          aria-label="Move the recording widget"
          className="cursor-grab touch-none p-1 text-red-700"
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
        >
          <GripVertical aria-hidden className="h-4 w-4" />
        </button>
        <span className="flex items-center gap-1.5 font-mono text-xs font-bold text-red-700">
          <span className={`h-2.5 w-2.5 rounded-full bg-red-600 ${paused ? "" : "animate-pulse"}`} />
          {paused ? "Paused" : "Recording"} {formatDuration(elapsed)}
        </span>
        <button
          aria-label={collapsed ? "Expand the recording widget" : "Collapse the recording widget"}
          className="ml-auto p-1 text-red-700"
          onClick={() => setCollapsed((value) => !value)}
        >
          {collapsed ? <ChevronUp aria-hidden className="h-4 w-4" /> : <ChevronDown aria-hidden className="h-4 w-4" />}
        </button>
      </div>
      {!collapsed && (
        <div className="grid gap-2 p-2">
          <p className="truncate text-xs text-zinc-600"><Mic aria-hidden className="mr-1 inline h-3 w-3" />{patientName}</p>
          <div className="flex gap-2">
            {canPause && (paused
              ? <Button className="flex-1" size="sm" variant="outline" onClick={resume}><Play aria-hidden className="h-3 w-3" />Resume</Button>
              : <Button className="flex-1" size="sm" variant="outline" onClick={pause}><Pause aria-hidden className="h-3 w-3" />Pause</Button>)}
            <Button className="flex-1" size="sm" onClick={stop}><Square aria-hidden className="h-3 w-3" />Stop</Button>
          </div>
        </div>
      )}
    </section>,
    document.body,
  );
}
