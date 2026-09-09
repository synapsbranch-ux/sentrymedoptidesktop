import * as React from "react";
import { cn } from "../../lib";
import { translateNode, useI18n } from "../../i18n";

export function Badge({ children, tone = "neutral", className }: { children: React.ReactNode; tone?: "neutral" | "success" | "warning" | "danger"; className?: string }) {
  const { t } = useI18n();
  const tones = { neutral: "border-zinc-300 bg-zinc-50 text-zinc-700", success: "border-emerald-300 bg-emerald-50 text-emerald-800", warning: "border-amber-300 bg-amber-50 text-amber-900", danger: "border-red-300 bg-red-50 text-red-800" };
  return <span className={cn("inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-semibold capitalize", tones[tone], className)}>{translateNode(children, t)}</span>;
}
export function Table({ children }: { children: React.ReactNode }) { return <div className="max-w-full overflow-x-auto overscroll-x-contain"><table className="w-full min-w-max text-left text-sm">{children}</table></div>; }
export function Th({ children, className }: { children: React.ReactNode; className?: string }) { const { t } = useI18n(); return <th className={cn("whitespace-nowrap border-b border-zinc-200 bg-zinc-50 px-4 py-3 text-[11px] font-bold uppercase tracking-wide text-zinc-500", className)}>{translateNode(children, t)}</th>; }
export function Td({ children, className }: { children: React.ReactNode; className?: string }) { return <td className={cn("border-b border-zinc-100 px-4 py-3 align-middle", className)}>{children}</td>; }
export function Skeleton({ className }: { className?: string }) { return <div className={cn("animate-pulse rounded-md bg-zinc-100", className)} />; }
export function EmptyState({ title, description, action }: { title: string; description: string; action?: React.ReactNode }) { const { t } = useI18n(); return <div className="grid min-h-56 place-items-center p-8 text-center"><div><div className="mx-auto mb-4 h-10 w-10 rounded-full border border-zinc-300 bg-zinc-50" /><h3 className="font-semibold">{t(title)}</h3><p className="mt-1 max-w-sm text-sm text-zinc-500">{t(description)}</p>{action && <div className="mt-4">{action}</div>}</div></div>; }
export function ErrorState({ message, retry }: { message: string; retry?: () => void }) { const { t } = useI18n(); return <div role="alert" className="min-w-0 break-words rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-900"><strong>{t("This section could not connect to the clinic server.")}</strong><p className="mt-1">{t(message)}</p><p className="mt-2 text-xs opacity-80">{t("On a phone or tablet, verify the clinic Wi-Fi and keep the SentryMed desktop server running.")}</p>{retry && <button className="mt-2 min-h-11 font-semibold underline" onClick={retry}>{t("Reconnect and try again")}</button>}</div>; }

/**
 * The paging control every server-paged table uses. It shows which rows are on
 * screen out of how many exist, so it is obvious that the rest are on the
 * server rather than missing.
 */
export function Pager({ page, pageSize, total, hasMore, onPrevious, onNext }: { page: number; pageSize: number; total: number; hasMore: boolean; onPrevious(): void; onNext(): void }) {
  const { t } = useI18n();
  if (total === 0) return null;
  const lastPage = Math.max(1, Math.ceil(total / pageSize));
  return <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-sm text-[var(--muted-foreground)]">
    <span>{`${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, total)} ${t("of")} ${total}`}</span>
    <div className="flex items-center gap-2">
      <button type="button" className="min-h-9 rounded-[var(--radius)] border border-[var(--input)] px-3 text-xs font-semibold disabled:opacity-40" disabled={page <= 1} onClick={onPrevious}>{t("Previous")}</button>
      <span className="font-mono text-xs">{page} / {lastPage}</span>
      <button type="button" className="min-h-9 rounded-[var(--radius)] border border-[var(--input)] px-3 text-xs font-semibold disabled:opacity-40" disabled={!hasMore} onClick={onNext}>{t("Next")}</button>
    </div>
  </div>;
}
