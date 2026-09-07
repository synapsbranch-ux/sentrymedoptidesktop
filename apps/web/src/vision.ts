/**
 * Optotype geometry and chart generation for the digital acuity chart.
 *
 * Everything here is pure so it can be unit tested: the screen component owns the
 * pixels, this module owns the arithmetic that decides how big they should be.
 */

/** Sloan letters, the ten-letter set standard acuity charts are built from. */
export const sloanLetters = ["C", "D", "H", "K", "N", "O", "R", "S", "V", "Z"] as const;
/** Digits used for patients who read numbers more readily than Latin letters. */
export const chartDigits = ["2", "3", "4", "5", "6", "7", "8", "9"] as const;
/** Landolt C gap directions, in degrees clockwise from the right. */
export const landoltAngles = [0, 45, 90, 135, 180, 225, 270, 315] as const;
/** Tumbling E limb directions, in degrees clockwise from the right. */
export const tumblingEAngles = [0, 90, 180, 270] as const;

export type Optotype = "sloan" | "landolt" | "tumblingE" | "numbers";

/** The chart lines, in logMAR. 0.00 is 20/20 and each step is one line. */
export const logMarLines = [
  -0.3, -0.2, -0.1, 0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.6,
];

const snellenByLogMar: [number, string][] = [
  [-0.3, "20/10"], [-0.2, "20/12.5"], [-0.1, "20/16"], [0, "20/20"], [0.1, "20/25"],
  [0.2, "20/32"], [0.3, "20/40"], [0.4, "20/50"], [0.5, "20/63"], [0.6, "20/80"],
  [0.7, "20/100"], [0.8, "20/125"], [0.9, "20/160"], [1, "20/200"], [1.1, "20/250"],
  [1.2, "20/320"], [1.3, "20/400"], [1.4, "20/500"], [1.5, "20/630"], [1.6, "20/800"],
];

/** Nearest standard chart line, or the raw logMAR when no line is close enough. */
export function snellenFromLogMar(value: number): string {
  let best = "";
  let distance = Number.POSITIVE_INFINITY;
  for (const [logMar, snellen] of snellenByLogMar) {
    const gap = Math.abs(logMar - value);
    if (gap < distance) { best = snellen; distance = gap; }
  }
  return distance > 0.06 ? `logMAR ${value.toFixed(2)}` : best;
}

/** Metric equivalent of the same line, for clinics that chart in 6 metres. */
export function metricFromLogMar(value: number): string {
  const snellen = snellenFromLogMar(value);
  if (!snellen.startsWith("20/")) return snellen;
  const denominator = Number(snellen.slice(3)) * 0.3;
  return `6/${denominator % 1 === 0 ? denominator : denominator.toFixed(1)}`;
}

/**
 * mulberry32: a small deterministic generator. The same seed always produces the same
 * chart, so the screen and the phone agree on what is displayed without sending the
 * letters over the wire; a new seed reshuffles it, so a patient cannot recite a line
 * they memorised with the other eye.
 */
export function createRandom(seed: number): () => number {
  let state = (seed >>> 0) || 1;
  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let value = Math.imul(state ^ (state >>> 15), 1 | state);
    value = (value + Math.imul(value ^ (value >>> 7), 61 | value)) ^ value;
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296;
  };
}

/**
 * Height of an optotype in millimetres. A 20/20 letter subtends five arc-minutes
 * overall at the test distance, and every logMAR step scales that by a factor of ten
 * to the power of the step — which is what makes the chart valid at any distance,
 * provided the distance and the screen scale are both known.
 */
export function optotypeHeightMm(logMar: number, distanceMm: number): number {
  const fiveArcMinutes = 2 * Math.tan((2.5 / 60) * (Math.PI / 180));
  return distanceMm * fiveArcMinutes * Math.pow(10, logMar);
}

export function optotypeHeightPx(logMar: number, distanceMm: number, pixelsPerMm: number): number {
  return optotypeHeightMm(logMar, distanceMm) * pixelsPerMm;
}

/** ID-1 card width in millimetres, the reference object used to calibrate a screen. */
export const referenceCardWidthMm = 85.6;

export interface ChartLine {
  /** What to draw: a glyph for letters and digits, a rotation for the others. */
  glyph: string;
  angle: number;
}

/**
 * One randomised line. Letters never repeat within a line, so a patient guessing a
 * repeat cannot be scored correct by accident.
 */
export function buildChartLine(optotype: Optotype, seed: number, logMar: number, count: number): ChartLine[] {
  // Folding the level into the seed keeps each line independent: stepping down and
  // back up re-draws the same line, but two different lines never share a sequence.
  const random = createRandom(Math.round((seed + logMar * 1000) * 7919));
  const line: ChartLine[] = [];
  if (optotype === "landolt" || optotype === "tumblingE") {
    const angles = optotype === "landolt" ? landoltAngles : tumblingEAngles;
    for (let index = 0; index < count; index += 1) {
      line.push({ glyph: optotype === "landolt" ? "C" : "E", angle: angles[Math.floor(random() * angles.length)] });
    }
    return line;
  }
  const pool = [...(optotype === "numbers" ? chartDigits : sloanLetters)] as string[];
  for (let index = 0; index < count; index += 1) {
    if (pool.length === 0) break;
    const position = Math.floor(random() * pool.length);
    line.push({ glyph: pool.splice(position, 1)[0], angle: 0 });
  }
  return line;
}

/** How many optotypes fit on a line at this size, within the standard 1-to-5 range. */
export function lineLength(logMar: number, distanceMm: number, pixelsPerMm: number, availablePx: number): number {
  const height = optotypeHeightPx(logMar, distanceMm, pixelsPerMm);
  if (height <= 0) return 1;
  const spacing = height * 2;
  return Math.max(1, Math.min(5, Math.floor(availablePx / spacing)));
}

/** The next line down the chart, or the same one at the end. */
export function stepLogMar(current: number, direction: 1 | -1): number {
  const index = logMarLines.findIndex((line) => Math.abs(line - current) < 0.001);
  const next = index === -1 ? logMarLines.indexOf(0) : index + direction;
  return logMarLines[Math.min(logMarLines.length - 1, Math.max(0, next))];
}
