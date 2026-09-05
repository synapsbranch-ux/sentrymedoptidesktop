import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "../../lib";

export const Dialog = DialogPrimitive.Root;
export const DialogTrigger = DialogPrimitive.Trigger;
export const DialogClose = DialogPrimitive.Close;
export function DialogContent({ className, children, ...props }: DialogPrimitive.DialogContentProps) {
  return <DialogPrimitive.Portal><DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/50" /><DialogPrimitive.Content className={cn("fixed inset-0 z-50 h-dvh w-full overflow-y-auto bg-white px-4 pb-[max(1rem,env(safe-area-inset-bottom))] pt-[max(1rem,env(safe-area-inset-top))] shadow-xl focus:outline-none sm:left-1/2 sm:top-1/2 sm:inset-auto sm:max-h-[90vh] sm:h-auto sm:w-[calc(100%-2rem)] sm:max-w-2xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-lg sm:border sm:border-zinc-200 sm:p-6", className)} {...props}>{children}<DialogPrimitive.Close aria-label="Close" className="absolute right-3 top-[max(.75rem,env(safe-area-inset-top))] grid h-11 w-11 place-items-center rounded-md text-zinc-500 hover:bg-zinc-100 sm:right-4 sm:top-4 sm:h-8 sm:w-8"><X className="h-5 w-5 sm:h-4 sm:w-4" /></DialogPrimitive.Close></DialogPrimitive.Content></DialogPrimitive.Portal>;
}
export function DialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("mb-5 grid gap-1", className)} {...props} />; }
export function DialogTitle({ className, ...props }: DialogPrimitive.DialogTitleProps) { return <DialogPrimitive.Title className={cn("text-lg font-bold", className)} {...props} />; }
export function DialogDescription({ className, ...props }: DialogPrimitive.DialogDescriptionProps) { return <DialogPrimitive.Description className={cn("text-sm text-zinc-500", className)} {...props} />; }
export function DialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) { return <div className={cn("mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)} {...props} />; }
