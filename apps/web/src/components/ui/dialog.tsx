import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "../../lib";

export const Dialog = DialogPrimitive.Root;
export const DialogTrigger = DialogPrimitive.Trigger;
export const DialogClose = DialogPrimitive.Close;
export function DialogContent({ className, children, ...props }: DialogPrimitive.DialogContentProps) {
  return <DialogPrimitive.Portal>
    <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/50" />
    <DialogPrimitive.Content
      className={cn(
        "fixed inset-x-0 bottom-0 z-50 max-h-[calc(100dvh-.5rem)] w-full max-w-2xl touch-pan-y overflow-x-hidden overflow-y-auto overscroll-contain rounded-t-xl border border-[var(--border)] bg-[var(--card)] p-4 pb-[max(1rem,env(safe-area-inset-bottom))] text-[var(--card-foreground)] shadow-xl focus:outline-none [-webkit-overflow-scrolling:touch] sm:bottom-auto sm:left-1/2 sm:top-1/2 sm:max-h-[calc(100dvh-2rem)] sm:w-[calc(100dvw-2rem)] sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-[var(--radius)] sm:p-6",
        className,
      )}
      {...props}
    >
      <DialogPrimitive.Close aria-label="Close" className="sticky top-0 z-30 -mb-10 ml-auto grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] text-[var(--muted-foreground)] shadow-sm hover:bg-[var(--muted)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)]">
        <X className="h-4 w-4" />
      </DialogPrimitive.Close>
      <div className="min-w-0">{children}</div>
    </DialogPrimitive.Content>
  </DialogPrimitive.Portal>;
}
export function DialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("mb-5 grid min-w-0 gap-1 pr-10", className)} {...props} />; }
export function DialogTitle({ className, ...props }: DialogPrimitive.DialogTitleProps) { return <DialogPrimitive.Title className={cn("text-lg font-bold", className)} {...props} />; }
export function DialogDescription({ className, ...props }: DialogPrimitive.DialogDescriptionProps) { return <DialogPrimitive.Description className={cn("text-sm text-[var(--muted-foreground)]", className)} {...props} />; }
export function DialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("sticky bottom-0 z-20 -mx-4 -mb-4 mt-6 flex flex-col-reverse gap-2 border-t border-[var(--border)] bg-[var(--card)] p-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:-mx-6 sm:-mb-6 sm:flex-row sm:justify-end sm:p-6", className)} {...props} />; }
