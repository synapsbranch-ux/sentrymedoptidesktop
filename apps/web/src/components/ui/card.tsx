import * as React from "react";
import { cn } from "../../lib";
import { translateNode, useI18n } from "../../i18n";

export function Card({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("min-w-0 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] text-[var(--card-foreground)]", className)} {...props} />; }
export function CardHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("min-w-0 flex flex-col gap-1 p-4 sm:p-5", className)} {...props} />; }
export function CardTitle({ className, children, ...props }: React.HTMLAttributes<HTMLHeadingElement>) { const { t } = useI18n(); return <h3 className={cn("font-semibold tracking-tight", className)} {...props}>{translateNode(children, t)}</h3>; }
export function CardDescription({ className, children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) { const { t } = useI18n(); return <p className={cn("text-sm text-[var(--muted-foreground)]", className)} {...props}>{translateNode(children, t)}</p>; }
export function CardContent({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("min-w-0 p-4 pt-0 sm:p-5 sm:pt-0", className)} {...props} />; }
