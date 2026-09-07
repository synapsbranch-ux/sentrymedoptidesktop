import * as React from "react";

export type Eye = "OD" | "OS" | "OU";

/** Point on a 100x100 chart canvas, in percentage coordinates. */
export interface ChartCell {
  id: string;
  label: string;
  hint?: string;
  path?: string;
  cx: number;
  cy: number;
}

const centre = 50;
const outerRadius = 44;
const innerRadius = 13;

function polar(radius: number, degrees: number) {
  const radians = (degrees * Math.PI) / 180;
  return { x: centre + radius * Math.cos(radians), y: centre + radius * Math.sin(radians) };
}

/** Annular sector swept clockwise in screen coordinates, used for field quadrants. */
function sector(startDegrees: number, endDegrees: number) {
  const outerStart = polar(outerRadius, startDegrees);
  const outerEnd = polar(outerRadius, endDegrees);
  const innerStart = polar(innerRadius, startDegrees);
  const innerEnd = polar(innerRadius, endDegrees);
  return [
    `M${innerStart.x},${innerStart.y}`,
    `L${outerStart.x},${outerStart.y}`,
    `A${outerRadius},${outerRadius} 0 0 1 ${outerEnd.x},${outerEnd.y}`,
    `L${innerEnd.x},${innerEnd.y}`,
    `A${innerRadius},${innerRadius} 0 0 0 ${innerStart.x},${innerStart.y}`,
    "Z",
  ].join(" ");
}

/**
 * Confrontation field zones. A visual field is charted from the patient's point of
 * view, so the temporal field of the right eye sits on the right of the drawing and
 * the left eye is its mirror image — the same handedness a perimeter printout uses.
 */
export function fieldCells(eye: Eye): ChartCell[] {
  const temporalOnRight = eye === "OD";
  const rightSide = temporalOnRight ? "temporal" : "nasal";
  const leftSide = temporalOnRight ? "nasal" : "temporal";
  const midRadius = (innerRadius + outerRadius) / 2;
  const quadrant = (startDegrees: number, endDegrees: number, vertical: string, horizontal: string): ChartCell => {
    const middle = polar(midRadius, (startDegrees + endDegrees) / 2);
    return {
      id: `${vertical}-${horizontal}`,
      label: `${vertical} ${horizontal}`,
      path: sector(startDegrees, endDegrees),
      cx: middle.x,
      cy: middle.y,
    };
  };
  return [
    quadrant(-90, 0, "superior", rightSide),
    quadrant(0, 90, "inferior", rightSide),
    quadrant(90, 180, "inferior", leftSide),
    quadrant(180, 270, "superior", leftSide),
    { id: "central", label: "central", cx: centre, cy: centre },
  ];
}

/**
 * The nine diagnostic positions of gaze, drawn as the examiner sees the patient:
 * the patient's right gaze is on the left of the grid. Each cell names the pair of
 * muscles that position tests, R/L prefixing the patient's right and left eye.
 */
export const motilityCells: ChartCell[] = [
  { id: "up-right", label: "Up & patient right", hint: "RSR / LIO", cx: 20, cy: 20 },
  { id: "up", label: "Elevation", hint: "SR / IO", cx: 50, cy: 20 },
  { id: "up-left", label: "Up & patient left", hint: "LSR / RIO", cx: 80, cy: 20 },
  { id: "right", label: "Patient right", hint: "RLR / LMR", cx: 20, cy: 50 },
  { id: "primary", label: "Primary position", hint: "straight ahead", cx: 50, cy: 50 },
  { id: "left", label: "Patient left", hint: "LLR / RMR", cx: 80, cy: 50 },
  { id: "down-right", label: "Down & patient right", hint: "RIR / LSO", cx: 20, cy: 80 },
  { id: "down", label: "Depression", hint: "IR / SO", cx: 50, cy: 80 },
  { id: "down-left", label: "Down & patient left", hint: "LIR / RSO", cx: 80, cy: 80 },
];

/** Schematic anterior segment: lids, cornea, iris and pupil seen face on. */
export function AnteriorBackdrop() {
  return (
    <g>
      <path d="M4,42 Q50,10 96,42 Q50,74 4,42 Z" fill="white" stroke="#27272a" strokeWidth="1.5" />
      <circle cx="50" cy="42" r="19" fill="#aecbdf" stroke="#27272a" strokeWidth="1" />
      <circle cx="50" cy="42" r="8.5" fill="#111827" />
      <circle cx="46" cy="38" r="2.5" fill="white" opacity="0.85" />
    </g>
  );
}

/**
 * Schematic fundus. The optic disc is nasal to the macula, so on a right eye it
 * falls on the right of the drawing and on a left eye on the left — the paired
 * layout where the two discs face each other.
 */
export function FundusBackdrop({ eye }: { eye: Eye }) {
  const discX = eye === "OD" ? 68 : 32;
  // Nasal is +1 for a right eye and -1 for a left eye, so the same geometry mirrors.
  const nasal = eye === "OD" ? 1 : -1;
  const maculaX = discX - nasal * 28;
  const clip = `fundus-globe-${eye}`;
  return (
    <g>
      <defs>
        <clipPath id={clip}><circle cx="50" cy="50" r="44.4" /></clipPath>
      </defs>
      <circle cx="50" cy="50" r="45" fill="#f0a868" stroke="#27272a" strokeWidth="1.2" />
      <g clipPath={`url(#${clip})`}>
        {/* Macula and foveal reflex, roughly two disc diameters temporal to the disc. */}
        <circle cx={maculaX} cy="52" r="9.5" fill="#c2703a" opacity="0.5" />
        <circle cx={maculaX} cy="52" r="1.6" fill="#8a4a1d" />
        {/* Temporal arcades leave the disc and arch above and below the macula. */}
        <path d={`M${discX},43 Q${discX - nasal * 12},19 ${discX - nasal * 42},33`} fill="none" stroke="#b91c1c" strokeWidth="1.2" />
        <path d={`M${discX},57 Q${discX - nasal * 12},81 ${discX - nasal * 42},67`} fill="none" stroke="#b91c1c" strokeWidth="1.2" />
        {/* Shorter nasal branches running to the periphery on the far side of the disc. */}
        <path d={`M${discX + nasal * 2},44 Q${discX + nasal * 12},30 ${discX + nasal * 17},21`} fill="none" stroke="#b91c1c" strokeWidth="0.9" />
        <path d={`M${discX + nasal * 2},56 Q${discX + nasal * 12},70 ${discX + nasal * 17},79`} fill="none" stroke="#b91c1c" strokeWidth="0.9" />
        {/* Optic disc with its physiological cup. */}
        <ellipse cx={discX} cy="50" rx="7.5" ry="8.5" fill="#fde68a" stroke="#a16207" strokeWidth="0.8" />
        <ellipse cx={discX} cy="50" rx="3" ry="3.4" fill="#fef9c3" stroke="#a16207" strokeWidth="0.4" />
      </g>
    </g>
  );
}
