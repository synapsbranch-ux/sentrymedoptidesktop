import * as React from "react";
import { useParams } from "react-router-dom";
import { api } from "../api";
import { useRealtime } from "../realtime";
import {
  buildChartLine,
  lineLength,
  optotypeHeightPx,
  referenceCardWidthMm,
  snellenFromLogMar,
  type Optotype,
} from "../vision";

interface DisplayInfo { distanceMm: number; pixelsPerMm: number; calibratedAt: string; label: string }
interface VisionState {
  mode: "blank" | "acuity" | "colour" | "amsler" | "fixation";
  eye: "OD" | "OS" | "OU";
  correction: string;
  logMar: number;
  optotype: Optotype;
  seed: number;
  singleLine: boolean;
  plate: number;
  display: DisplayInfo;
  results: unknown[];
}
interface Session { id: string; room: string; state: VisionState; revision: number }

/**
 * Screen calibration is a property of one physical monitor, not of the clinic, so it
 * lives in this device's own storage and is never synced. A wrong scale here silently
 * invalidates every measurement, so the display refuses to show optotypes until the
 * screen has been calibrated and a test distance entered on this device.
 */
const calibrationKey = "sentrymed.vision.calibration";
interface Calibration { pixelsPerMm: number; distanceMm: number; calibratedAt: string; label: string }

function readCalibration(): Calibration | null {
  try {
    const raw = window.localStorage.getItem(calibrationKey);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Calibration;
    return parsed.pixelsPerMm > 0 && parsed.distanceMm > 0 ? parsed : null;
  } catch {
    return null;
  }
}

function writeCalibration(value: Calibration) {
  try {
    window.localStorage.setItem(calibrationKey, JSON.stringify(value));
  } catch {
    // A screen with storage blocked simply asks for calibration again next time.
  }
}

export function VisionDisplayPage() {
  const { id = "" } = useParams();
  const { revision } = useRealtime();
  const [session, setSession] = React.useState<Session | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [calibration, setCalibration] = React.useState<Calibration | null>(() => readCalibration());
  const [showCalibration, setShowCalibration] = React.useState(() => readCalibration() === null);

  const load = React.useCallback(async () => {
    try {
      setSession(await api.get<Session>(`/vision-tests/${id}`));
      setError(null);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not reach the clinic server");
    }
  }, [id]);

  // The realtime revision advances on every clinic write, so the display re-reads on
  // the phone's command within a round trip. The slow poll is the safety net for a
  // dropped event stream — on a clinic LAN it costs nothing.
  React.useEffect(() => { void load(); }, [load, revision]);
  React.useEffect(() => {
    const timer = window.setInterval(() => { void load(); }, 4000);
    return () => window.clearInterval(timer);
  }, [load]);

  // The phone must be able to see whether this screen is calibrated before it trusts a
  // measurement, so the display publishes its own scale into the shared session.
  React.useEffect(() => {
    if (!session || !calibration) return;
    const current = session.state.display;
    if (current.pixelsPerMm === calibration.pixelsPerMm && current.distanceMm === calibration.distanceMm) return;
    void api.put(`/vision-tests/${id}/state`, {
      state: { ...session.state, display: { ...calibration } },
    }).catch(() => undefined);
  }, [session, calibration, id]);

  if (error) {
    return <Fullscreen><p className="text-2xl text-red-400">{error}</p></Fullscreen>;
  }
  if (!session) {
    return <Fullscreen><p className="text-2xl text-zinc-500">Connecting to the clinic server…</p></Fullscreen>;
  }
  if (showCalibration || !calibration) {
    return (
      <CalibrationScreen
        initial={calibration}
        onDone={(value) => { writeCalibration(value); setCalibration(value); setShowCalibration(false); }}
      />
    );
  }

  return (
    <Fullscreen>
      <button
        type="button"
        onClick={() => setShowCalibration(true)}
        className="absolute right-4 top-4 rounded-md border border-zinc-700 px-3 py-2 text-xs text-zinc-500 hover:text-zinc-200"
      >
        {session.room} · {(calibration.distanceMm / 1000).toFixed(2)} m · recalibrate
      </button>
      <ChartSurface state={session.state} calibration={calibration} />
    </Fullscreen>
  );
}

