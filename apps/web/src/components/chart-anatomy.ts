import type { Eye } from "./chart-backdrops";

/** Versioned, generic anatomy. Coordinates are illustration units, never patient measurements. */
export const anatomyVersion = "generic-eye-v1";
export const anatomyRegions = [
  { id: "cornea", label: "Cornea", color: "#38bdf8" },
  { id: "iris", label: "Iris", color: "#4f8a8b" },
  { id: "lens", label: "Lens", color: "#c4b5fd" },
  { id: "sclera", label: "Sclera", color: "#cbd5e1" },
  { id: "vitreous", label: "Vitreous", color: "#bae6fd" },
  { id: "retina", label: "Retina", color: "#fb923c" },
  { id: "macula", label: "Macula", color: "#b45309" },
  { id: "optic_disc", label: "Optic disc", color: "#fde68a" },
  { id: "optic_nerve", label: "Optic nerve", color: "#fbbf24" },
] as const;
export type AnatomyRegion = typeof anatomyRegions[number]["id"];
// Finding types such as haemorrhage/drusen are not an anatomical anchor.
export function regionForStructure(structure: string): AnatomyRegion | null {
  return anatomyRegions.find(region => region.id === structure)?.id ?? null;
}
export function eyeLandmarks(eye: Eye) {
  const nasal = eye === "OD" ? 1 : -1;
  return { nasal, disc: [nasal * 0.32, 0, -0.88] as const, macula: [-nasal * 0.2, 0, -0.94] as const };
}
export interface AtlasView { yaw: number; pitch: number; zoom: number }
export const initialAtlasView: AtlasView = { yaw: -35, pitch: 12, zoom: 1 };
export function clampAtlasView(view: AtlasView): AtlasView {
  return { yaw: Math.max(-180, Math.min(180, view.yaw)), pitch: Math.max(-70, Math.min(70, view.pitch)), zoom: Math.max(0.7, Math.min(1.7, view.zoom)) };
}
