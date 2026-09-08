// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import type { QueueEntry } from "../types";
import { Dialog } from "./ui/dialog";
import { WaitingRoomScreen, WalkInForm, formatWait, initials, nextStage, waitMinutes, waitSeverity } from "./waiting-room";

afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

function entry(overrides: Partial<QueueEntry> = {}): QueueEntry {
  return {
    id: "q1", patientId: "p1", medicalRecordNumber: "PT-000001", patientName: "Rose Célestin",
    phone: "+509 3456 7890", visitReason: "Red eye since yesterday", appointmentId: "", encounterId: "",
    assignedDoctorId: "u1", assignedDoctorName: "Dr Pierre", arrivedAt: new Date(Date.now() - 5 * 60000).toISOString(),
    stage: "waiting_nurse", stageEnteredAt: new Date().toISOString(), estimatedWaitMinutes: null,
    waitEstimateSamples: 0, priority: 0, source: "staff", version: 1, updatedAt: "", ...overrides,
  };
}

function renderQueue(items: QueueEntry[]) {
  vi.spyOn(api, "get").mockImplementation((path: string) =>
    Promise.resolve(path.startsWith("/queue") ? { items } : { items: [] }) as never);
  // The realtime context has a safe default, so no SSE connection is opened here.
  return render(<I18nProvider><WaitingRoomScreen /></I18nProvider>);
}

describe("queue progression", () => {
  it("advances one step along the clinical path", () => {
    expect(nextStage("waiting_nurse")).toBe("pre_test");
    expect(nextStage("in_consultation")).toBe("checkout");
    expect(nextStage("checkout")).toBe("completed");
    expect(nextStage("completed")).toBe("completed");
    expect(nextStage("nonsense")).toBe("waiting_nurse");
  });
});

describe("wait timer", () => {
  it("counts up from arrival", () => {
    const now = Date.now();
    expect(waitMinutes(new Date(now - 90 * 60000).toISOString(), now)).toBe(90);
    expect(waitMinutes(new Date(now + 60000).toISOString(), now)).toBe(0);
    expect(waitMinutes("not-a-date", now)).toBe(0);
    expect(formatWait(45)).toBe("45 min");
    expect(formatWait(90)).toBe("1 h 30");
  });

  it("flags a long wait with words as well as colour", () => {
    expect(waitSeverity(10)).toEqual({ tone: "neutral", label: "Waiting" });
    expect(waitSeverity(31)).toEqual({ tone: "warning", label: "Long wait" });
    expect(waitSeverity(61)).toEqual({ tone: "danger", label: "Waiting over an hour" });
  });

  it("builds initials from whatever name is on record", () => {
    expect(initials("Rose Célestin")).toBe("RC");
    expect(initials("Prince")).toBe("P");
    expect(initials("Jean Baptiste Pierre Louis")).toBe("JB");
  });
});

