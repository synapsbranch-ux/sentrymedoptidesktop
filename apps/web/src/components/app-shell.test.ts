import { describe, expect, it } from "vitest";
import { visibleNav } from "./app-shell";
import { featureVisibility } from "../features";

const destinations = (role: string | undefined) => visibleNav(role).flatMap((group) => group.items.map((item) => item.to));

describe("sidebar", () => {
  it("offers no way into a feature the clinic has withheld", () => {
    const doctor = destinations("doctor");
    expect(featureVisibility.quotes).toBe(false);
    expect(featureVisibility.visionTesting).toBe(false);
    expect(featureVisibility.auditLog).toBe(false);
    expect(doctor).not.toContain("/quotes");
    expect(doctor).not.toContain("/vision-test");
    expect(doctor).not.toContain("/audit");
    // Withholding two entries must not take their neighbours with them.
    expect(doctor).toEqual(expect.arrayContaining(["/", "/patients", "/clinical", "/prescriptions", "/pos", "/billing", "/system"]));
  });

  it("still keeps doctor-only destinations away from other staff", () => {
    const nurse = destinations("nurse");
    for (const restricted of ["/finance", "/hr", "/reports", "/system", "/purchasing"]) {
      expect(nurse).not.toContain(restricted);
    }
    expect(nurse).toContain("/patients");
  });

  it("draws no heading for a section left with nothing in it", () => {
    for (const group of visibleNav("nurse")) expect(group.items.length).toBeGreaterThan(0);
  });
});
