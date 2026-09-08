// @vitest-environment jsdom
import * as React from "react";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { ClinicalEvolution } from "./clinical-evolution";
import { ErrorBoundary } from "./error-boundary";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const thresholds = { elevatedIOPMmHg: 21 };
const noPrevious = { hasPrevious: false, changes: [] };

function point(index: number) {
  return {
    encounterId: `e${index}`, encounterNumber: `EN-${index}`,
    date: new Date(Date.UTC(2024, 0, 1 + index)).toISOString(),
    refractions: { subjective: { source: "subjective", odSphere: -1 - index * 0.25, osSphere: -1, odCylinder: -0.5, osCylinder: -0.5, odAxis: 180, osAxis: 175, odSphericalEquivalent: -1.25, osSphericalEquivalent: -1.25 } },
    visualAcuity: { od: 0.1, os: 0.2 }, iop: { od: 15 + index, os: 16 }, pachymetry: { od: 540, os: 545 },
  };
}

function stubAPI(trends: unknown, delta: unknown = noPrevious) {
  vi.spyOn(api, "get").mockImplementation((path: string) =>
    Promise.resolve(path.includes("clinical-trends") ? trends : delta) as never);
}

function renderEvolution() {
  return render(
    <I18nProvider>
      <ErrorBoundary label="Evolution"><ClinicalEvolution encounterId="e1" patientId="p1" /></ErrorBoundary>
    </I18nProvider>,
  );
}

async function expectNoPanelFailure() {
  await waitFor(() => expect(screen.getByText("Since the last visit")).toBeTruthy());
  expect(screen.queryByRole("alert")).toBeNull();
}

describe("Evolution section", () => {
  it("renders a patient with no entries at all", async () => {
    stubAPI({ points: [], alerts: [], summary: {}, thresholds });
    renderEvolution();
    await expectNoPanelFailure();
    expect(screen.getByText("No previous visit")).toBeTruthy();
  });

  it("renders a patient with a single entry", async () => {
    stubAPI({ points: [point(0)], alerts: [], summary: {}, thresholds });
    renderEvolution();
    await expectNoPanelFailure();
  });

  it("renders a patient with 50+ entries and a previous-visit delta", async () => {
    stubAPI(
      { points: Array.from({ length: 60 }, (_, index) => point(index)), alerts: [], summary: {}, thresholds },
      { hasPrevious: true, previous: { encounterNumber: "EN-58", date: new Date(Date.UTC(2024, 1, 27)).toISOString() }, changes: [{ category: "IOP", changeType: "modified", eye: "OD", before: "15", after: "24", delta: "+9", severity: "danger" }] },
    );
    renderEvolution();
    await expectNoPanelFailure();
    expect(screen.getByText("+9")).toBeTruthy();
  });

  it("contains an unexpected panel error instead of unmounting the application", () => {
    const logged = vi.spyOn(console, "error").mockImplementation(() => undefined);
    function Broken(): React.ReactElement { throw new Error("boom"); }
    render(<I18nProvider><div>Rest of the application</div><ErrorBoundary label="Evolution"><Broken /></ErrorBoundary></I18nProvider>);
    expect(screen.getByRole("alert").textContent).toContain("This section could not be displayed");
    expect(screen.getByText("Rest of the application")).toBeTruthy();
    logged.mockRestore();
  });

  it("survives null, missing and malformed fields in a record", async () => {
    stubAPI(
      {
        points: [
          { encounterId: "e1", encounterNumber: "EN-1", date: "not-a-date", refractions: null, visualAcuity: null, iop: { od: null, os: Number.NaN }, pachymetry: undefined },
          { encounterId: "e2", encounterNumber: "EN-2", date: null, iop: { od: 18, os: null } },
        ],
        alerts: null,
        summary: null,
        // `thresholds` deliberately absent: the server omits it on a partial payload.
      },
      { hasPrevious: true, previous: { encounterNumber: "EN-1", date: null }, changes: [{ category: "", changeType: "", before: null, after: null, delta: null, severity: "info" }] },
    );
    renderEvolution();
    await expectNoPanelFailure();
  });
});
