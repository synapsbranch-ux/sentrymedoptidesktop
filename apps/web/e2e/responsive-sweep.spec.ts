import { expect, test, type Page } from "@playwright/test";

/**
 * Section 6 of the stabilisation brief: responsive layout on tablet and phone —
 * no screen may scroll sideways, no table may push the page wider than the
 * viewport, and a modal's actions must stay reachable on a small screen.
 */

const routes = ["/", "/patients", "/schedule", "/clinical", "/prescriptions", "/documents", "/inventory", "/pos", "/billing", "/insurance", "/lab", "/system"];
const viewports = [
  { name: "phone", width: 360, height: 720 },
  { name: "tablet", width: 768, height: 1024 },
];

async function signIn(page: Page) {
  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();
}

test("no screen scrolls sideways on a phone or a tablet", async ({ page }) => {
  await signIn(page);
  // Data present, so wide tables are actually rendered.
  const suffix = Date.now().toString().slice(-8);
  const patient = await page.request.post("/api/v1/patients", {
    data: { firstName: "Responsive", lastName: `Sweep${suffix}`, phone: `509${suffix}`, email: "a.very.long.clinic.address@example-clinic-domain.test", dateOfBirth: "1970-01-01", tags: ["glaucoma", "contact-lens", "follow-up"] },
  });
  expect(patient.ok(), await patient.text()).toBeTruthy();
  const patientId = (await patient.json()).id as string;
  await page.request.post("/api/v1/encounters", { data: { patientId, visitReason: "A deliberately long visit reason to push the table wide" } });
  await page.request.post("/api/v1/queue/walk-in", { data: { patientId, reason: "A deliberately long reason for the visit that could overflow" } });

  for (const viewport of viewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    for (const route of routes) {
      await page.goto(route);
      await page.waitForTimeout(300);
      const overflow = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(overflow.scrollWidth, `${route} at ${viewport.name} scrolls sideways`).toBeLessThanOrEqual(overflow.clientWidth + 1);
    }
  }
});

test("a modal's actions stay inside a phone screen", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 720 });
  await signIn(page);

  for (const [route, trigger] of [["/patients", "New patient"], ["/schedule", "New appointment"], ["/prescriptions", "New prescription"]] as const) {
    await page.goto(route);
    await page.getByRole("button", { name: trigger }).first().click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const bounds = await dialog.boundingBox();
    expect(bounds, `${trigger} dialog has no box`).not.toBeNull();
    expect(bounds!.x, `${trigger} dialog starts off-screen`).toBeGreaterThanOrEqual(-1);
    expect(bounds!.x + bounds!.width, `${trigger} dialog is cut off on the right`).toBeLessThanOrEqual(361);
    expect(bounds!.y + bounds!.height, `${trigger} dialog is taller than the screen`).toBeLessThanOrEqual(721);
    // The sticky footer keeps the submit control reachable without a hunt.
    const submit = dialog.getByRole("button", { name: /Create|Save|Issue|Add/ }).last();
    await expect(submit).toBeInViewport();
    await dialog.getByRole("button", { name: "Close", exact: true }).click();
  }
});