function Fullscreen({ children }: { children: React.ReactNode }) {
  return <div className="relative grid min-h-screen place-items-center bg-black p-6 text-center text-white">{children}</div>;
}

/**
 * Held against the screen, a bank card is the one ruler every clinic already has.
 * Matching the on-screen rectangle to it gives the pixels-per-millimetre this monitor
 * actually renders at, which no browser API reports reliably.
 */
function CalibrationScreen({ initial, onDone }: { initial: Calibration | null; onDone(value: Calibration): void }) {
  const [widthPx, setWidthPx] = React.useState(() => Math.round((initial?.pixelsPerMm ?? 3.78) * referenceCardWidthMm));
  const [distanceM, setDistanceM] = React.useState(() => String((initial?.distanceMm ?? 4000) / 1000));
  const [label, setLabel] = React.useState(initial?.label ?? "");
  const pixelsPerMm = widthPx / referenceCardWidthMm;
  const distanceMm = Number(distanceM.replace(",", ".")) * 1000;
  const valid = pixelsPerMm > 0.5 && distanceMm >= 500 && distanceMm <= 20000;

  return (
    <div className="grid min-h-screen place-items-center bg-white p-6">
      <div className="w-full max-w-2xl">
        <h1 className="text-2xl font-bold">Calibrate this screen</h1>
        <p className="mt-2 text-sm text-zinc-600">
          Hold any bank or ID card flat against the screen and drag the slider until the white rectangle is exactly the
          same width. This measurement stays on this device.
        </p>
        <div className="mt-6 rounded-lg border-2 border-black bg-zinc-100 p-4">
          <div
            className="h-[54px] rounded-md border-2 border-black bg-white"
            style={{ width: `${widthPx}px`, maxWidth: "100%" }}
            aria-label="Calibration rectangle"
          />
        </div>
        <label className="mt-4 grid gap-2 text-sm font-semibold">
          Card width on screen
          <input
            type="range"
            min={150}
            max={900}
            value={widthPx}
            onChange={(event) => setWidthPx(Number(event.target.value))}
            className="w-full"
          />
          <span className="font-mono text-xs font-normal text-zinc-500">
            {widthPx} px for {referenceCardWidthMm} mm → {pixelsPerMm.toFixed(2)} px/mm
          </span>
        </label>
        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <label className="grid gap-1.5 text-sm font-semibold">
            Test distance (metres)
            <input
              className="h-11 rounded-md border border-zinc-300 px-3 text-sm font-normal"
              inputMode="decimal"
              value={distanceM}
              onChange={(event) => setDistanceM(event.target.value)}
            />
          </label>
          <label className="grid gap-1.5 text-sm font-semibold">
            Screen name (optional)
            <input
              className="h-11 rounded-md border border-zinc-300 px-3 text-sm font-normal"
              value={label}
              placeholder="Lane 1 monitor"
              onChange={(event) => setLabel(event.target.value)}
            />
          </label>
        </div>
        <p className="mt-4 text-xs text-zinc-500">
          At this scale and distance a 20/20 letter is{" "}
          <strong>{optotypeHeightPx(0, distanceMm || 0, pixelsPerMm).toFixed(1)} px</strong> tall.
        </p>
        <button
          type="button"
          disabled={!valid}
          onClick={() => onDone({ pixelsPerMm, distanceMm, label: label.trim(), calibratedAt: new Date().toISOString() })}
          className="mt-6 h-12 w-full rounded-md bg-black px-4 font-semibold text-white disabled:opacity-40"
        >
          Save calibration
        </button>
      </div>
    </div>
  );
}

