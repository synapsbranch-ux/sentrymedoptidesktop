import { describe, expect, it } from "vitest";
import { addMoney, calculateBalance, formatMoney, minorToInputValue, parseMoneyInput, splitByPercent, subtractMoney, sumMoney } from "./money";

describe("parseMoneyInput", () => {
  it("reads a whole gourde amount as HTG, not centimes", () => {
    expect(parseMoneyInput("650")).toBe(65000);
    expect(parseMoneyInput("6500")).toBe(650000);
    expect(parseMoneyInput("65000")).toBe(6500000);
    expect(parseMoneyInput("650000")).toBe(65000000);
  });
  it("reads a decimal amount exactly", () => {
    expect(parseMoneyInput("650.50")).toBe(65050);
    expect(parseMoneyInput("0.50")).toBe(50);
    expect(parseMoneyInput("0")).toBe(0);
  });
  it("ignores thousands separators", () => {
    expect(parseMoneyInput("6,500")).toBe(650000);
    expect(parseMoneyInput("6,500.00")).toBe(650000);
  });
  it("treats blank or bare punctuation as zero rather than throwing", () => {
    expect(parseMoneyInput("")).toBe(0);
    expect(parseMoneyInput("-")).toBe(0);
    expect(parseMoneyInput(".")).toBe(0);
  });
  it("round-trips through minorToInputValue", () => {
    for (const minor of [0, 50, 65000, 650000, 6500000, 65000000, 123]) {
      expect(parseMoneyInput(minorToInputValue(minor))).toBe(minor);
    }
  });
});

describe("formatMoney", () => {
  it("never turns 650 HTG into 6.50 HTG", () => {
    expect(formatMoney(65000, "HTG")).toContain("650.00");
    expect(formatMoney(650000, "HTG")).toContain("6,500.00");
    expect(formatMoney(6500000, "HTG")).toContain("65,000.00");
    expect(formatMoney(65050, "HTG")).toContain("650.50");
  });
});

describe("money arithmetic helpers", () => {
  it("add/subtract/sum stay exact integers", () => {
    expect(addMoney(65000, 5000)).toBe(70000);
    expect(subtractMoney(70000, 5000)).toBe(65000);
    expect(sumMoney([65000, 5000, 25000])).toBe(95000);
  });
  it("calculateBalance is total minus paid", () => {
    expect(calculateBalance(1000000, 300000)).toBe(700000);
  });
  it("splitByPercent never loses or invents a minor unit", () => {
    for (const [total, percent] of [[1000000, 70], [1, 1], [999999, 33.33], [0, 50], [7, 100]] as const) {
      const [share, remainder] = splitByPercent(total, percent);
      expect(share + remainder).toBe(total);
      expect(share).toBeGreaterThanOrEqual(0);
      expect(remainder).toBeGreaterThanOrEqual(0);
    }
  });
});
