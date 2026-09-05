import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib";

const buttonVariants = cva("inline-flex min-h-11 items-center justify-center gap-2 rounded-md text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 sm:min-h-10", {
  variants: {
    variant: {
      default: "bg-black px-4 text-white hover:bg-zinc-800",
      outline: "border border-zinc-300 bg-white px-4 text-black hover:bg-zinc-100",
      ghost: "px-3 text-zinc-700 hover:bg-zinc-100 hover:text-black",
      destructive: "bg-red-700 px-4 text-white hover:bg-red-800",
    },
    size: { default: "h-11 sm:h-10", sm: "h-11 px-3 text-xs sm:h-9 sm:min-h-9", icon: "h-11 w-11 p-0 sm:h-10 sm:min-h-10 sm:w-10" },
  },
  defaultVariants: { variant: "default", size: "default" },
});

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> { asChild?: boolean }

export function Button({ className, variant, size, asChild, ...props }: ButtonProps) {
  const Component = asChild ? Slot : "button";
  return <Component className={cn(buttonVariants({ variant, size }), className)} {...props} />;
}
