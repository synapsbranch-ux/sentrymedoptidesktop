import * as React from "react";
import { cn } from "../../lib";

export function Badge({ children, tone = "neutral" }: { children: React.ReactNode; tone?: "neutral" | "success" | "warning" | "danger" }) {
  const tones = { neutral: "border-zinc-300 bg-zinc-50 text-zinc-700", success: "border-emerald-300 bg-emerald-50 text-emerald-800", warning: "border-amber-300 bg-amber-50 text-amber-900", danger: "border-red-300 bg-red-50 text-red-800" };
  return <span className={cn("inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-semibold capitalize", tones[tone])}>{children}</span>;
}
export function Table({ children }: { children: React.ReactNode }) { return <div className="overflow-x-auto"><table className="w-full text-left text-sm">{children}</table></div>; }
export function Th({ children, className }: { children: React.ReactNode; className?: string }) { return <th className={cn("whitespace-nowrap border-b border-zinc-200 bg-zinc-50 px-4 py-3 text-[11px] font-bold uppercase tracking-wide text-zinc-500", className)}>{children}</th>; }
export function Td({ children, className }: { children: React.ReactNode; className?: string }) { return <td className={cn("border-b border-zinc-100 px-4 py-3 align-middle", className)}>{children}</td>; }
export function Skeleton({ className }: { className?: string }) { return <div className={cn("animate-pulse rounded-md bg-zinc-100", className)} />; }
export function EmptyState({ title, description, action }: { title: string; description: string; action?: React.ReactNode }) { return <div className="grid min-h-56 place-items-center p-8 text-center"><div><div className="mx-auto mb-4 h-10 w-10 rounded-full border border-zinc-300 bg-zinc-50" /><h3 className="font-semibold">{title}</h3><p className="mt-1 max-w-sm text-sm text-zinc-500">{description}</p>{action && <div className="mt-4">{action}</div>}</div></div>; }
export function ErrorState({ message, retry }: { message: string; retry?: () => void }) { return <div role="alert" className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-900"><strong>Unable to load this section.</strong><p className="mt-1">{message}</p>{retry && <button className="mt-2 underline" onClick={retry}>Try again</button>}</div>; }

