// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { I18nProvider } from "../i18n";
import { StatusLegend, StatusPill, stageAppearance, statusAppearance } from "./status";

afterEach(cleanup);

// The colour required by the brief for each appointment status.
const requiredHues: Record<string, string> = {
  scheduled: "blue",
  confirmed: "emerald",
  checked_in: "amber",
  waiting: "amber",
  in_consultation: "purple",
  completed: "zinc",
  cancelled: "red",
  no_show: "red",
};

describe("appointment status appearance", () => {
  it("gives every status the colour the clinic asked for", () => {
    for (const [status, hue] of Object.entries(requiredHues)) {
      const appearance = statusAppearance(status);
      expect(appearance.className, status).toContain(hue);
      expect(appearance.chipClassName, status).toContain(hue);
      expect(appearance.edgeClassName, status).toContain(hue);
    }
  });

  it("separates no-show from cancelled by more than a shade of red", () => {
    const cancelled = statusAppearance("cancelled");
    const noShow = statusAppearance("no_show");
    expect(noShow.striped).toBe(true);
    expect(cancelled.striped).toBeUndefined();
    expect(noShow.label).not.toBe(cancelled.label);
    expect(noShow.Icon).not.toBe(cancelled.Icon);
  });

  it("never relies on colour alone: every status carries a label and an icon", () => {
    for (const status of [...Object.keys(requiredHues), "something_unmapped", ""]) {
      const appearance = statusAppearance(status);
      expect(appearance.label.length, status).toBeGreaterThan(0);
      expect(appearance.Icon, status).toBeTruthy();
    }
  });

  it("falls back to a readable label for a status it does not know", () => {
    expect(statusAppearance("rescheduled_twice").label).toBe("rescheduled twice");
    expect(statusAppearance("").label).toBe("Unknown");
  });
});

describe("waiting-room stage appearance", () => {
  it("maps every stage onto the same seven colours", () => {
    for (const [stage, hue] of Object.entries({
      checked_in: "amber", waiting_nurse: "amber", pre_test: "amber", waiting_doctor: "amber",
      in_consultation: "purple", checkout: "emerald", completed: "zinc",
    })) {
      const appearance = stageAppearance(stage);
      expect(appearance.className, stage).toContain(hue);
      expect(appearance.label.length, stage).toBeGreaterThan(0);
    }
  });

  it("uses the same appearance for in-consultation on both screens", () => {
    expect(stageAppearance("in_consultation").className).toBe(statusAppearance("in_consultation").className);
  });
});

describe("status rendering", () => {
  it("renders the words, not only the colour", () => {
    render(<I18nProvider><StatusPill appearance={statusAppearance("no_show")} /></I18nProvider>);
    expect(screen.getByText("No-show")).toBeTruthy();
  });

  it("shows a legend covering all seven statuses", () => {
    render(<I18nProvider><StatusLegend /></I18nProvider>);
    for (const label of ["Scheduled", "Confirmed", "Checked in / waiting", "In consultation", "Completed", "Cancelled", "No-show"]) {
      expect(screen.getByText(label), label).toBeTruthy();
    }
  });
});
