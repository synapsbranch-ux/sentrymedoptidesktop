import { describe, expect, it } from "vitest";
import { money, todayInput } from "./lib";

describe("formatting helpers", () => {
  it("uses integer minor units for money", () => expect(money(12345, "USD")).toContain("123.45"));
  it("returns an ISO local date", () => expect(todayInput()).toMatch(/^\d{4}-\d{2}-\d{2}$/));
});

