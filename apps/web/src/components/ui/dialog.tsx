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
        "fixed left-1/2 top-1/2 z-50 max-h-[calc(100dvh-1rem)] w-[calc(100dvw-1rem)] max-w-2xl -translate-x-1/2 -translate-y-1/2 overflow-x-hidden overflow-y-auto overscroll-contain rounded-lg border border-zinc-200 bg-white p-4 shadow-xl focus:outline-none sm:max-h-[calc(100dvh-2rem)] sm:w-[calc(100dvw-2rem)] sm:p-6",
        className,
      )}
      {...props}
    >
      <DialogPrimitive.Close aria-label="Close" className="sticky top-0 z-30 -mb-9 ml-auto grid h-9 w-9 shrink-0 place-items-center rounded-md border border-zinc-200 bg-white text-zinc-500 shadow-sm hover:bg-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black">
        <X className="h-4 w-4" />
      </DialogPrimitive.Close>
      <div className="min-w-0">{children}</div>
    </DialogPrimitive.Content>
  </DialogPrimitive.Portal>;
}
export function DialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("mb-5 grid min-w-0 gap-1 pr-10", className)} {...props} />; }
export function DialogTitle({ className, ...props }: DialogPrimitive.DialogTitleProps) { return <DialogPrimitive.Title className={cn("text-lg font-bold", className)} {...props} />; }
export function DialogDescription({ className, ...props }: DialogPrimitive.DialogDescriptionProps) { return <DialogPrimitive.Description className={cn("text-sm text-zinc-500", className)} {...props} />; }
export function DialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("sticky bottom-0 z-20 -mx-4 -mb-4 mt-6 flex flex-col-reverse gap-2 border-t border-zinc-200 bg-white p-4 sm:-mx-6 sm:-mb-6 sm:flex-row sm:justify-end sm:p-6", className)} {...props} />; }
