import { describe, expect, it } from "vitest";
import { refractionForSource } from "./components/clinical-evolution";

describe("refraction source selection", () => {
  it("returns only the requested source instead of falling back across devices", () => {
    const point = {
      refractions: {
        subjective: { source: "subjective", odSphere: -2, osSphere: null, odCylinder: -1, osCylinder: null, odAxis: 180, osAxis: null, odSphericalEquivalent: -2.5, osSphericalEquivalent: null },
        autorefraction: { source: "autorefraction", odSphere: -3, osSphere: null, odCylinder: -1, osCylinder: null, odAxis: 175, osAxis: null, odSphericalEquivalent: -3.5, osSphericalEquivalent: null },
      },
    };
    expect(refractionForSource(point, "subjective")?.odSphere).toBe(-2);
    expect(refractionForSource(point, "autorefraction")?.odSphere).toBe(-3);
    expect(refractionForSource(point, "prescription")).toBeUndefined();
  });
});