function ChartSurface({ state, calibration }: { state: VisionState; calibration: Calibration }) {
  const [width, setWidth] = React.useState(() => window.innerWidth);
  React.useEffect(() => {
    const onResize = () => setWidth(window.innerWidth);
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  if (state.mode === "fixation") {
    return <div className="h-24 w-24 rounded-full bg-white" aria-label="Fixation target" />;
  }
  if (state.mode !== "acuity") {
    return <p className="text-xl text-zinc-600">Waiting for the examiner…</p>;
  }

  const heightPx = optotypeHeightPx(state.logMar, calibration.distanceMm, calibration.pixelsPerMm);
  const count = state.singleLine ? 1 : lineLength(state.logMar, calibration.distanceMm, calibration.pixelsPerMm, width * 0.85);
  const line = buildChartLine(state.optotype, state.seed, state.logMar, count);
  const tooLarge = heightPx > window.innerHeight * 0.9;

  if (tooLarge) {
    return (
      <div className="max-w-3xl">
        <p className="text-3xl font-bold text-amber-400">This line does not fit on this screen</p>
        <p className="mt-4 text-lg text-zinc-400">
          {snellenFromLogMar(state.logMar)} needs {Math.round(heightPx)} px at {(calibration.distanceMm / 1000).toFixed(2)} m.
          Move the screen closer and recalibrate the distance, or test this level with a printed chart.
        </p>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center gap-[0.6em] bg-black" style={{ fontSize: `${heightPx}px` }}>
      {line.map((entry, index) => (
        <Optotype key={index} optotype={state.optotype} glyph={entry.glyph} angle={entry.angle} sizePx={heightPx} />
      ))}
    </div>
  );
}

/**
 * The height a glyph is actually drawn at is a fraction of its font size that differs
 * between fonts and between glyphs — round letters overshoot the flat ones by a few
 * percent. Measuring each glyph and scaling by its own result is what makes the letter
 * on screen subtend the angle the acuity level claims; a single constant would make
 * every letter quietly the wrong size, and round ones the most wrong.
 */
function useGlyphHeightRatio(fontFamily: string, glyph: string) {
  return React.useMemo(() => {
    try {
      const context = document.createElement("canvas").getContext("2d");
      if (!context) return 0.72;
      context.font = `bold 100px ${fontFamily}`;
      const metrics = context.measureText(glyph);
      const height = (metrics.actualBoundingBoxAscent ?? 0) + (metrics.actualBoundingBoxDescent ?? 0);
      return height > 10 && height < 200 ? height / 100 : 0.72;
    } catch {
      return 0.72;
    }
  }, [fontFamily, glyph]);
}

const chartFontFamily = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

function Optotype({ optotype, glyph, angle, sizePx }: { optotype: Optotype; glyph: string; angle: number; sizePx: number }) {
  const heightRatio = useGlyphHeightRatio(chartFontFamily, glyph);
  if (optotype === "landolt" || optotype === "tumblingE") {
    // Both are built on the same 5x5 grid as a Sloan letter, with a stroke and a gap
    // one fifth of the overall height, so their angular size matches the letters.
    const unit = 100 / 5;
    return (
      <svg width={sizePx} height={sizePx} viewBox="0 0 100 100" style={{ transform: `rotate(${angle}deg)` }} aria-hidden="true">
        {optotype === "landolt" ? (
          <>
            <circle cx="50" cy="50" r="40" fill="none" stroke="white" strokeWidth={unit} />
            <rect x="70" y={50 - unit / 2} width="30" height={unit} fill="black" />
          </>
        ) : (
          <g fill="white">
            <rect x="0" y="0" width={unit} height="100" />
            <rect x="0" y="0" width="100" height={unit} />
            <rect x="0" y={50 - unit / 2} width="100" height={unit} />
            <rect x="0" y={100 - unit} width="100" height={unit} />
          </g>
        )}
      </svg>
    );
  }
  return (
    <span
      className="font-bold leading-none text-white"
      style={{ fontFamily: chartFontFamily, fontSize: `${sizePx / heightRatio}px`, lineHeight: 1 }}
      aria-hidden="true"
    >
      {glyph}
    </span>
  );
}