describe("WaitingRoomScreen", () => {
  it("shows everyone waiting with reason, practitioner and a live timer", async () => {
    renderQueue([entry(), entry({ id: "q2", patientId: "p2", patientName: "Marc Antoine", arrivedAt: new Date(Date.now() - 75 * 60000).toISOString(), visitReason: "Broken frame", assignedDoctorName: "" })]);
    await waitFor(() => expect(screen.getByText("Rose Célestin")).toBeTruthy());
    expect(screen.getByText("Red eye since yesterday")).toBeTruthy();
    expect(screen.getByText("Dr Pierre")).toBeTruthy();
    expect(screen.getByText("No practitioner assigned")).toBeTruthy();
    expect(screen.getByText("5 min")).toBeTruthy();
    // The long wait is flagged in words, not only by the amber card.
    expect(screen.getByText("1 h 15")).toBeTruthy();
    expect(screen.getByText("Waiting over an hour")).toBeTruthy();
  });

  it("moves a patient on in a single click with no confirmation", async () => {
    const patch = vi.spyOn(api, "patch").mockResolvedValue({} as never);
    renderQueue([entry()]);
    await waitFor(() => expect(screen.getByText("Rose Célestin")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "In consultation" }));
    await waitFor(() => expect(patch).toHaveBeenCalledWith("/queue/q1", expect.objectContaining({ stage: "in_consultation", version: 1 })));
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("offers checkout and completion directly, so arrival to checkout is a short click path", async () => {
    const patch = vi.spyOn(api, "patch").mockResolvedValue({} as never);
    renderQueue([entry({ stage: "in_consultation" })]);
    await waitFor(() => expect(screen.getByText("Rose Célestin")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Checkout" }));
    await waitFor(() => expect(patch).toHaveBeenCalledWith("/queue/q1", expect.objectContaining({ stage: "checkout" })));
    fireEvent.click(screen.getByRole("button", { name: "Completed" }));
    await waitFor(() => expect(patch).toHaveBeenCalledWith("/queue/q1", expect.objectContaining({ stage: "completed" })));
  });

  it("labels a queue entry with no appointment as a walk-in", async () => {
    renderQueue([entry(), entry({ id: "q3", patientId: "p3", patientName: "Booked Patient", appointmentId: "a1" })]);
    await waitFor(() => expect(screen.getByText("Booked Patient")).toBeTruthy());
    const rows = screen.getAllByRole("listitem");
    expect(within(rows[0]).getByText("Walk-in")).toBeTruthy();
    expect(within(rows[1]).queryByText("Walk-in")).toBeNull();
  });

  it("shows an empty state with the walk-in action rather than a blank panel", async () => {
    renderQueue([]);
    expect(await screen.findByText("Waiting room is clear")).toBeTruthy();
  });
});

describe("WalkInForm", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));

  it("registers and queues in one request", async () => {
    vi.spyOn(api, "get").mockResolvedValue({ items: [], page: 1, limit: 5, total: 0, hasMore: false } as never);
    const post = vi.spyOn(api, "post").mockResolvedValue({} as never);
    const onSaved = vi.fn();
    render(<I18nProvider><Dialog open><WalkInForm onSaved={onSaved} /></Dialog></I18nProvider>);
    fireEvent.change(screen.getByLabelText("First name"), { target: { value: "Rose" } });
    fireEvent.change(screen.getByLabelText("Last name"), { target: { value: "Célestin" } });
    fireEvent.change(screen.getByLabelText("Phone"), { target: { value: "50934567890" } });
    fireEvent.change(screen.getByLabelText("Reason for visit"), { target: { value: "Red eye" } });
    fireEvent.click(screen.getByRole("button", { name: "Add to waiting room" }));
    await waitFor(() => expect(post).toHaveBeenCalledWith("/queue/walk-in", expect.objectContaining({
      firstName: "Rose", lastName: "Célestin", phone: "50934567890", reason: "Red eye",
    })));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
  });

  it("surfaces existing records as the name is typed so a duplicate is not created", async () => {
    const match = { id: "p9", medicalRecordNumber: "PT-000009", firstName: "Rose", lastName: "Célestin", dateOfBirth: "1990-01-01", phone: "50934567890" };
    vi.spyOn(api, "get").mockImplementation((path: string) =>
      Promise.resolve(path.startsWith("/patients") ? { items: [match], page: 1, limit: 5, total: 1, hasMore: false } : { items: [] }) as never);
    const post = vi.spyOn(api, "post").mockResolvedValue({} as never);
    render(<I18nProvider><Dialog open><WalkInForm onSaved={() => undefined} /></Dialog></I18nProvider>);
    fireEvent.change(screen.getByLabelText("First name"), { target: { value: "Rose" } });
    fireEvent.change(screen.getByLabelText("Last name"), { target: { value: "Célestin" } });
    await act(async () => { vi.advanceTimersByTime(320); });
    const suggestion = await screen.findByText("Already registered? Select the record instead of creating a second one.");
    const existing = within(suggestion.parentElement as HTMLElement).getByRole("button");
    expect(existing.textContent).toContain("PT-000009");
    fireEvent.click(existing);
    await waitFor(() => expect(post).toHaveBeenCalledWith("/queue/walk-in", expect.objectContaining({ patientId: "p9" })));
  });
});
