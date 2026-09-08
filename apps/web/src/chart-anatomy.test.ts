import { describe, expect, it } from "vitest";
import { clampAtlasView, eyeLandmarks, regionForStructure } from "./components/chart-anatomy";

describe("anatomical view boundaries", () => {
  it("maps explicit structures but never guesses anatomy from a finding type", () => {
    expect(regionForStructure("cornea")).toBe("cornea");
    expect(regionForStructure("optic_disc")).toBe("optic_disc");
    for (const ambiguous of ["drusen", "haemorrhage", "exudate", "other", "cup_disc_ratio", ""]) expect(regionForStructure(ambiguous)).toBeNull();
  });
  it("mirrors nasal landmarks between eyes without swapping anterior or superior axes", () => {
    const od = eyeLandmarks("OD"), os = eyeLandmarks("OS");
    expect(od.disc[0]).toBeGreaterThan(od.macula[0]);
    expect(os.disc[0]).toBeLessThan(os.macula[0]);
    expect(od.disc[0]).toBe(-os.disc[0]);
    expect(od.disc.slice(1)).toEqual(os.disc.slice(1));
    expect(od.macula.slice(1)).toEqual(os.macula.slice(1));
  });
  it("bounds navigation without accepting an upside-down or clipped camera", () => {
    expect(clampAtlasView({yaw:600,pitch:-180,zoom:9})).toEqual({yaw:180,pitch:-70,zoom:1.7});
    expect(clampAtlasView({yaw:-600,pitch:180,zoom:0})).toEqual({yaw:-180,pitch:70,zoom:0.7});
  });
});
