// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { AuthProvider } from "../auth";
import { I18nProvider } from "../i18n";
import { POSPage } from "./pos";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const frame = { id: "item-frame", sku: "FR-1", barcode: "5901234123457", category: "frame", name: "Classic frame", brand: "Acme", model: "", attributes: {}, supplierId: "", costMinor: 0, salePriceMinor: 150000, currency: "HTG", quantity: 3, reorderLevel: 0, trackStock: true, procedureCode: "", durationMinutes: 0, bookable: false, lowStock: false, version: 1, updatedAt: "" };

function stubApi(overrides: Record<string, unknown> = {}) {
  return vi.spyOn(api, "get").mockImplementation((path: string) => {
    if (path.startsWith("/inventory")) return Promise.resolve({ items: [frame], total: 1 } as never);
    if (path.startsWith("/pos/service-types")) return Promise.resolve({ items: [{ id: "svc", sku: "SVC-EXAM", name: "Eye examination", salePriceMinor: 200000, currency: "HTG", durationMinutes: 30, bookable: true, procedureCode: "", unpriced: false }] } as never);
    if (path.startsWith("/payment-methods")) return Promise.resolve({ items: [{ id: "pm_cash", name: "Cash" }, { id: "pm_card", name: "Card" }] } as never);
    if (path.startsWith("/cash-register")) return Promise.resolve({ open: true, id: "reg-1" } as never);
    if (path.startsWith("/pos/parked")) return Promise.resolve({ items: [] } as never);
    // The page reads the signed-in cashier and the clinic's paper sizes.
    if (path.startsWith("/setup/status")) return Promise.resolve({ required: false } as never);
    if (path.startsWith("/auth/me")) return Promise.resolve({ user: { id: "u1", username: "doctor", displayName: "Dr Joseph", role: "doctor" } } as never);
    if (path.startsWith("/settings")) return Promise.resolve({ settings: { clinic: { name: "Clinique de Lunettes" }, printing: { documentPaper: "A4", receiptWidth: "80mm" } }, versions: {} } as never);
    return Promise.resolve((overrides[path] ?? { items: [] }) as never);
  });
}

async function renderPOS() {
  render(<I18nProvider><AuthProvider><POSPage /></AuthProvider></I18nProvider>);
  await waitFor(() => expect(screen.getByText("Classic frame")).toBeTruthy());
}

describe("Point of Sale", () => {
  beforeEach(() => {
    // jsdom implements neither; the receipt path calls both on its own frame.
    vi.spyOn(window, "print").mockImplementation(() => undefined);
    vi.spyOn(window, "focus").mockImplementation(() => undefined);
    let counter = 0;
    Object.defineProperty(globalThis, "crypto", { value: { ...globalThis.crypto, randomUUID: () => `tender-${++counter}` }, configurable: true });
  });

  it("adds a scanned barcode to the cart without a pointer", async () => {
    stubApi();
    await renderPOS();
    const field = screen.getByLabelText("Scan a barcode");
    fireEvent.change(field, { target: { value: "5901234123457" } });
    await act(async () => { fireEvent.submit(field.closest("form")!); });
    await waitFor(() => expect(screen.getAllByText("Classic frame").length).toBeGreaterThan(1));
  });

  it("refuses a code that matches nothing rather than guessing", async () => {
    stubApi();
    await renderPOS();
    const field = screen.getByLabelText("Scan a barcode");
    fireEvent.change(field, { target: { value: "0000000000000" } });
    await act(async () => { fireEvent.submit(field.closest("form")!); });
    // Nothing was added: the only mention of the frame is the catalogue tile.
    await waitFor(() => expect(screen.getAllByText("Classic frame")).toHaveLength(1));
  });

  it("applies a line discount to the sale total", async () => {
    stubApi();
    await renderPOS();
    await act(async () => { fireEvent.click(screen.getByText("Classic frame")); });
    const discount = await screen.findByLabelText("Discount on Classic frame");
    await act(async () => { fireEvent.change(discount, { target: { value: "50000" } }); });
    // 150000 minor units less a 50000 discount leaves 100000 — one thousand.
    await waitFor(() => expect(screen.getByRole("button", { name: /1,000\.00/ })).toBeTruthy());
  });

  it("sends one payment per tender when a sale is split", async () => {
    stubApi();
    const post = vi.spyOn(api, "post").mockResolvedValue({ invoiceNumber: "INV-1", balanceMinor: 0, payments: [] } as never);
    await renderPOS();
    await act(async () => { fireEvent.click(screen.getByText("Classic frame")); });
    await act(async () => { fireEvent.click(await screen.findByRole("button", { name: /Add tender/ })); });
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: /Add tender/ })); });
    const amounts = screen.getAllByLabelText(/Amount \(HTG\)/);
    await act(async () => { fireEvent.change(amounts[0], { target: { value: "100000" } }); });
    await act(async () => { fireEvent.change(amounts[1], { target: { value: "50000" } }); });
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: /Complete sale/ })); });
    await waitFor(() => expect(post).toHaveBeenCalled());
    const body = post.mock.calls[0][1] as { payments: { amountMinor: number }[] };
    expect(body.payments.map((payment) => payment.amountMinor)).toEqual([100000, 50000]);
  });

  it("leaves the balance owed when no payment is taken", async () => {
    stubApi();
    const post = vi.spyOn(api, "post").mockResolvedValue({ invoiceNumber: "INV-2", balanceMinor: 150000, payments: [] } as never);
    await renderPOS();
    await act(async () => { fireEvent.click(screen.getByText("Classic frame")); });
    expect(screen.getByText(/Payment is never required/)).toBeTruthy();
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: /Complete sale/ })); });
    await waitFor(() => expect(post).toHaveBeenCalled());
    expect((post.mock.calls[0][1] as { payments: unknown[] }).payments).toEqual([]);
  });

  it("offers the clinic's own services beside the stock catalogue", async () => {
    stubApi();
    await renderPOS();
    expect(screen.getByText("Eye examination")).toBeTruthy();
  });

  it("sends the receipt to the thermal roll, not through the document print path", async () => {
    stubApi();
    vi.spyOn(api, "post").mockResolvedValue({ invoiceNumber: "INV-3", balanceMinor: 0, payments: [] } as never);
    await renderPOS();
    await act(async () => { fireEvent.click(screen.getByText("Classic frame")); });
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: /Complete sale/ })); });
    const frame = await waitFor(() => {
      const found = document.body.querySelector("iframe");
      if (!found) throw new Error("no receipt frame");
      return found;
    });
    const markup = frame.contentDocument!.documentElement.innerHTML;
    expect(markup).toContain("size: 80mm auto");
    expect(markup).toContain("Classic frame");
    // The on-page document paper rule is never involved in a receipt.
    expect(document.head.querySelector("[data-print-paper]")).toBeNull();
  });
});
