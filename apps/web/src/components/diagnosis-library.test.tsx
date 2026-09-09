// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { DiagnosisLibrary } from "./diagnosis-library";

afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });
beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));

const categories = { items: [{ category: "Glaucoma", count: 36 }, { category: "Cornea", count: 48 }] };
const firstPage = {
  items: [{ code: "H10.9", description: "Unspecified conjunctivitis", category: "Conjunctiva", synonyms: ["conjunctivitis", "red eye"], source: "builtin" }],
  page: 1, limit: 25, total: 30, hasMore: true,
};

function stubApi() {
  return vi.spyOn(api, "get").mockImplementation((path: string) =>
    Promise.resolve((path.startsWith("/diagnosis-codes/categories") ? categories : firstPage) as never));
}

async function openLibrary(onSelect = vi.fn()) {
  render(<I18nProvider><DiagnosisLibrary onSelect={onSelect} /></I18nProvider>);
  fireEvent.click(screen.getByRole("button", { name: /browse diagnosis library/i }));
  await act(async () => { vi.advanceTimersByTime(350); });
  return onSelect;
}

describe("DiagnosisLibrary", () => {
  it("offers the reference categories and a page of codes", async () => {
    stubApi();
    await openLibrary();
    await waitFor(() => expect(screen.getByRole("option", { name: /Glaucoma \(36\)/ })).toBeTruthy());
    expect(screen.getByText("Unspecified conjunctivitis")).toBeTruthy();
    expect(screen.getByText(/30 matching codes/)).toBeTruthy();
  });

  it("searches the server rather than filtering a preloaded list", async () => {
    const get = stubApi();
    await openLibrary();
    fireEvent.change(screen.getByPlaceholderText(/search by complaint or code/i), { target: { value: "red eye" } });
    await act(async () => { vi.advanceTimersByTime(350); });
    await waitFor(() => expect(get.mock.calls.some((call) => String(call[0]).includes("q=red%20eye"))).toBe(true));
  });

  it("hands the selected entry back with its code", async () => {
    stubApi();
    const onSelect = await openLibrary();
    fireEvent.click(await screen.findByRole("button", { name: "Use" }));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ code: "H10.9", description: "Unspecified conjunctivitis" }));
  });

  it("pages forward instead of loading every code at once", async () => {
    const get = stubApi();
    await openLibrary();
    fireEvent.click(await screen.findByRole("button", { name: "Next" }));
    await act(async () => { vi.advanceTimersByTime(350); });
    await waitFor(() => expect(get.mock.calls.some((call) => String(call[0]).includes("page=2"))).toBe(true));
  });
});
