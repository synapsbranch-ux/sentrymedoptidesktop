import { expect, test, type ConsoleMessage, type Page } from "@playwright/test";

/**
 * Section 6 of the stabilisation brief: no screen may produce an uncaught error
 * or a React/framework warning. This walks every route the doctor role can
 * reach, with data present, and fails with the offending message.
 */

const routes = [
  "/", "/patients", "/schedule", "/clinical", "/prescriptions", "/documents",
  "/inventory", "/stock-takes", "/purchasing", "/pos", "/billing", "/insurance", "/lab",
  "/finance", "/reports", "/system",
];

// Failures that come from the test environment rather than the application.
function isEnvironmentNoise(text: string) {
  return text.includes("Failed to load resource") // favicon/manifest in the dev build
    || text.includes("Download the React DevTools");
}

function collect(page: Page) {
  const problems: string[] = [];
  page.on("console", (message: ConsoleMessage) => {
    if (message.type() !== "error" && message.type() !== "warning") return;
    const text = message.text();
    if (isEnvironmentNoise(text)) return;
    problems.push(`console.${message.type()}: ${text}`);
  });
  page.on("pageerror", (error) => problems.push(`uncaught: ${error.message}`));
  return problems;
}

test("no screen produces an uncaught error or a framework warning", async ({ page }) => {
  const problems = collect(page);

  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();

  // Realistic data, so empty states are not the only thing exercised.
  const suffix = Date.now().toString().slice(-8);
  const patient = await page.request.post("/api/v1/patients", {
    data: { firstName: "Console", lastName: `Sweep${suffix}`, phone: `509${suffix}`, dateOfBirth: "1990-05-04", tags: [] },
  });
  expect(patient.ok(), await patient.text()).toBeTruthy();
  const patientId = (await patient.json()).id as string;
  await page.request.post("/api/v1/encounters", { data: { patientId, visitReason: "Console sweep" } });
  await page.request.post("/api/v1/queue/walk-in", { data: { patientId, reason: "Console sweep" } });
  // Far enough ahead that this sweep cannot collide with another spec's booking:
  // the server rejects a second appointment in the same slot with 409.
  const slot = new Date(Date.now() + 60 * 86_400_000);
  slot.setUTCHours(3, 0, 0, 0);
  await page.request.post("/api/v1/appointments", {
    data: { patientId, practitionerId: "", startsAt: slot.toISOString(), durationMinutes: 30, type: "eye_exam", reason: "Console sweep", notes: "" },
  });

  for (const route of routes) {
    await page.goto(route);
    // The shell renders every page inside an error boundary; a boundary that has
    // tripped is a failure even when nothing reached the console.
    await expect(page.getByText("This section could not be displayed")).toHaveCount(0);
    await page.waitForTimeout(400);
    expect(problems, `${route} produced: ${problems.join(" | ")}`).toEqual([]);
  }

  // The waiting-room screen and a consultation, which are not their own routes.
  await page.goto("/schedule");
  await page.getByRole("button", { name: "Waiting room" }).click();
  await expect(page.getByText("Waiting room", { exact: false }).first()).toBeVisible();
  await page.waitForTimeout(400);
  expect(problems, `waiting room produced: ${problems.join(" | ")}`).toEqual([]);

  await page.goto("/clinical");
  await page.getByRole("button", { name: new RegExp(`Sweep${suffix}`) }).click();
  const dialog = page.getByRole("dialog");
  for (const tab of ["Pre-test", "Doctor exam", "Evolution", "Charts", "Recording"]) {
    await dialog.getByRole("button", { name: tab, exact: true }).click();
    await page.waitForTimeout(500);
    expect(problems, `consultation tab ${tab} produced: ${problems.join(" | ")}`).toEqual([]);
  }
});
