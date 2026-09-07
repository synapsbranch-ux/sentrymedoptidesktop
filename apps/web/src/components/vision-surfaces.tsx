import * as React from "react";
import {
  amslerSquares,
  packPlateDots,
  plateColour,
  plateDigits,
  type PlateDot,
} from "../vision";

export interface AmslerMark { x: number; y: number; shape: string; color: string; label: string; structure: string; grade?: string }

/**
 * A generated pseudo-isochromatic plate. The number is drawn to an offscreen canvas and
 * each dot is coloured by whether its centre falls on the digit, which is what lets an
 * arbitrary number be shown without hand-authoring a mask per plate.
 */
export function ColourPlate({ seed, plate, sizePx }: { seed: number; plate: number; sizePx: number }) {
  const digits = plateDigits(seed, plate);
  const dots = React.useMemo(() => packPlateDots(seed + plate * 7), [seed, plate]);
  const figureFlags = useFigureMask(digits, dots);

  return (
    <svg width={sizePx} height={sizePx} viewBox="0 0 1000 1000" role="img" aria-label="Colour vision screening plate">
      <circle cx="500" cy="500" r="495" fill="#f4f1e8" />
      {dots.map((dot, index) => (
        <circle
          key={index}
          cx={dot.x * 1000}
          cy={dot.y * 1000}
          r={dot.r * 1000}
          fill={plateColour(seed + plate, index, figureFlags[index] ?? false)}
        />
      ))}
    </svg>
  );
}

/** Which dots land on the digit, decided by sampling the rendered glyph rather than by geometry. */
function useFigureMask(digits: string, dots: PlateDot[]): boolean[] {
  return React.useMemo(() => {
    const size = 600;
    try {
      const canvas = document.createElement("canvas");
      canvas.width = size;
      canvas.height = size;
      const context = canvas.getContext("2d", { willReadFrequently: true });
      if (!context) return dots.map(() => false);
      context.fillStyle = "#000";
      context.fillRect(0, 0, size, size);
      context.fillStyle = "#fff";
      context.textAlign = "center";
      context.textBaseline = "middle";
      context.font = `bold ${size * 0.62}px ui-monospace, Menlo, Consolas, monospace`;
      context.fillText(digits, size / 2, size / 2);
      const pixels = context.getImageData(0, 0, size, size).data;
      return dots.map((dot) => {
        const x = Math.min(size - 1, Math.max(0, Math.round(dot.x * size)));
        const y = Math.min(size - 1, Math.max(0, Math.round(dot.y * size)));
        return pixels[(y * size + x) * 4] > 128;
      });
    } catch {
      return dots.map(() => false);
    }
  }, [digits, dots]);
}

/**
 * The Amsler grid the patient marks directly. Drawing is by drag, so a distortion can be
 * traced rather than tapped square by square, and every mark is a percentage of the grid
 * so it means the same thing on any screen it is later reviewed on.
 */
export function AmslerSurface({
  marks,
  onMark,
  sizePx,
  readOnly = false,
  inverted = true,
}: {
  marks: AmslerMark[];
  onMark?(next: AmslerMark[]): void;
  sizePx: number;
  readOnly?: boolean;
  inverted?: boolean;
}) {
  const surface = React.useRef<SVGSVGElement>(null);
  const drawing = React.useRef(false);
  const line = inverted ? "#ffffff" : "#111827";
  const ground = inverted ? "#000000" : "#ffffff";
  const cells = Array.from({ length: amslerSquares + 1 }, (_, index) => (index / amslerSquares) * 100);

  const add = (event: React.PointerEvent<SVGSVGElement>) => {
    if (readOnly || !onMark || !surface.current) return;
    const box = surface.current.getBoundingClientRect();
    const x = ((event.clientX - box.left) / box.width) * 100;
    const y = ((event.clientY - box.top) / box.height) * 100;
    if (x < 0 || x > 100 || y < 0 || y > 100) return;
    // Marks within a square of one already made are dropped, so a slow drag leaves a
    // trace rather than hundreds of stacked points.
    if (marks.some((mark) => Math.hypot(mark.x - x, mark.y - y) < 100 / amslerSquares / 2)) return;
    onMark([...marks, { x, y, shape: "amsler", color: "#dc2626", label: "", structure: "amsler", grade: "distorted" }]);
  };

  return (
    <svg
      ref={surface}
      width={sizePx}
      height={sizePx}
      viewBox="0 0 100 100"
      className={readOnly ? undefined : "touch-none"}
      role={readOnly ? "img" : "application"}
      aria-label="Amsler grid"
      onPointerDown={(event) => { drawing.current = true; surface.current?.setPointerCapture(event.pointerId); add(event); }}
      onPointerMove={(event) => { if (drawing.current) add(event); }}
      onPointerUp={(event) => { drawing.current = false; surface.current?.releasePointerCapture(event.pointerId); }}
      onPointerCancel={() => { drawing.current = false; }}
    >
      <rect x="0" y="0" width="100" height="100" fill={ground} />
      {cells.map((position) => (
        <React.Fragment key={position}>
          <line x1={position} y1="0" x2={position} y2="100" stroke={line} strokeWidth="0.25" />
          <line x1="0" y1={position} x2="100" y2={position} stroke={line} strokeWidth="0.25" />
        </React.Fragment>
      ))}
      <circle cx="50" cy="50" r="1.1" fill={line} />
      {marks.map((mark, index) => (
        <circle key={index} cx={mark.x} cy={mark.y} r={1.8} fill={mark.color} fillOpacity={0.75} />
      ))}
    </svg>
  );
}
