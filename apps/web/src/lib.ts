import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

function applicationLocale() {
  return typeof document === "undefined" ? undefined : document.documentElement.lang || undefined;
}

export function money(minor: number, currency = "HTG") {
  return new Intl.NumberFormat(applicationLocale(), { style: "currency", currency }).format(minor / 100);
}

export function dateTime(value?: string) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(applicationLocale(), { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export function todayInput() {
  const date = new Date();
  date.setMinutes(date.getMinutes() - date.getTimezoneOffset());
  return date.toISOString().slice(0, 10);
}
