// @vitest-environment jsdom
import * as React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { SignaturePad, SignatureSettings, drawStrokes, strokesAreEmpty } from "./signature";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

beforeEach(() => {
  URL.createObjectURL = vi.fn(() => "blob:clinic/signature");
  URL.revokeObjectURL = vi.fn();
  // jsdom has no 2D canvas implementation; the pad only needs the calls to land.
  HTMLCanvasElement.prototype.getContext = vi.fn(() => ({
    clearRect: vi.fn(), beginPath: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(),
    stroke: vi.fn(), arc: vi.fn(), fill: vi.fn(),
  })) as never;
  HTMLCanvasElement.prototype.toBlob = function (callback: BlobCallback) { callback(new Blob(["png"], { type: "image/png" })); } as never;
  Element.prototype.setPointerCapture = vi.fn();
  Element.prototype.releasePointerCapture = vi.fn();
});

describe("stroke handling", () => {
  it("treats a pad with no ink as empty", () => {
    expect(strokesAreEmpty([])).toBe(true);
    expect(strokesAreEmpty([[], []])).toBe(true);
    expect(strokesAreEmpty([[{ x: 1, y: 1 }]])).toBe(false);
  });

  it("draws a single tap as a dot rather than nothing", () => {
    const context = { clearRect: vi.fn(), beginPath: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(), stroke: vi.fn(), arc: vi.fn(), fill: vi.fn() };
    drawStrokes(context as unknown as CanvasRenderingContext2D, [[{ x: 5, y: 5 }]], 600, 200);
    expect(context.arc).toHaveBeenCalled();
    expect(context.stroke).not.toHaveBeenCalled();
  });

  it("draws a multi-point stroke as a line and keeps strokes separate", () => {
    const context = { clearRect: vi.fn(), beginPath: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(), stroke: vi.fn(), arc: vi.fn(), fill: vi.fn() };
    drawStrokes(context as unknown as CanvasRenderingContext2D, [
      [{ x: 1, y: 1 }, { x: 2, y: 2 }],
      [{ x: 8, y: 8 }, { x: 9, y: 9 }],
    ], 600, 200);
    expect(context.beginPath).toHaveBeenCalledTimes(2);
    expect(context.stroke).toHaveBeenCalledTimes(2);
  });
});

describe("SignaturePad", () => {
  it("captures a drawn signature from pointer events, which covers mouse, trackpad and finger", async () => {
    const onDrawn = vi.fn();
    render(<I18nProvider><SignaturePad busy={false} onDrawn={onDrawn} /></I18nProvider>);
    const canvas = screen.getByLabelText("Signature pad");
    expect(canvas.className).toContain("touch-none");
    expect((screen.getByRole("button", { name: "Save drawn signature" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.pointerDown(canvas, { pointerId: 1, clientX: 10, clientY: 10 });
    fireEvent.pointerMove(canvas, { pointerId: 1, clientX: 40, clientY: 30 });
    fireEvent.pointerUp(canvas, { pointerId: 1 });
    await waitFor(() => expect((screen.getByRole("button", { name: "Save drawn signature" }) as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(screen.getByRole("button", { name: "Save drawn signature" }));
    await waitFor(() => expect(onDrawn).toHaveBeenCalledWith(expect.any(Blob)));
  });

  it("clears the pad", async () => {
    render(<I18nProvider><SignaturePad busy={false} onDrawn={() => undefined} /></I18nProvider>);
    const canvas = screen.getByLabelText("Signature pad");
    fireEvent.pointerDown(canvas, { pointerId: 1, clientX: 10, clientY: 10 });
    fireEvent.pointerUp(canvas, { pointerId: 1 });
    const clear = screen.getByRole("button", { name: "Clear" });
    await waitFor(() => expect((clear as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(clear);
    await waitFor(() => expect((screen.getByRole("button", { name: "Save drawn signature" }) as HTMLButtonElement).disabled).toBe(true));
  });
});

describe("SignatureSettings", () => {
  it("saves only to the signed-in user's own signature", async () => {
    vi.spyOn(api, "get").mockResolvedValue({ present: false } as never);
    const put = vi.spyOn(api, "put").mockResolvedValue({} as never);
    render(<I18nProvider><SignatureSettings /></I18nProvider>);
    const canvas = await screen.findByLabelText("Signature pad");
    fireEvent.pointerDown(canvas, { pointerId: 1, clientX: 10, clientY: 10 });
    fireEvent.pointerMove(canvas, { pointerId: 1, clientX: 40, clientY: 40 });
    fireEvent.pointerUp(canvas, { pointerId: 1 });
    fireEvent.click(screen.getByRole("button", { name: "Save drawn signature" }));
    await waitFor(() => expect(put).toHaveBeenCalledWith("/me/signature", expect.any(FormData)));
    // There is no route that writes another user's signature, and none is used.
    expect(put.mock.calls.every(([path]) => path === "/me/signature")).toBe(true);
    const form = put.mock.calls[0][1] as FormData;
    expect(form.get("method")).toBe("drawn");
  });

  it("says a prescription prints without a signature until one is added", async () => {
    vi.spyOn(api, "get").mockResolvedValue({ present: false } as never);
    render(<I18nProvider><SignatureSettings /></I18nProvider>);
    expect(await screen.findByText(/blank signature line until you add one/)).toBeTruthy();
  });

  it("reports a stored signature and how it was made", async () => {
    vi.spyOn(api, "get").mockResolvedValue({ present: true, signature: { method: "uploaded", mediaType: "image/png", sizeBytes: 10, version: 1, updatedAt: "" } } as never);
    vi.spyOn(api, "blob").mockResolvedValue({ blob: new Blob(["x"]), filename: "" });
    render(<I18nProvider><SignatureSettings /></I18nProvider>);
    expect(await screen.findByText("On file")).toBeTruthy();
    expect(screen.getByText("Uploaded image")).toBeTruthy();
  });
});
