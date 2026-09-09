import { expect, test } from "@playwright/test";

test("patient gender saves and appointment creation searches existing patients", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();

  const email = `workflow-${Date.now()}@example.test`;
  const created = await page.request.post("/api/v1/patients", { data: {
    firstName: "Workflow", lastName: `Patient-${Date.now()}`, sex: "male",
    email, employer: "Preserved employer", tags: ["follow-up"],
  } });
  expect(created.status()).toBe(201);
  const patient = await created.json() as { id: string };
  await page.goto(`/patients?id=${patient.id}`);
  await page.getByRole("complementary", { name: "Patient record" }).getByRole("button", { name: "Edit", exact: true }).click();
  const edit = page.getByRole("dialog", { name: "Edit patient record" });
  await edit.getByLabel("Sex", { exact: true }).selectOption("female");
  const saved = page.waitForResponse((res) => res.url().endsWith(`/api/v1/patients/${patient.id}`) && res.request().method() === "PUT");
  await edit.getByRole("button", { name: "Save changes" }).click();
  expect((await saved).status()).toBe(200);
  await expect(edit).toBeHidden();
  const persisted = await (await page.request.get(`/api/v1/patients/${patient.id}`)).json();
  expect(persisted).toMatchObject({ sex: "female", employer: "Preserved employer", tags: ["follow-up"] });

  await page.goto("/schedule");
  await page.getByRole("button", { name: "New appointment" }).click();
  const appointment = page.getByRole("dialog", { name: "Create appointment" });
  await appointment.getByRole("combobox", { name: "Patient" }).fill(email);
  const option = appointment.getByRole("option").filter({ hasText: email });
  await expect(option).toHaveCount(1);
  await option.click();
  await appointment.getByLabel("Start — date", { exact: true }).fill("2037-10-11");
  const scheduled = page.waitForResponse((res) => res.url().endsWith("/api/v1/appointments") && res.request().method() === "POST");
  await appointment.getByRole("button", { name: "Create appointment", exact: true }).click();
  const response = await scheduled;
  expect(response.status()).toBe(201);
  expect(response.request().postDataJSON()).toMatchObject({ patientId: patient.id });
  await expect(appointment).toBeHidden();
});
