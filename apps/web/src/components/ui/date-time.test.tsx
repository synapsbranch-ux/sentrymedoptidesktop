// @vitest-environment jsdom
import * as React from "react";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import { Dialog, DialogContent, DialogTitle } from "./dialog";
import { DateTimeInput, TimePicker, joinDateTime, minuteOptions, parseTimeValue, splitDateTime } from "./date-time";

afterEach(cleanup);

const wrap = (node: React.ReactElement) => render(<I18nProvider>{node}</I18nProvider>);

describe("time value handling", () => {
  it("parses the forms a stored record can hold and rejects impossible ones", () => {
    expect(parseTimeValue("09:30")).toEqual({ hour: 9, minute: 30 });
    expect(parseTimeValue("9:05")).toEqual({ hour: 9, minute: 5 });
    expect(parseTimeValue("14:45:00")).toEqual({ hour: 14, minute: 45 });
    for (const bad of ["", "24:00", "12:60", "noon", "12", "12:5"]) expect(parseTimeValue(bad)).toBeNull();
  });

  it("keeps a stored off-step minute selectable", () => {
    expect(minuteOptions(15, null)).toEqual([0, 15, 30, 45]);
    expect(minuteOptions(15, 37)).toEqual([0, 15, 30, 37, 45]);
  });

  it("splits and rejoins a datetime-local value without drifting", () => {
    expect(splitDateTime("2026-03-04T09:30")).toEqual({ date: "2026-03-04", time: "09:30" });
    expect(splitDateTime("2026-03-04T09:30:00.000")).toEqual({ date: "2026-03-04", time: "09:30" });
    expect(joinDateTime("2026-03-04", "09:30")).toBe("2026-03-04T09:30");
    expect(joinDateTime("2026-03-04", "")).toBe("2026-03-04T00:00");
    expect(joinDateTime("", "09:30")).toBe("");
  });
});

describe("TimePicker", () => {
  it("renders an hour selector that does not depend on a native time widget", () => {
    wrap(<TimePicker label="Start" value="09:30" onChange={() => undefined} />);
    const hour = screen.getByLabelText("Start — hour") as HTMLSelectElement;
    const minute = screen.getByLabelText("Start — minute") as HTMLSelectElement;
    expect(hour.tagName).toBe("SELECT");
    expect(hour.value).toBe("09");
    expect(minute.value).toBe("30");
    expect(within(hour).getAllByRole("option")).toHaveLength(25);
  });

  it("reports a full HH:MM on either half changing", () => {
    const onChange = vi.fn();
    wrap(<TimePicker label="Start" value="09:30" onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Start — hour"), { target: { value: "14" } });
    expect(onChange).toHaveBeenLastCalledWith("14:30");
    fireEvent.change(screen.getByLabelText("Start — minute"), { target: { value: "45" } });
    expect(onChange).toHaveBeenLastCalledWith("09:45");
  });

  it("completes a half-entered time instead of emitting an invalid value", () => {
    const onChange = vi.fn();
    wrap(<TimePicker label="Start" value="" onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Start — hour"), { target: { value: "08" } });
    expect(onChange).toHaveBeenLastCalledWith("08:00");
  });
});

describe("DateTimeInput", () => {
  it("keeps date and time independent and redisplays a saved value", () => {
    const onChange = vi.fn();
    const { rerender } = wrap(<DateTimeInput label="Start" value="2026-03-04T09:30" onChange={onChange} />);
    expect((screen.getByLabelText("Start — date") as HTMLInputElement).value).toBe("2026-03-04");
    expect((screen.getByLabelText("Start — hour") as HTMLSelectElement).value).toBe("09");
    fireEvent.change(screen.getByLabelText("Start — hour"), { target: { value: "16" } });
    expect(onChange).toHaveBeenLastCalledWith("2026-03-04T16:30");
    rerender(<I18nProvider><DateTimeInput label="Start" value="2026-03-04T16:30" onChange={onChange} /></I18nProvider>);
    expect((screen.getByLabelText("Start — hour") as HTMLSelectElement).value).toBe("16");
  });

  it("stays operable inside a modal", () => {
    wrap(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Create appointment</DialogTitle>
          <DateTimeInput label="Start" value="2026-03-04T09:30" onChange={() => undefined} />
        </DialogContent>
      </Dialog>,
    );
    const dialog = screen.getByRole("dialog");
    const hour = within(dialog).getByLabelText("Start — hour");
    expect(hour).toBeTruthy();
    // A native select paints its list above the page, so a modal's own
    // overflow/stacking context cannot hide it the way a custom popup can.
    expect(hour.tagName).toBe("SELECT");
  });
});
