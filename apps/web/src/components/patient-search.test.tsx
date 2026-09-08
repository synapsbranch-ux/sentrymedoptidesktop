// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { PatientPicker, activeFilterCount, emptyPatientFilters, patientAge, patientSearchQuery, type PatientSearchResult } from "./patient-search";

afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

function patient(overrides: Partial<PatientSearchResult> = {}): PatientSearchResult {
  return {
    id: "p1", medicalRecordNumber: "PT-000001", firstName: "Jéan", middleName: "", lastName: "Étienne", preferredName: "",
    sex: "male", dateOfBirth: "1985-03-04", phone: "+509 3456 7890", alternatePhone: "", email: "", address: "", city: "",
    occupation: "", employer: "", preferredLanguage: "", communicationPreference: "", referralSource: "", referringProvider: "",
    notes: "", tags: [], version: 1, createdAt: "", updatedAt: "", updatedBy: "", lastVisitAt: "2026-01-05T10:00:00Z", ...overrides,
  };
}

describe("patient search query", () => {
  it("asks the server for one page and never the whole table", () => {
    expect(patientSearchQuery("jean", emptyPatientFilters, 2, 25)).toBe("/patients?page=2&limit=25&q=jean&status=active");
  });

  it("passes every filter through and omits the empty ones", () => {
    const query = patientSearchQuery("", { ...emptyPatientFilters, sex: "female", ageMin: "18", ageMax: "65", insurance: "payer-1", practitioner: "user-1", lastVisitFrom: "2025-01-01", lastVisitUntil: "2026-01-01", status: "all" }, 1, 25);
    for (const expected of ["sex=female", "ageMin=18", "ageMax=65", "insurance=payer-1", "practitioner=user-1", "lastVisitFrom=2025-01-01", "lastVisitUntil=2026-01-01", "status=all"]) {
      expect(query).toContain(expected);
    }
    expect(query).not.toContain("q=");
    expect(query).not.toContain("civilStatus");
  });

  it("counts only filters that actually narrow the list", () => {
    expect(activeFilterCount(emptyPatientFilters)).toBe(0);
    expect(activeFilterCount({ ...emptyPatientFilters, sex: "male" })).toBe(1);
    expect(activeFilterCount({ ...emptyPatientFilters, status: "archived" })).toBe(1);
  });
});

describe("age from date of birth", () => {
  it("computes a whole year count and rejects unusable dates", () => {
    const now = new Date();
    const tenYearsAgo = new Date(now.getFullYear() - 10, now.getMonth(), now.getDate());
    expect(patientAge(tenYearsAgo.toISOString().slice(0, 10))).toBe(10);
    expect(patientAge("")).toBeNull();
    expect(patientAge("not-a-date")).toBeNull();
    expect(patientAge(new Date(now.getFullYear() + 1, 0, 1).toISOString().slice(0, 10))).toBeNull();
  });
});

describe("PatientPicker", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));

  // The I18n provider fetches the clinic language on mount; only patient
  // lookups are under test here.
  const patientCalls = (get: { mock: { calls: unknown[][] } }) =>
    get.mock.calls.map((call) => String(call[0])).filter((path) => path.startsWith("/patients"));

  it("debounces at 300ms and searches on the server", async () => {
    const get = vi.spyOn(api, "get").mockResolvedValue({ items: [patient()], page: 1, limit: 10, total: 1, hasMore: false } as never);
    render(<I18nProvider><PatientPicker value="" onChange={() => undefined} /></I18nProvider>);
    const field = screen.getByRole("combobox");
    for (const value of ["j", "je", "jea", "jean"]) fireEvent.change(field, { target: { value } });
    expect(patientCalls(get)).toHaveLength(0);
    await act(async () => { vi.advanceTimersByTime(310); });
    await waitFor(() => expect(patientCalls(get)).toHaveLength(1));
    expect(patientCalls(get)[0]).toContain("q=jean");
    expect(patientCalls(get)[0]).toContain("limit=10");
  });

  it("shows enough to identify the right person and reports the selection", async () => {
    vi.spyOn(api, "get").mockResolvedValue({ items: [patient()], page: 1, limit: 10, total: 1, hasMore: false } as never);
    const onChange = vi.fn();
    render(<I18nProvider><PatientPicker value="" onChange={onChange} /></I18nProvider>);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "jean" } });
    await act(async () => { vi.advanceTimersByTime(310); });
    const option = await screen.findByRole("option");
    expect(option.textContent).toContain("PT-000001");
    expect(option.textContent).toContain("1985-03-04");
    expect(option.textContent).toContain("+509 3456 7890");
    fireEvent.mouseDown(option);
    expect(onChange).toHaveBeenCalledWith("p1", expect.objectContaining({ id: "p1" }));
  });

  it("does not query the server for a single character", async () => {
    const get = vi.spyOn(api, "get").mockResolvedValue({ items: [], page: 1, limit: 10, total: 0, hasMore: false } as never);
    render(<I18nProvider><PatientPicker value="" onChange={() => undefined} /></I18nProvider>);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "j" } });
    await act(async () => { vi.advanceTimersByTime(400); });
    expect(patientCalls(get)).toHaveLength(0);
  });
});
