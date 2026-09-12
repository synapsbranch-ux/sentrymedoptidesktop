/**
 * The one place this application converts between an integer count of minor
 * currency units (what the server stores and returns — see docs/DATABASE.md)
 * and the decimal amount a person types or reads on screen.
 *
 * Every other screen that needs to show or collect an amount should call
 * formatMoney/parseMoneyInput (or use <MoneyInput>) rather than dividing or
 * multiplying by 100 itself. A form that asked staff to type minor units
 * directly — "650" meaning 6.50 HTG instead of 650 HTG — was the root cause of
 * the currency bug: nobody thinks in centimes, so everybody typed the face
 * value and it landed one hundred times too small.
 */

function applicationLocale() {
  return typeof document === "undefined" ? undefined : document.documentElement.lang || undefined;
}

/** Every currency this clinic uses today (HTG, USD) has two decimal places. */
function fractionDigits(_currency: string): number {
  return 2;
}

/** Formats an integer minor-unit amount for display, e.g. 65000 -> "650.00 HTG". */
export function formatMoney(minor: number, currency = "HTG"): string {
  const scale = 10 ** fractionDigits(currency);
  return new Intl.NumberFormat(applicationLocale(), { style: "currency", currency }).format((minor || 0) / scale);
}

/**
 * Formats an integer minor-unit amount as a plain, unlocalized decimal for an
 * editable field, e.g. 65000 -> "650.00". Deliberately has no thousands
 * separator so it round-trips exactly through parseMoneyInput.
 */
export function minorToInputValue(minor: number, currency = "HTG"): string {
  const digits = fractionDigits(currency);
  const scale = 10 ** digits;
  const value = Math.round(minor || 0);
  const sign = value < 0 ? "-" : "";
  const absolute = Math.abs(value);
  const whole = Math.floor(absolute / scale);
  const fraction = String(absolute % scale).padStart(digits, "0");
  return digits > 0 ? `${sign}${whole}.${fraction}` : `${sign}${whole}`;
}

/**
 * Parses what a person typed — "650", "650.5", "6,500.00", "-12" — into exact
 * integer minor units. This works on the digit string itself; it never
 * multiplies a floating-point number by 100, so it cannot inherit binary
 * floating-point rounding error the way `Math.round(parseFloat(x) * 100)`
 * can for values such as 1.005.
 */
export function parseMoneyInput(input: string, currency = "HTG"): number {
  const digits = fractionDigits(currency);
  let text = (input ?? "").trim().replace(/[^\d.,-]/g, "");
  if (text === "" || text === "-") return 0;
  const negative = text.startsWith("-");
  text = text.replace(/-/g, "");
  if (text === "") return 0;

  // Which separator (if any) is the decimal point, as opposed to a thousands
  // grouping mark? If both "." and "," appear, whichever appears last is the
  // decimal point. If only one kind appears exactly once, it is a decimal
  // point unless it is followed by exactly three digits — "6,500" and
  // "6.500" read as six thousand five hundred, not 6.5, since every currency
  // this clinic uses has two decimal places. Two or more of the same
  // separator ("6,500,000") is always grouping, never a decimal point.
  const dotCount = (text.match(/\./g) ?? []).length;
  const commaCount = (text.match(/,/g) ?? []).length;
  let decimalChar = "";
  if (dotCount > 0 && commaCount > 0) {
    decimalChar = text.lastIndexOf(".") > text.lastIndexOf(",") ? "." : ",";
  } else if (dotCount === 1 || commaCount === 1) {
    const only = dotCount === 1 ? "." : ",";
    const after = text.slice(text.indexOf(only) + 1);
    if (after.length !== 3) decimalChar = only;
  }

  let rawWhole = text;
  let rawFraction = "";
  if (decimalChar) {
    const at = text.lastIndexOf(decimalChar);
    rawWhole = text.slice(0, at);
    rawFraction = text.slice(at + 1);
  }
  const whole = rawWhole.replace(/[.,]/g, "").replace(/^0+(?=\d)/, "") || "0";
  const fraction = (rawFraction.replace(/[.,]/g, "") + "0".repeat(digits)).slice(0, digits);
  const minor = Number(whole + fraction);
  if (!Number.isFinite(minor)) return 0;
  return negative ? -minor : minor;
}

export function addMoney(a: number, b: number): number {
  return a + b;
}
export function subtractMoney(a: number, b: number): number {
  return a - b;
}
export function sumMoney(amounts: number[]): number {
  return amounts.reduce((total, amount) => total + amount, 0);
}
export function calculateBalance(totalMinor: number, paidMinor: number): number {
  return totalMinor - paidMinor;
}

/**
 * Client-side preview only — splits totalMinor into a percent% share (rounded
 * to the nearest minor unit) and the remainder, so a form can show an
 * estimated insurance/patient split as staff type. The server independently
 * computes and validates the authoritative split; this never substitutes for
 * that check.
 */
export function splitByPercent(totalMinor: number, percent: number): [insuranceShare: number, remainder: number] {
  if (!totalMinor || totalMinor <= 0 || !percent || percent <= 0) return [0, totalMinor || 0];
  const share = Math.min(totalMinor, Math.floor((totalMinor * percent) / 100 + 0.5));
  return [share, totalMinor - share];
}
