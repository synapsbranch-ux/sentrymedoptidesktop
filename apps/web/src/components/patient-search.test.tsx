// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { Field, FieldGroup } from "./ui/input";
import { PatientPicker, activeFilterCount, emptyPatientFilters, patientAge, patientSearchQuery, type PatientSearchResult } from "./patient-search";

afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

function patient(overrides: Partial<PatientSearchResult> = {}): PatientSearchResult {
  return {
    id: "p1", medicalRecordNumber: "PT-000001", firstName: "Jéan", middleName: "", lastName: "Étienne", preferredName: "",
    sex: "male", dateOfBirth: "1985-03-04", phone: "+509 3456 7890", alternatePhone: "", email: "", address: "", city: "",
    occupation: "", employer: "", preferredLanguage: "", communicationPreference: "", referralSource: "", referringProvider: "",
    civilStatus: "", religion: "", religionOther: "", notes: "", tags: [], version: 1, createdAt: "", updatedAt: "", updatedBy: "", lastVisitAt: "2026-01-05T10:00:00Z", ...overrides,
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
    fireEvent.click(option);
    expect(onChange).toHaveBeenCalledWith("p1", expect.objectContaining({ id: "p1" }));
  });

  it("selects on click, so keyboard and touch activation both work", async () => {
    vi.spyOn(api, "get").mockResolvedValue({ items: [patient()], page: 1, limit: 10, total: 1, hasMore: false } as never);
    const onChange = vi.fn();
    render(<I18nProvider><PatientPicker value="" onChange={onChange} /></I18nProvider>);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "jean" } });
    await act(async () => { vi.advanceTimersByTime(310); });
    const option = await screen.findByRole("option");
    // A keyboard activation produces click with no preceding mousedown.
    fireEvent.click(option);
    expect(onChange).toHaveBeenCalledWith("p1", expect.objectContaining({ id: "p1" }));
  });

  it("is never wrapped in a label, which would send result clicks to the input", () => {
    // A <label> may contain one labelable control and forwards clicks to it, so
    // a composite control like this one belongs in FieldGroup.
    const { container } = render(
      <I18nProvider>
        <FieldGroup label="Patient"><PatientPicker value="" onChange={() => undefined} /></FieldGroup>
        <Field label="Plain"><input aria-label="Plain" /></Field>
      </I18nProvider>,
    );
    const combobox = screen.getByRole("combobox");
    expect(combobox.closest("label")).toBeNull();
    expect(container.querySelector('[role="group"]')).toBeTruthy();
  });

  it("does not query the server for a single character", async () => {
    const get = vi.spyOn(api, "get").mockResolvedValue({ items: [], page: 1, limit: 10, total: 0, hasMore: false } as never);
    render(<I18nProvider><PatientPicker value="" onChange={() => undefined} /></I18nProvider>);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "j" } });
    await act(async () => { vi.advanceTimersByTime(400); });
    expect(patientCalls(get)).toHaveLength(0);
  });
  it("rejects typed text until an existing patient is selected", () => {
    vi.spyOn(api, "get").mockResolvedValue({ items: [] } as never);
    render(<I18nProvider><PatientPicker required value="" onChange={() => undefined} /></I18nProvider>);
    const input = screen.getByRole("combobox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "unselected name" } });
    expect(input.checkValidity()).toBe(false);
    expect(input.validationMessage).toContain("Select an existing patient");
  });

  it("loads the selected patient when an appointment is reopened", async () => {
    const get = vi.spyOn(api, "get").mockImplementation(async (path) => path === "/patients/p1" ? patient() : {} as never);
    render(<I18nProvider><PatientPicker value="p1" onChange={() => undefined} /></I18nProvider>);
    await screen.findByText("Jéan Étienne");
    expect(patientCalls(get)).toContain("/patients/p1");
    expect(screen.getByRole("button", { name: "Change patient" })).toBeTruthy();
  });

  it("keeps the saved selection on a failed load and allows retry", async () => {
    let fail = true;
    vi.spyOn(api, "get").mockImplementation(async (path) => {
      if (path === "/patients/p1") { if (fail) throw new Error("Offline"); return patient() as never; }
      return {} as never;
    });
    const change = vi.fn();
    render(<I18nProvider><PatientPicker value="p1" onChange={change} /></I18nProvider>);
    const retry = await screen.findByRole("button", { name: "Retry" });
    expect(change).not.toHaveBeenCalled();
    fail = false;
    fireEvent.click(retry);
    await screen.findByText("Jéan Étienne");
  });

  it("supports arrow/Enter selection and hides stale results", async () => {
    vi.spyOn(api, "get").mockResolvedValue({ items: [patient()], total: 1 } as never);
    const change = vi.fn();
    render(<I18nProvider><PatientPicker value="" onChange={change} /></I18nProvider>);
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "jean" } });
    await act(async () => { vi.advanceTimersByTime(310); });
    await screen.findByRole("option");
    fireEvent.change(input, { target: { value: "other" } });
    expect(screen.queryByRole("option")).toBeNull();
    await act(async () => { vi.advanceTimersByTime(310); });
    await screen.findByRole("option");
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(change).toHaveBeenCalledWith("p1", expect.objectContaining({ id: "p1" }));
  });

});
