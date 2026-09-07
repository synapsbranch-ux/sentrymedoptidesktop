import { describe, expect, it } from "vitest";
import {
  amslerSideMm,
  amslerSquareSizeMm,
  buildChartLine,
  createRandom,
  lineLength,
  metricFromLogMar,
  optotypeHeightMm,
  packPlateDots,
  plateDigits,
  snellenFromLogMar,
  stepLogMar,
} from "./vision";

describe("optotype geometry", () => {
  it("sizes a 20/20 letter to about 8.7 mm at six metres", () => {
    expect(optotypeHeightMm(0, 6000)).toBeCloseTo(8.73, 2);
  });

  it("scales one logMAR line by roughly a quarter", () => {
    const twentyTwenty = optotypeHeightMm(0, 4000);
    expect(optotypeHeightMm(0.1, 4000) / twentyTwenty).toBeCloseTo(1.2589, 3);
    expect(optotypeHeightMm(1, 4000) / twentyTwenty).toBeCloseTo(10, 6);
  });

  it("keeps the same angular size when distance and level are both halved in effect", () => {
    // Doubling the distance doubles the physical letter for the same acuity line.
    expect(optotypeHeightMm(0.3, 8000)).toBeCloseTo(optotypeHeightMm(0.3, 4000) * 2, 6);
  });
});

describe("acuity notation", () => {
  it("names the standard chart lines", () => {
    expect(snellenFromLogMar(0)).toBe("20/20");
    expect(snellenFromLogMar(0.3)).toBe("20/40");
    expect(snellenFromLogMar(1)).toBe("20/200");
  });

  it("reports logMAR when no standard line is close", () => {
    expect(snellenFromLogMar(2.4)).toBe("logMAR 2.40");
  });

  it("converts to the metric notation", () => {
    expect(metricFromLogMar(0)).toBe("6/6");
    expect(metricFromLogMar(0.3)).toBe("6/12");
  });
});

describe("randomised chart lines", () => {
  it("is deterministic for a seed and level", () => {
    const first = buildChartLine("sloan", 42, 0.3, 5);
    const second = buildChartLine("sloan", 42, 0.3, 5);
    expect(first).toEqual(second);
  });

  it("reshuffles when the seed changes", () => {
    const first = buildChartLine("sloan", 42, 0.3, 5).map((entry) => entry.glyph).join("");
    const second = buildChartLine("sloan", 43, 0.3, 5).map((entry) => entry.glyph).join("");
    expect(first).not.toBe(second);
  });

  it("gives different lines to different acuity levels under one seed", () => {
    const line = buildChartLine("sloan", 42, 0.3, 5).map((entry) => entry.glyph).join("");
    const nextLine = buildChartLine("sloan", 42, 0.4, 5).map((entry) => entry.glyph).join("");
    expect(line).not.toBe(nextLine);
  });

  it("never repeats a letter within a line", () => {
    const glyphs = buildChartLine("sloan", 7, 0, 5).map((entry) => entry.glyph);
    expect(new Set(glyphs).size).toBe(glyphs.length);
  });

  it("varies only the orientation for directional optotypes", () => {
    const line = buildChartLine("tumblingE", 9, 0.2, 4);
    expect(line.every((entry) => entry.glyph === "E")).toBe(true);
    expect(line.every((entry) => [0, 90, 180, 270].includes(entry.angle))).toBe(true);
  });
});

describe("chart layout", () => {
  it("shows at most five optotypes and always at least one", () => {
    expect(lineLength(0, 4000, 4, 4000)).toBe(5);
    expect(lineLength(1.6, 4000, 4, 300)).toBe(1);
  });

  it("walks the chart one line at a time and stops at the ends", () => {
    expect(stepLogMar(0, 1)).toBeCloseTo(0.1, 5);
    expect(stepLogMar(0, -1)).toBeCloseTo(-0.1, 5);
    expect(stepLogMar(-0.3, -1)).toBeCloseTo(-0.3, 5);
    expect(stepLogMar(1.6, 1)).toBeCloseTo(1.6, 5);
  });
});

describe("seeded generator", () => {
  const draw = (seed: number, count: number) => {
    const random = createRandom(seed);
    return Array.from({ length: count }, () => random());
  };

  it("repeats its sequence for the same seed", () => {
    expect(draw(123, 5)).toEqual(draw(123, 5));
  });

  it("produces a different sequence for a different seed", () => {
    expect(draw(123, 5)).not.toEqual(draw(124, 5));
  });

  it("stays inside the unit interval", () => {
    const random = createRandom(999);
    for (let index = 0; index < 200; index += 1) {
      const value = random();
      expect(value).toBeGreaterThanOrEqual(0);
      expect(value).toBeLessThan(1);
    }
  });
});

describe("colour screening plates", () => {
  it("packs dots without overlap and inside the disc", () => {
    const dots = packPlateDots(11);
    expect(dots.length).toBeGreaterThan(400);
    for (const dot of dots) {
      expect(Math.hypot(dot.x - 0.5, dot.y - 0.5) + dot.r).toBeLessThanOrEqual(0.49);
    }
    for (let i = 0; i < dots.length; i += 1) {
      for (let j = i + 1; j < dots.length; j += 1) {
        expect(Math.hypot(dots[i].x - dots[j].x, dots[i].y - dots[j].y)).toBeGreaterThanOrEqual(dots[i].r + dots[j].r);
      }
    }
  });

  it("is deterministic, so both devices see the same plate", () => {
    expect(packPlateDots(11, 60)).toEqual(packPlateDots(11, 60));
    expect(packPlateDots(11)).toEqual(packPlateDots(11));
    expect(plateDigits(11, 3)).toBe(plateDigits(11, 3));
  });

  it("gives different plates different numbers", () => {
    const numbers = new Set(Array.from({ length: 12 }, (_, index) => plateDigits(99, index + 1)));
    expect(numbers.size).toBeGreaterThan(4);
  });

  it("never starts a number with zero", () => {
    for (let plate = 1; plate <= 24; plate += 1) {
      expect(plateDigits(5, plate).startsWith("0")).toBe(false);
    }
  });
});

describe("Amsler geometry", () => {
  it("makes each square one degree at the reading distance", () => {
    expect(amslerSquareSizeMm(330)).toBeCloseTo(5.76, 2);
  });

  it("scales the whole grid with the distance", () => {
    expect(amslerSideMm(660)).toBeCloseTo(amslerSideMm(330) * 2, 6);
  });
});
