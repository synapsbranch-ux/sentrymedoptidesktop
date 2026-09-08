import { Ban, CalendarClock, CheckCheck, CircleCheck, Clock3, Eye, Hourglass, Stethoscope, UserCheck, UserX, Wallet } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { cn } from "../lib";
import { useI18n } from "../i18n";

/**
 * C1: one definition of what every appointment status and waiting-room stage
 * looks like, used by the calendar, the appointment list and the waiting room so
 * the same status never reads differently in two places.
 *
 * Colour is never the only signal. Every appearance carries a label and an icon,
 * so the status stays readable for a colour-blind reader and in black-and-white
 * print, and the no-show state is additionally striped rather than relying on
 * being a darker red than "cancelled".
 */

export interface StatusAppearance {
  label: string;
  Icon: LucideIcon;
  /** Border, background and text for a pill or a card edge. */
  className: string;
  /** Solid fill for a dense calendar chip. */
  chipClassName: string;
  /** A left edge that survives a greyscale print. */
  edgeClassName: string;
  striped?: boolean;
}

const appearances = {
  scheduled: {
    label: "Scheduled", Icon: CalendarClock,
    className: "border-blue-300 bg-blue-50 text-blue-900",
    chipClassName: "bg-blue-100 text-blue-900 border-blue-300",
    edgeClassName: "border-l-4 border-l-blue-500",
  },
  confirmed: {
    label: "Confirmed", Icon: CircleCheck,
    className: "border-emerald-300 bg-emerald-50 text-emerald-900",
    chipClassName: "bg-emerald-100 text-emerald-900 border-emerald-300",
    edgeClassName: "border-l-4 border-l-emerald-600",
  },
  waiting: {
    label: "Checked in / waiting", Icon: Hourglass,
    className: "border-amber-300 bg-amber-50 text-amber-900",
    chipClassName: "bg-amber-100 text-amber-900 border-amber-300",
    edgeClassName: "border-l-4 border-l-amber-500",
  },
  in_consultation: {
    label: "In consultation", Icon: Stethoscope,
    className: "border-purple-300 bg-purple-50 text-purple-900",
    chipClassName: "bg-purple-100 text-purple-900 border-purple-300",
    edgeClassName: "border-l-4 border-l-purple-600",
  },
  completed: {
    label: "Completed", Icon: CheckCheck,
    className: "border-zinc-300 bg-zinc-100 text-zinc-700",
    chipClassName: "bg-zinc-200 text-zinc-700 border-zinc-300",
    edgeClassName: "border-l-4 border-l-zinc-400",
  },
  cancelled: {
    label: "Cancelled", Icon: Ban,
    className: "border-red-300 bg-red-50 text-red-800",
    chipClassName: "bg-red-100 text-red-800 border-red-300",
    edgeClassName: "border-l-4 border-l-red-500",
  },
  no_show: {
    label: "No-show", Icon: UserX,
    className: "border-red-800 bg-red-100 text-red-950",
    chipClassName: "bg-red-200 text-red-950 border-red-800",
    edgeClassName: "border-l-4 border-l-red-900",
    striped: true,
  },
} satisfies Record<string, StatusAppearance>;

const unknownAppearance: StatusAppearance = {
  label: "Unknown", Icon: Clock3,
  className: "border-zinc-300 bg-zinc-50 text-zinc-700",
  chipClassName: "bg-zinc-100 text-zinc-700 border-zinc-300",
  edgeClassName: "border-l-4 border-l-zinc-300",
};

/** A diagonal hatch, so "no-show" is distinguishable without seeing the hue. */
export const stripedBackground =
  "repeating-linear-gradient(45deg, transparent, transparent 4px, rgba(127,29,29,.18) 4px, rgba(127,29,29,.18) 8px)";

/** Appointment status as stored by the server. */
export function statusAppearance(status: string): StatusAppearance {
  switch (status) {
    case "scheduled": return appearances.scheduled;
    case "confirmed": return appearances.confirmed;
    case "checked_in":
    case "waiting": return { ...appearances.waiting, label: status === "checked_in" ? "Checked in" : "Waiting" };
    case "in_consultation": return appearances.in_consultation;
    case "completed": return appearances.completed;
    case "cancelled": return appearances.cancelled;
    case "no_show": return appearances.no_show;
    default: return { ...unknownAppearance, label: humanise(status) };
  }
}

/** Waiting-room stage, mapped onto the same seven colours. */
export function stageAppearance(stage: string): StatusAppearance {
  switch (stage) {
    case "checked_in": return { ...appearances.waiting, label: "Checked in" };
    case "waiting_nurse": return { ...appearances.waiting, label: "Waiting for nurse" };
    case "pre_test": return { ...appearances.waiting, label: "Pre-test", Icon: Eye };
    case "waiting_doctor": return { ...appearances.waiting, label: "Waiting for doctor" };
    case "in_consultation": return appearances.in_consultation;
    case "checkout": return { ...appearances.confirmed, label: "Checkout", Icon: Wallet };
    case "completed": return { ...appearances.completed, label: "Completed", Icon: UserCheck };
    default: return { ...unknownAppearance, label: humanise(stage) };
  }
}

function humanise(value: string) {
  return value ? value.replaceAll("_", " ") : "Unknown";
}

/** The status as a pill: colour plus icon plus words, in that order of reliance. */
export function StatusPill({ appearance, className, compact }: { appearance: StatusAppearance; className?: string; compact?: boolean }) {
  const { t } = useI18n();
  const { Icon } = appearance;
  return (
    <span
      className={cn("inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-2 py-0.5 text-[11px] font-semibold", appearance.className, className)}
      style={appearance.striped ? { backgroundImage: stripedBackground } : undefined}
    >
      <Icon aria-hidden className={compact ? "h-3 w-3" : "h-3.5 w-3.5"} />
      {t(appearance.label)}
    </span>
  );
}

/** The legend that makes the colour scheme learnable on the calendar screen. */
export function StatusLegend({ className }: { className?: string }) {
  const { t } = useI18n();
  return (
    <div className={cn("flex flex-wrap items-center gap-1.5", className)} aria-label={t("Appointment status colours")}>
      {(["scheduled", "confirmed", "waiting", "in_consultation", "completed", "cancelled", "no_show"] as const).map((key) => (
        <StatusPill key={key} compact appearance={appearances[key]} />
      ))}
    </div>
  );
}
