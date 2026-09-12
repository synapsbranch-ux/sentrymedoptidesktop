import * as React from "react";
import { Input } from "./input";
import { minorToInputValue, parseMoneyInput } from "../../money";

export interface MoneyInputProps
  extends Omit<React.ComponentPropsWithRef<"input">, "value" | "onChange" | "type"> {
  /** Current amount in integer minor units — never a decimal typed directly. */
  value: number;
  currency?: string;
  onValueChange(minor: number): void;
}

/**
 * The only control in this application a person should type a money amount
 * into. It always displays and accepts a plain decimal in major units
 * ("650" or "650.50") and converts to/from integer minor units at this one
 * boundary. Screens that instead bound a plain number input straight to a
 * *Minor field asked staff to type minor units by hand — 650 typed there
 * became 6.50 HTG once stored — which was the currency bug's root cause.
 *
 * The typed text is kept in local state so a value is never reformatted out
 * from under a person mid-keystroke (typing "6" would otherwise snap back to
 * "0.06"); it only reformats to a canonical "650.00" on blur.
 */
export function MoneyInput({ value, currency = "HTG", onValueChange, className, onFocus, onBlur, ...props }: MoneyInputProps) {
  const [text, setText] = React.useState(() => (value ? minorToInputValue(value, currency) : ""));
  const focused = React.useRef(false);

  React.useEffect(() => {
    if (!focused.current) setText(value ? minorToInputValue(value, currency) : "");
  }, [value, currency]);

  return (
    <div className="relative">
      <Input
        {...props}
        type="text"
        inputMode="decimal"
        autoComplete="off"
        className={cnClass(className)}
        value={text}
        onFocus={(event) => {
          focused.current = true;
          onFocus?.(event);
        }}
        onChange={(event) => {
          const next = event.target.value;
          if (!/^-?[\d.,]*$/.test(next)) return;
          setText(next);
          onValueChange(parseMoneyInput(next, currency));
        }}
        onBlur={(event) => {
          focused.current = false;
          const minor = parseMoneyInput(event.target.value, currency);
          setText(minor ? minorToInputValue(minor, currency) : "");
          onValueChange(minor);
          onBlur?.(event);
        }}
      />
      <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs font-semibold text-[var(--muted-foreground)]">
        {currency}
      </span>
    </div>
  );
}

function cnClass(className?: string) {
  return ["pr-14 text-right font-mono tabular-nums", className].filter(Boolean).join(" ");
}
