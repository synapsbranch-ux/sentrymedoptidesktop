import { expect, test } from "@playwright/test";

/**
 * D3 acceptance, driven through the real browser fetch/FormData path.
 *
 * Regression: api.put() unconditionally JSON.stringify()'d its body. The
 * signature save flow is the only PUT call site in the app that sends
 * FormData, so no other screen exercised it — a doctor drawing an ordinary
 * signature on the pad was told it exceeded 2 MB, which was never true; the
 * request never reached the server as a file upload at all. Every other test
 * for this flow (Go handler tests built with Go's own multipart writer,
 * component tests that mock api.put directly) was structurally unable to
 * catch it, because none of them go through the browser's real fetch(). This
 * one does: draw on the pad exactly as a doctor would, in a real browser.
 */
test("a doctor can draw and save their own signature", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();

  await page.goto("/system");
  await page.getByRole("button", { name: "My signature" }).click();
  await expect(page.getByText("No signature yet.")).toBeVisible();

  const pad = page.getByLabel("Signature pad");
  const box = await pad.boundingBox();
  if (!box) throw new Error("signature pad has no bounding box");

  // Draw a simple stroke across the pad. Dispatched as real PointerEvents
  // (what the component's onPointerDown/Move/Up handlers listen for) rather
  // than through page.mouse, because this project emulates a touch device and
  // synthetic mouse events do not reliably land as pointer input under that
  // emulation — a finger on an actual tablet does, which is the case this
  // component exists to support.
  const points = [
    [box.x + box.width * 0.2, box.y + box.height * 0.5],
    [box.x + box.width * 0.4, box.y + box.height * 0.35],
    [box.x + box.width * 0.6, box.y + box.height * 0.4],
    [box.x + box.width * 0.8, box.y + box.height * 0.7],
  ];
  await pad.dispatchEvent("pointerdown", { pointerId: 1, pointerType: "touch", isPrimary: true, clientX: points[0][0], clientY: points[0][1], button: 0, buttons: 1 });
  for (const [x, y] of points.slice(1)) {
    await pad.dispatchEvent("pointermove", { pointerId: 1, pointerType: "touch", isPrimary: true, clientX: x, clientY: y, buttons: 1 });
  }
  await pad.dispatchEvent("pointerup", { pointerId: 1, pointerType: "touch", isPrimary: true, clientX: points.at(-1)![0], clientY: points.at(-1)![1], button: 0, buttons: 0 });

  await page.getByRole("button", { name: "Save drawn signature" }).click();

  // The old bug surfaced exactly this message for an ordinary small signature.
  await expect(page.getByText(/2 MB/)).toHaveCount(0);
  await expect(page.getByText("Signature saved")).toBeVisible();
  await expect(page.getByText("On file")).toBeVisible();
  await expect(page.getByText("Drawn on screen")).toBeVisible();

  // Persisted server-side, not just in local component state.
  await page.reload();
  await page.getByRole("button", { name: "My signature" }).click();
  await expect(page.getByText("On file")).toBeVisible();
  await expect(page.getByText("Drawn on screen")).toBeVisible();
});
