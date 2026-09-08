import { describe, expect, it } from "vitest";
import { civilStatusOptions, civilStatusText, religionOptions, religionText } from "./demographics";

describe("civil status", () => {
  it("offers exactly the six options the clinic asked for", () => {
    expect(civilStatusOptions.map((option) => option.value)).toEqual([
      "single", "married", "common_law", "divorced", "separated", "widowed",
    ]);
  });

  it("reads as blank when nothing was recorded, since the field is optional", () => {
    expect(civilStatusText("")).toBe("");
    expect(civilStatusText("married")).toBe("Married");
    expect(civilStatusText("unknown_value")).toBe("");
  });
});

describe("religion", () => {
  it("ends with Other and Prefer not to say", () => {
    const values = religionOptions.map((option) => option.value);
    expect(values).toContain("other");
    expect(values).toContain("prefer_not_to_say");
    expect(values).toContain("none");
  });

  it("shows the free text alongside Other and never instead of a listed value", () => {
    expect(religionText("other", "Rastafari")).toBe("Other — Rastafari");
    expect(religionText("other", "")).toBe("Other");
    expect(religionText("catholic", "Rastafari")).toBe("Catholic");
    expect(religionText("prefer_not_to_say", "")).toBe("Prefer not to say");
    expect(religionText("", "")).toBe("");
  });
});
