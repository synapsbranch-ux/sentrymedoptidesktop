import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib";
import { translateNode, useI18n } from "../../i18n";

const buttonVariants = cva("inline-flex min-h-11 items-center justify-center gap-2 rounded-[var(--radius)] text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)] focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 sm:min-h-10", {
  variants: {
    variant: {
      default: "bg-[var(--primary)] px-4 text-[var(--primary-foreground)] hover:bg-[var(--primary-hover)]",
      outline: "border border-[var(--input)] bg-[var(--card)] px-4 text-[var(--foreground)] hover:bg-[var(--muted)]",
      ghost: "px-3 text-[var(--muted-foreground)] hover:bg-[var(--muted)] hover:text-[var(--foreground)]",
      destructive: "bg-red-700 px-4 text-white hover:bg-red-800",
    },
    size: { default: "h-11 sm:h-10", sm: "h-11 px-3 text-xs sm:h-9 sm:min-h-9", icon: "h-11 w-11 p-0 sm:h-10 sm:min-h-10 sm:w-10" },
  },
  defaultVariants: { variant: "default", size: "default" },
});

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> { asChild?: boolean }

export function Button({ className, variant, size, asChild, ...props }: ButtonProps) {
  const Component = asChild ? Slot : "button";
  const { t } = useI18n();
  const children = asChild ? props.children : translateNode(props.children, t);
  return <Component className={cn(buttonVariants({ variant, size }), className)} {...props}>{children}</Component>;
}
