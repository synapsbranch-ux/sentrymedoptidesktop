// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { isLanguageCode, localizeDOM, missingTranslations, supportedLanguages, translateMessage, translationCatalog } from "./i18n";

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

  it("ships a complete non-empty catalog for every available language", () => {
    expect(translationCatalog.length).toBeGreaterThan(2000);
    for (const language of supportedLanguages) {
      if (language.code !== "en") expect(missingTranslations(language.code)).toEqual([]);
    }
  });

  it("localizes legacy text and attributes without translating the clinic name", () => {
    document.body.innerHTML = '<main title="Clinic Management"><h1>Clinic identity</h1><p data-i18n-skip>Clinique Le Bon Spécialiste</p></main>';
    localizeDOM(document.body, "fr", (message) => translateMessage("fr", message));
    expect(document.querySelector("h1")).toHaveTextContent("Identité de la clinique");
    expect(document.querySelector("main")).toHaveAttribute("title", "Gestion de clinique");
    expect(document.querySelector("p")).toHaveTextContent("Clinique Le Bon Spécialiste");

    localizeDOM(document.body, "de", (message) => translateMessage("de", message));
    expect(document.querySelector("h1")).toHaveTextContent("Klinikidentität");
    expect(document.querySelector("p")).toHaveTextContent("Clinique Le Bon Spécialiste");
  });
});
