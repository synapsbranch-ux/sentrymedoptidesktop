import { expect, test } from "@playwright/test";

// D4 acceptance: select the patient, create the prescription, sign, print — all
// without a consultation, and the result stays in the patient's history flagged
// as not originating from one.
test("a prescription can be issued outside a consultation and is flagged as such", async ({ page }) => {
  const suffix = Date.now().toString().slice(-8);
  const lastName = `Standalone${suffix}`;

  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();

  const patient = await page.request.post("/api/v1/patients", { data: { firstName: "Rose", lastName, tags: [] } });
  expect(patient.ok(), await patient.text()).toBeTruthy();
  const patientId = (await patient.json()).id as string;

  await page.goto("/prescriptions");
  await page.getByRole("button", { name: "New prescription" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("combobox", { name: "Patient" }).fill(lastName);
  // Scoped to the search listbox: a native <select> also exposes options.
  await dialog.getByRole("listbox").getByRole("option").first().click();
  await dialog.getByLabel("Prescription type").selectOption("medication");
  await dialog.getByLabel("Medication", { exact: true }).fill("Timolol 0.5%");
  await dialog.getByLabel("Dosage", { exact: true }).fill("1 drop twice daily");
  await dialog.getByRole("button", { name: "Issue and open for printing" }).click();

  // The issued document opens ready to print.
  await expect(page.getByRole("button", { name: "Print / PDF" })).toBeVisible();
  await expect(page.getByText("Issued outside a consultation.")).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Close", exact: true }).click();

  // It is in the register, flagged, and attached to the patient file.
  // The register renders a table on wide screens and cards on narrow ones;
  // assert on whichever is actually shown.
  await expect(page.getByText("Outside a consultation").locator("visible=true").first()).toBeVisible();
  const history = await page.request.get(`/api/v1/prescriptions?patientId=${patientId}`);
  const items = (await history.json()).items as Array<{ standalone: boolean; encounterId: string }>;
  expect(items).toHaveLength(1);
  expect(items[0].standalone).toBe(true);
  expect(items[0].encounterId).toBe("");
});
