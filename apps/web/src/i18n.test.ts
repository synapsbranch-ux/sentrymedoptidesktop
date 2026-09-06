import { describe, expect, it } from "vitest";
import { isLanguageCode, supportedLanguages } from "./i18n";

describe("supported application languages", () => {
  it("contains every configured clinic language", () => {
    expect(supportedLanguages.map(({ code }) => code)).toEqual([
      "en", "fr", "ht", "pt", "es", "de", "zh-CN", "ru", "ja", "ko", "id",
    ]);
  });

  it("rejects unknown language values", () => {
    expect(isLanguageCode("fr")).toBe(true);
    expect(isLanguageCode("xx")).toBe(false);
    expect(isLanguageCode(null)).toBe(false);
  });
});
