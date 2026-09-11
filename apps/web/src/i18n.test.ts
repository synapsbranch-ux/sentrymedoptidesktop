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

  it("preserves clinical abbreviations and currency codes in every locale", () => {
    const protectedTerms = ["OD", "OS", "OU", "logMAR", "mmHg", "CCT", "HTG", "USD"];
    for (const language of supportedLanguages) {
      if (language.code === "en") continue;
      for (const message of translationCatalog) {
        const translated = translateMessage(language.code, message);
        for (const term of protectedTerms) {
          if (message.match(new RegExp(`(^|[^A-Za-z])${term}([^A-Za-z]|$)`))) {
            expect(translated, `${language.code}: ${message}`).toMatch(new RegExp(`(^|[^A-Za-z])${term}([^A-Za-z]|$)`));
          }
        }
      }
    }
  });

  it("does not leave full English interface sentences in translated locales", () => {
    const codeLike = /(?:^|\s)(?:sm:|md:|lg:|xl:|grid(?:-cols)?|flex|gap-|rounded-|py-|pl-|pr-|text-|items-|hover:)/;
    for (const language of supportedLanguages) {
      if (language.code === "en") continue;
      const untranslated = translationCatalog.filter((message) => {
        const words = message.match(/[A-Za-z]+/g) ?? [];
        return words.length >= 4 && translateMessage(language.code, message) === message && !codeLike.test(message);
      });
      expect(untranslated, language.code).toEqual([]);
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
