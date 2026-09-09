// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { printDocument, printThermalReceipt, receiptMarkup } from "./printing";

afterEach(() => {
  vi.restoreAllMocks();
  document.head.querySelectorAll("[data-print-paper]").forEach((node) => node.remove());
  document.body.querySelectorAll("iframe").forEach((node) => node.remove());
});

const sale = {
  clinicName: "Clinique de Lunettes",
  invoiceNumber: "INV-2026-000012",
  issuedAt: "2026-09-09 10:15",
  currency: "HTG",
  lines: [{ description: "Classic frame", quantity: 2, unitPriceMinor: 150000, discountMinor: 20000 }],
  totalMinor: 280000,
  payments: [{ method: "Cash", amountMinor: 280000 }],
};

describe("printing paths", () => {
  it("prints a document on the clinic's paper size, not a roll", () => {
    vi.useFakeTimers();
    const print = vi.spyOn(window, "print").mockImplementation(() => undefined);
    printDocument("Letter");
    const style = document.head.querySelector("[data-print-paper]");
    expect(style?.textContent).toContain("size: Letter");
    expect(style?.textContent).toContain("margin: 12mm");
    vi.advanceTimersByTime(100);
    expect(print).toHaveBeenCalled();
    // The rule is removed again, so it cannot leak into the next print.
    expect(document.head.querySelector("[data-print-paper]")).toBeNull();
    vi.useRealTimers();
  });

  it("prints a receipt from its own isolated page, never the clinic stylesheet", () => {
    vi.useFakeTimers();
    printThermalReceipt("<div>Receipt</div>", "58mm");
    const frame = document.body.querySelector("iframe");
    expect(frame).not.toBeNull();
    const markup = frame!.contentDocument!.documentElement.innerHTML;
    expect(markup).toContain("size: 58mm auto");
    expect(markup).toContain("margin: 0");
    expect(markup).toContain("Receipt");
    // The receipt page carries no link to the application's own stylesheet.
    expect(frame!.contentDocument!.querySelectorAll("link[rel=stylesheet]")).toHaveLength(0);
    vi.useRealTimers();
  });

  it("does not put the on-screen print rule on a receipt", () => {
    vi.useFakeTimers();
    printThermalReceipt("<div>Receipt</div>");
    expect(document.head.querySelector("[data-print-paper]")).toBeNull();
    vi.useRealTimers();
  });

  it("lays a sale out for a narrow roll with its totals and tender", () => {
    const markup = receiptMarkup(sale);
    expect(markup).toContain("Clinique de Lunettes");
    expect(markup).toContain("INV-2026-000012");
    expect(markup).toContain("Classic frame");
    expect(markup).toContain("TOTAL");
    expect(markup).toContain("Cash");
  });

  it("escapes what a cashier typed rather than letting it become markup", () => {
    const markup = receiptMarkup({ ...sale, customer: '<img src=x onerror="alert(1)">' });
    expect(markup).not.toContain("<img src=x");
    expect(markup).toContain("&lt;img src=x");
  });

  it("shows an unpaid balance rather than implying the sale was settled", () => {
    const markup = receiptMarkup({ ...sale, payments: [{ method: "Cash", amountMinor: 100000 }], balanceMinor: 180000 });
    expect(markup).toContain("Balance owed");
  });
});
