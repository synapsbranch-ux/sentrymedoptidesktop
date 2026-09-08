import * as React from "react";
import { cn } from "../../lib";
import { useI18n } from "../../i18n";

export function Input({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  const { t } = useI18n();
  return <input className={cn("flex h-11 w-full rounded-[var(--radius)] border border-[var(--input)] bg-[var(--card)] px-3 py-2 text-sm text-[var(--foreground)] placeholder:text-[var(--muted-foreground)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)] disabled:opacity-50", className)} {...props} placeholder={props.placeholder ? t(props.placeholder) : undefined} />;
}

export function Textarea({ className, ...props }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const { t } = useI18n();
  return <textarea className={cn("flex min-h-24 w-full rounded-[var(--radius)] border border-[var(--input)] bg-[var(--card)] px-3 py-2 text-sm text-[var(--foreground)] placeholder:text-[var(--muted-foreground)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)] disabled:opacity-50", className)} {...props} placeholder={props.placeholder ? t(props.placeholder) : undefined} />;
}

export function Select({ className, children, ...props }: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn("flex h-11 w-full rounded-[var(--radius)] border border-[var(--input)] bg-[var(--card)] px-3 text-sm text-[var(--foreground)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)] disabled:opacity-50", className)} {...props}>{children}</select>;
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  const { t } = useI18n();
  return <label className="grid gap-1.5 text-sm font-medium text-[var(--foreground)]"><span>{t(label)}</span>{children}{hint && <span className="text-xs font-normal text-[var(--muted-foreground)]">{t(hint)}</span>}</label>;
}

/**
 * The same layout as Field, for a control made of more than one element — a
 * search combobox with its result list, or a date paired with a time picker.
 *
 * A <label> may contain only one labelable control, and browsers forward a click
 * anywhere inside it to that control. Wrapping a combobox in Field therefore made
 * every result click land on the search input instead of selecting the result:
 * the picker looked like it did nothing. Composite controls label themselves.
 */
export function FieldGroup({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  const { t } = useI18n();
  return <div role="group" aria-label={t(label)} className="grid gap-1.5 text-sm font-medium text-[var(--foreground)]"><span>{t(label)}</span>{children}{hint && <span className="text-xs font-normal text-[var(--muted-foreground)]">{t(hint)}</span>}</div>;
}
