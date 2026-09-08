// @vitest-environment jsdom
import * as React from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import * as store from "../recording-store";
import { FloatingRecorder, RecordingProvider, clampWidgetPosition, extensionFor, formatDuration, useRecording } from "./recording";

afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

class FakeMediaRecorder {
  static instances: FakeMediaRecorder[] = [];
  static isTypeSupported = () => true;
  ondataavailable: ((event: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null;
  state = "inactive";
  constructor(public stream: MediaStream, public options: { mimeType: string }) { FakeMediaRecorder.instances.push(this); }
  start() { this.state = "recording"; }
  pause() { this.state = "paused"; }
  resume() { this.state = "recording"; }
  stop() { this.state = "inactive"; this.onstop?.(); }
  emit(blob: Blob) { this.ondataavailable?.({ data: blob }); }
}

beforeEach(() => {
  FakeMediaRecorder.instances = [];
  vi.stubGlobal("MediaRecorder", FakeMediaRecorder);
  Object.defineProperty(window, "isSecureContext", { value: true, configurable: true });
  Object.defineProperty(navigator, "mediaDevices", {
    value: { getUserMedia: vi.fn(async () => ({ getTracks: () => [{ stop: vi.fn() }] } as unknown as MediaStream)) },
    configurable: true,
  });
  Element.prototype.setPointerCapture = vi.fn();
  vi.spyOn(store, "persistChunk").mockResolvedValue(undefined);
  vi.spyOn(store, "clearRecovered").mockResolvedValue(undefined);
  vi.spyOn(store, "loadRecovered").mockResolvedValue(null);
});

function Harness({ children }: { children?: React.ReactNode }) {
  const { start, recording, elapsed } = useRecording();
  return (
    <div>
      <button onClick={() => void start()}>start</button>
      <span>state:{recording ? "on" : "off"}</span>
      <span>elapsed:{elapsed}</span>
      {children}
    </div>
  );
}

function renderRecorder() {
  return render(
    <I18nProvider>
      <RecordingProvider encounterId="e1" patientName="Rose Célestin">
        <Harness />
        <FloatingRecorder />
      </RecordingProvider>
    </I18nProvider>,
  );
}

describe("recording helpers", () => {
  it("keeps a dragged widget inside the viewport", () => {
    const viewport = { width: 1000, height: 800 };
    expect(clampWidgetPosition(300, 400, viewport)).toEqual({ x: 300, y: 400 });
    expect(clampWidgetPosition(-50, -50, viewport)).toEqual({ x: 0, y: 0 });
    expect(clampWidgetPosition(5000, 5000, viewport)).toEqual({ x: 760, y: 736 });
    // A viewport narrower than the widget still yields a usable position.
    expect(clampWidgetPosition(100, 100, { width: 200, height: 40 })).toEqual({ x: 0, y: 0 });
  });

  it("formats elapsed time and picks a file extension per container", () => {
    expect(formatDuration(0)).toBe("0:00");
    expect(formatDuration(65)).toBe("1:05");
    expect(extensionFor("audio/mp4")).toBe("m4a");
    expect(extensionFor("audio/ogg;codecs=opus")).toBe("ogg");
    expect(extensionFor("audio/webm;codecs=opus")).toBe("webm");
  });
});

describe("floating widget", () => {
  it("appears only while recording and shows the patient, the state and the time", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderRecorder();
    expect(screen.queryByRole("status", { name: "Consultation recording in progress" })).toBeNull();
    await act(async () => { fireEvent.click(screen.getByText("start")); });
    const widget = await screen.findByRole("status", { name: "Consultation recording in progress" });
    expect(widget.textContent).toContain("Rose Célestin");
    expect(widget.textContent).toContain("Recording 0:00");
    await act(async () => { vi.advanceTimersByTime(2000); });
    expect(widget.textContent).toContain("Recording 0:02");
  });

  it("pauses, resumes and stops from the widget", async () => {
    renderRecorder();
    await act(async () => { fireEvent.click(screen.getByText("start")); });
    const widget = await screen.findByRole("status", { name: "Consultation recording in progress" });
    fireEvent.click(screen.getByRole("button", { name: "Pause" }));
    await waitFor(() => expect(widget.textContent).toContain("Paused"));
    expect(FakeMediaRecorder.instances[0].state).toBe("paused");
    fireEvent.click(screen.getByRole("button", { name: "Resume" }));
    await waitFor(() => expect(FakeMediaRecorder.instances[0].state).toBe("recording"));
    const post = vi.spyOn(api, "post").mockResolvedValue({} as never);
    FakeMediaRecorder.instances[0].emit(new Blob(["audio"]));
    fireEvent.click(screen.getByRole("button", { name: "Stop" }));
    await waitFor(() => expect(post).toHaveBeenCalledWith("/encounters/e1/recordings", expect.any(FormData)));
    await waitFor(() => expect(screen.queryByRole("status", { name: "Consultation recording in progress" })).toBeNull());
  });

  it("collapses and can be dragged without leaving the viewport", async () => {
    renderRecorder();
    await act(async () => { fireEvent.click(screen.getByText("start")); });
    const widget = await screen.findByRole("status", { name: "Consultation recording in progress" });
    fireEvent.click(screen.getByRole("button", { name: "Collapse the recording widget" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Stop" })).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Expand the recording widget" }));
    expect(await screen.findByRole("button", { name: "Stop" })).toBeTruthy();

    // jsdom's PointerEvent drops clientX/clientY, so the drag itself cannot be
    // simulated here; the handle's presence and the clamping are checked instead.
    const handle = screen.getByRole("button", { name: "Move the recording widget" });
    expect(handle.className).toContain("touch-none");
    expect(widget.style.position).toBe("");
    expect(widget.className).toContain("fixed");
  });
});

describe("interrupted recordings", () => {
  it("keeps each chunk locally as it arrives", async () => {
    renderRecorder();
    await act(async () => { fireEvent.click(screen.getByText("start")); });
    const chunk = new Blob(["audio"]);
    FakeMediaRecorder.instances[0].emit(chunk);
    await waitFor(() => expect(store.persistChunk).toHaveBeenCalledWith("e1", chunk, expect.any(String), expect.any(Number)));
  });

  it("offers audio captured before an interruption instead of losing it", async () => {
    vi.spyOn(store, "loadRecovered").mockResolvedValue({
      encounterId: "e1", chunks: [new Blob(["audio"])], mimeType: "audio/webm", durationSeconds: 42, startedAt: "",
    });
    const post = vi.spyOn(api, "post").mockResolvedValue({} as never);
    function Recover() {
      const { recovered, saveRecovered } = useRecording();
      return recovered ? <button onClick={() => void saveRecovered()}>recover {recovered.durationSeconds}</button> : null;
    }
    render(<I18nProvider><RecordingProvider encounterId="e1" patientName="Rose"><Recover /></RecordingProvider></I18nProvider>);
    const button = await screen.findByText("recover 42");
    fireEvent.click(button);
    await waitFor(() => expect(post).toHaveBeenCalledWith("/encounters/e1/recordings", expect.any(FormData)));
  });

  it("keeps the local copy when the upload fails, rather than discarding the audio", async () => {
    vi.spyOn(store, "loadRecovered").mockResolvedValue({
      encounterId: "e1", chunks: [new Blob(["audio"])], mimeType: "audio/webm", durationSeconds: 42, startedAt: "",
    });
    vi.spyOn(api, "post").mockRejectedValue(new Error("network down"));
    function Recover() {
      const { recovered, saveRecovered } = useRecording();
      return recovered ? <button onClick={() => void saveRecovered()}>recover</button> : null;
    }
    render(<I18nProvider><RecordingProvider encounterId="e1" patientName="Rose"><Recover /></RecordingProvider></I18nProvider>);
    fireEvent.click(await screen.findByText("recover"));
    await waitFor(() => expect(api.post).toHaveBeenCalled());
    expect(store.clearRecovered).not.toHaveBeenCalledWith("e1");
  });
});
