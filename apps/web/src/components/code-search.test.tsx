// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { CodeCombobox } from "./code-search";

afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });
beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));

const codes = [{ code: "H52.13", description: "Myopia, bilateral" }];
const codeCalls = (get: { mock: { calls: unknown[][] } }) =>
  get.mock.calls.map((call) => String(call[0])).filter((path) => path.startsWith("/codes"));

function renderCombobox(onSelect = vi.fn()) {
  render(<I18nProvider><CodeCombobox endpoint="/codes/icd10" placeholder="Search a diagnosis code" onSelect={onSelect} /></I18nProvider>);
  return onSelect;
}

describe("CodeCombobox", () => {
  it("searches once the clinician stops typing, not once per keystroke", async () => {
    const get = vi.spyOn(api, "get").mockResolvedValue({ items: codes } as never);
    renderCombobox();
    const field = screen.getByRole("textbox");
    for (const value of ["m", "my", "myo", "myop"]) fireEvent.change(field, { target: { value } });
    expect(codeCalls(get)).toHaveLength(0);
    await act(async () => { vi.advanceTimersByTime(310); });
    await waitFor(() => expect(codeCalls(get)).toHaveLength(1));
    expect(codeCalls(get)[0]).toContain("q=myop");
  });

  it("selects on click, so keyboard and assistive-technology activation work", async () => {
    vi.spyOn(api, "get").mockResolvedValue({ items: codes } as never);
    const onSelect = renderCombobox();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "myopia" } });
    await act(async () => { vi.advanceTimersByTime(310); });
    const option = await screen.findByRole("option");
    // A keyboard activation produces click with no preceding mousedown.
    fireEvent.click(option);
    expect(onSelect).toHaveBeenCalledWith(codes[0]);
  });

  it("does not query the server for a single character", async () => {
    const get = vi.spyOn(api, "get").mockResolvedValue({ items: [] } as never);
    renderCombobox();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "m" } });
    await act(async () => { vi.advanceTimersByTime(400); });
    expect(codeCalls(get)).toHaveLength(0);
  });
});
