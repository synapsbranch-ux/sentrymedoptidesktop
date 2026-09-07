import { describe, expect, it } from "vitest";
import { fieldCells, fundusClockHour } from "./components/chart-backdrops";

describe("fundus clock notation", () => {
  it("uses the clinical clock-face orientation", () => {
    expect(fundusClockHour(50, 5)).toBe(12);
    expect(fundusClockHour(95, 50)).toBe(3);
    expect(fundusClockHour(50, 95)).toBe(6);
    expect(fundusClockHour(5, 50)).toBe(9);
  });
});

describe("confrontation field orientation", () => {
  it("mirrors nasal and temporal labels for OD and OS", () => {
    expect(fieldCells("OD")[0].id).toBe("superior-temporal");
    expect(fieldCells("OS")[0].id).toBe("superior-nasal");
  });
});
