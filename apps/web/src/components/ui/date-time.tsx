import * as React from "react";
import { Clock3 } from "lucide-react";
import { cn } from "../../lib";
import { useI18n } from "../../i18n";
import { Input, Select } from "./input";

/**
 * The clinic's only hour selector.
 *
 * It is built from two native `<select>` elements on purpose. `<input type="time">`
 * and the time half of `<input type="datetime-local">` are not implemented by the
 * WebKitGTK/WKWebView engines behind the desktop shell, which silently degrade the
 * field to a plain text box with no hour selector — the reason no time control
 * appeared anywhere in the application. A native select also renders its list
 * above the page at the OS level, so it can never be clipped by a modal's
 * `overflow-y-auto` or hidden behind its z-index, and it is keyboard-navigable
 * and touch-friendly without any extra code.
 */

const pad = (value: number) => value.toString().padStart(2, "0");

export function formatTimeValue(hour: number, minute: number) {
  return `${pad(hour)}:${pad(minute)}`;
}

/** Accepts "H:MM", "HH:MM" and "HH:MM:SS"; rejects anything out of range. */
export function parseTimeValue(value: string): { hour: number; minute: number } | null {
  const match = /^(\d{1,2}):(\d{2})(?::\d{2})?$/.exec((value ?? "").trim());
  if (!match) return null;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (hour > 23 || minute > 59) return null;
  return { hour, minute };
}

/** Steps through the hour, always including the minute already on the record. */
export function minuteOptions(step: number, current: number | null) {
  const safeStep = Number.isInteger(step) && step > 0 && step <= 30 ? step : 5;
  const values = new Set<number>();
  for (let minute = 0; minute < 60; minute += safeStep) values.add(minute);
  if (current !== null) values.add(current);
  return [...values].sort((left, right) => left - right);
}

function displayTime(hour: number, minute: number, locale?: string) {
  const reference = new Date(2000, 0, 1, hour, minute);
  return reference.toLocaleTimeString(locale, { hour: "numeric", minute: "2-digit" });
}

export interface TimePickerProps {
  /** "HH:MM" in 24-hour form, or "" when no time is set. */
  value: string;
  onChange(value: string): void;
  minuteStep?: number;
  disabled?: boolean;
  required?: boolean;
  className?: string;
  /** Prefix for the hour and minute accessible names, e.g. "Start". */
  label?: string;
}

export function TimePicker({ value, onChange, minuteStep = 5, disabled, required, className, label = "Time" }: TimePickerProps) {
  const { t, language } = useI18n();
  const parsed = parseTimeValue(value);
  const hour = parsed?.hour ?? null;
  const minute = parsed?.minute ?? null;
  const minutes = minuteOptions(minuteStep, minute);

  const commit = (nextHour: number | null, nextMinute: number | null) => {
    if (nextHour === null && nextMinute === null) return onChange("");
    onChange(formatTimeValue(nextHour ?? 0, nextMinute ?? 0));
  };

  return (
    <div className={cn("flex items-center gap-2", className)} data-testid="time-picker">
      <Clock3 aria-hidden className="h-4 w-4 shrink-0 text-[var(--muted-foreground)]" />
      <Select
        aria-label={`${t(label)} — ${t("hour")}`}
        className="w-auto min-w-20 font-mono"
        disabled={disabled}
        required={required}
        value={hour === null ? "" : pad(hour)}
        onChange={(event) => commit(event.target.value === "" ? null : Number(event.target.value), minute)}
      >
        <option value="">{t("Hour")}</option>
        {Array.from({ length: 24 }, (_, index) => (
          <option key={index} value={pad(index)}>{pad(index)}</option>
        ))}
      </Select>
      <span aria-hidden className="font-mono text-sm font-bold text-[var(--muted-foreground)]">:</span>
      <Select
        aria-label={`${t(label)} — ${t("minute")}`}
        className="w-auto min-w-20 font-mono"
        disabled={disabled}
        required={required}
        value={minute === null ? "" : pad(minute)}
        onChange={(event) => commit(hour, event.target.value === "" ? null : Number(event.target.value))}
      >
        <option value="">{t("Minute")}</option>
        {minutes.map((option) => (
          <option key={option} value={pad(option)}>{pad(option)}</option>
        ))}
      </Select>
      {hour !== null && minute !== null && (
        <span className="whitespace-nowrap text-xs text-[var(--muted-foreground)]">{displayTime(hour, minute, language)}</span>
      )}
    </div>
  );
}

/** Splits "YYYY-MM-DDTHH:MM" into its two halves; tolerates a full ISO string. */
export function splitDateTime(value: string) {
  const [date = "", rest = ""] = (value ?? "").split("T");
  const time = parseTimeValue(rest.slice(0, 5));
  return { date, time: time ? formatTimeValue(time.hour, time.minute) : "" };
}

export function joinDateTime(date: string, time: string) {
  if (!date) return "";
  const parsed = parseTimeValue(time);
  return `${date}T${parsed ? formatTimeValue(parsed.hour, parsed.minute) : "00:00"}`;
}

export interface DateTimeInputProps {
  /** Local "YYYY-MM-DDTHH:MM", the same shape `datetime-local` produced. */
  value: string;
  onChange(value: string): void;
  minuteStep?: number;
  disabled?: boolean;
  required?: boolean;
  label?: string;
}

/** Replaces `<input type="datetime-local">`, whose time half does not render everywhere. */
export function DateTimeInput({ value, onChange, minuteStep, disabled, required, label = "Time" }: DateTimeInputProps) {
  const { t } = useI18n();
  const { date, time } = splitDateTime(value);
  return (
    <div className="grid gap-2 sm:grid-cols-[minmax(9rem,1fr)_auto] sm:items-center">
      <Input
        aria-label={`${t(label)} — ${t("date")}`}
        type="date"
        disabled={disabled}
        required={required}
        value={date}
        onChange={(event) => onChange(joinDateTime(event.target.value, time))}
      />
      <TimePicker
        label={label}
        minuteStep={minuteStep}
        disabled={disabled}
        required={required}
        value={time}
        onChange={(next) => onChange(joinDateTime(date, next))}
      />
    </div>
  );
}
