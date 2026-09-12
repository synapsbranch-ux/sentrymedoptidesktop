import * as React from "react";
import { money } from "../lib";

/**
 * Printing is split by output device, not routed through one generic call.
 *
 * A till receipt goes to a 58 or 80 mm thermal roll with no margins and a
 * continuous page length; an invoice, prescription, lab requisition or contract
 * goes to A4 or Letter with margins. Those are different page geometries, and a
 * single shared print function can only ever be wrong for one of them — which is
 * how receipts end up printed across a full sheet and invoices end up cut off.
 *
 * So there are two functions here and they never share a stylesheet.
 */

export type ReceiptWidth = "58mm" | "80mm";
export type DocumentPaper = "A4" | "Letter";

/**
 * Standard-paper documents. The page is already on screen inside a
 * `.print-area.document-print` element; this sets the paper size the clinic
 * uses and hands off to the browser's own print pipeline, which is all a
 * normal printer needs.
 */
export function printDocument(paper: DocumentPaper = "A4") {
  const style = document.createElement("style");
  style.setAttribute("data-print-paper", "true");
  style.textContent = `@page { size: ${paper}; margin: 12mm; }`;
  document.head.append(style);
  window.setTimeout(() => {
    // window.print() blocks until the native print/save dialog closes on
    // most platforms; if it throws instead (a webview with no print handler
    // configured can reject rather than silently no-op), the paper-size rule
    // must still be removed or it corrupts the geometry of every print after
    // it, including ones from a page the user has since navigated away to.
    try {
      window.print();
    } finally {
      style.remove();
    }
  }, 50);
}

/**
 * Thermal receipts. A browser cannot give one document two page geometries, so
 * the receipt is printed from its own isolated iframe with its own `@page`
 * rule — the clinic application's stylesheet never reaches it. The roll is
 * continuous, so the page height is `auto` and every margin is zero.
 */
export function printThermalReceipt(markup: string, width: ReceiptWidth = "80mm") {
  const frame = document.createElement("iframe");
  frame.setAttribute("aria-hidden", "true");
  frame.style.cssText = "position:fixed;right:0;bottom:0;width:0;height:0;border:0;";
  document.body.append(frame);
  const view = frame.contentWindow;
  const receiptDocument = frame.contentDocument;
  if (!view || !receiptDocument) { frame.remove(); return; }
  receiptDocument.open();
  receiptDocument.write(`<!doctype html><html><head><meta charset="utf-8"><style>
    @page { size: ${width} auto; margin: 0; }
    html, body { margin: 0; padding: 0; width: ${width}; background: #fff; color: #000; }
    body { font-family: "Courier New", ui-monospace, monospace; font-size: 11px; line-height: 1.35; padding: 3mm 2mm; }
    .center { text-align: center; }
    .row { display: flex; justify-content: space-between; gap: 6px; }
    .row span:last-child { white-space: nowrap; }
    .rule { border-top: 1px dashed #000; margin: 4px 0; }
    .bold { font-weight: 700; }
    .big { font-size: 13px; }
    .small { font-size: 9px; }
  </style></head><body>${markup}</body></html>`);
  receiptDocument.close();
  const cleanUp = () => window.setTimeout(() => frame.remove(), 1000);
  view.addEventListener("afterprint", cleanUp);
  // Give the isolated document a tick to lay out before the dialog opens.
  window.setTimeout(() => { view.focus(); view.print(); cleanUp(); }, 100);
}

export interface ReceiptLine { description: string; quantity: number; unitPriceMinor: number; discountMinor?: number }
export interface ReceiptData {
  clinicName: string;
  clinicAddress?: string;
  clinicPhone?: string;
  invoiceNumber: string;
  issuedAt: string;
  cashier?: string;
  customer?: string;
  currency: string;
  lines: ReceiptLine[];
  discountMinor?: number;
  totalMinor: number;
  payments?: { method: string; amountMinor: number }[];
  balanceMinor?: number;
  footer?: string;
}

const escapeHTML = (value: string) => value.replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[character] as string));

/** Builds the narrow-roll markup for one sale. Text only — a thermal head prints no images worth having. */
export function receiptMarkup(receipt: ReceiptData): string {
  const row = (left: string, right: string, className = "") => `<div class="row ${className}"><span>${escapeHTML(left)}</span><span>${escapeHTML(right)}</span></div>`;
  const lines = receipt.lines.map((line) => {
    const gross = line.quantity * line.unitPriceMinor - (line.discountMinor ?? 0);
    return `${row(line.description, money(gross, receipt.currency))}${line.quantity > 1 || line.discountMinor ? `<div class="small">${escapeHTML(`${line.quantity} × ${money(line.unitPriceMinor, receipt.currency)}${line.discountMinor ? ` less ${money(line.discountMinor, receipt.currency)}` : ""}`)}</div>` : ""}`;
  }).join("");
  const payments = (receipt.payments ?? []).map((payment) => row(payment.method, money(payment.amountMinor, receipt.currency))).join("");
  return [
    `<div class="center bold big">${escapeHTML(receipt.clinicName)}</div>`,
    receipt.clinicAddress ? `<div class="center small">${escapeHTML(receipt.clinicAddress)}</div>` : "",
    receipt.clinicPhone ? `<div class="center small">${escapeHTML(receipt.clinicPhone)}</div>` : "",
    `<div class="rule"></div>`,
    row("Receipt", receipt.invoiceNumber),
    row("Date", receipt.issuedAt),
    receipt.cashier ? row("Served by", receipt.cashier) : "",
    receipt.customer ? row("Customer", receipt.customer) : "",
    `<div class="rule"></div>`,
    lines,
    `<div class="rule"></div>`,
    receipt.discountMinor ? row("Discount", `-${money(receipt.discountMinor, receipt.currency)}`) : "",
    row("TOTAL", money(receipt.totalMinor, receipt.currency), "bold big"),
    payments,
    receipt.balanceMinor ? row("Balance owed", money(receipt.balanceMinor, receipt.currency), "bold") : "",
    `<div class="rule"></div>`,
    `<div class="center small">${escapeHTML(receipt.footer ?? "Thank you")}</div>`,
  ].join("");
}

/** Prints a sale to the thermal roll. Never use this for anything on paper. */
export function printReceipt(receipt: ReceiptData, width: ReceiptWidth = "80mm") {
  printThermalReceipt(receiptMarkup(receipt), width);
}

/** A button that prints the enclosing `.document-print` article on standard paper. */
export function PrintDocumentButton({ paper, children, className }: { paper?: DocumentPaper; children: React.ReactNode; className?: string }) {
  return <button type="button" className={className} onClick={() => printDocument(paper)}>{children}</button>;
}
