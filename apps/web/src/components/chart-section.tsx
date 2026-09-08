import * as React from "react";
import { anatomyRegions, type AnatomyRegion } from "./chart-anatomy";

/** Illustrative sagittal section. Its horizontal axis is anterior/posterior, not nasal/temporal. */
export function ChartSection({ region, onExplore, eye }: { region: AnatomyRegion | null; onExplore(region: AnatomyRegion): void; eye: string }) {
  const gradient = React.useId().replaceAll(":", "");
  const style = (id: AnatomyRegion) => ({ fill: anatomyRegions.find(item => item.id === id)!.color, stroke: region === id ? "#2563eb" : "#475569", strokeWidth: region === id ? 2 : 0.7 });
  const part = (id: AnatomyRegion) => ({ ...style(id), onClick: () => onExplore(id), className: "cursor-pointer" });
  return <svg viewBox="0 0 140 110" role="img" aria-label={`${eye} 2.5D anatomical section`} className="w-full min-w-0 rounded border bg-slate-50">
    <defs><radialGradient id={gradient}><stop stopColor="#fff" /><stop offset="1" stopColor="#dbeafe" /></radialGradient></defs>
    <text x="8" y="9" fontSize="4" fill="#475569">Anterior</text><text x="113" y="9" fontSize="4" fill="#475569">Posterior</text>
    <path d="M109 46 L135 42 L135 60 L109 58 Z" {...part("optic_nerve")} />
    <path d="M40 18 C139 -2 141 108 40 88 Q24 54 40 18Z" {...part("sclera")} />
    <path d="M41 23 C128 6 130 100 41 83 Q29 54 41 23Z" {...part("retina")} />
    <path d="M43 27 C121 12 123 94 43 79 Q30 54 43 27Z" {...part("vitreous")} fill={`url(#${gradient})`} />
    <path d="M40 24 Q5 54 40 83 Q15 54 40 24Z" {...part("cornea")} />
    <path d="M39 27 L43 44 L39 44 L34 30Z M39 80 L43 64 L39 64 L34 78Z" {...part("iris")} />
    <ellipse cx="49" cy="54" rx="8" ry="21" {...part("lens")} />
    <text x="75" y="55" fontSize="4.5" textAnchor="middle" fill="#475569">Vitreous</text>
    <text x="70" y="103" fontSize="3.8" textAnchor="middle" fill="#475569">Generic section · no lesion depth is shown</text>
  </svg>;
}
