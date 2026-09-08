import { expect, test } from "@playwright/test";

// B2 acceptance: walk a test patient through arrival, waiting, consultation and
// checkout, and count the clicks. More than five means it is not simple enough.
test("a walk-in reaches checkout in five clicks or fewer", async ({ page }) => {
  const suffix = Date.now().toString().slice(-8);
  const lastName = `Walkin${suffix}`;

  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();
  await page.getByRole("navigation", { name: "Primary navigation" }).getByRole("link", { name: "Queue" }).click();

  let clicks = 0;
  const click = async (locator: ReturnType<typeof page.getByRole>) => { clicks += 1; await locator.click(); };

  // 1. Open the walk-in form.
  await click(page.getByRole("button", { name: "Walk-in" }).first());
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("First name").fill("Marie");
  await dialog.getByLabel("Last name").fill(lastName);
  await dialog.getByLabel("Phone").fill(`509${suffix}`);
  await dialog.getByLabel("Reason for visit").fill("Sudden blurred vision");

  // 2. Register and queue in one step.
  await click(dialog.getByRole("button", { name: "Add to waiting room" }));
  const row = page.getByRole("listitem").filter({ hasText: lastName });
  await expect(row).toBeVisible();
  await expect(row.getByText("Sudden blurred vision")).toBeVisible();
  await expect(row.getByText("Walk-in")).toBeVisible();

  // 3, 4, 5. Waiting to in consultation to checkout to completed, no confirmations.
  await click(row.getByRole("button", { name: "In consultation" }));
  await expect(row.getByText("In consultation")).toBeVisible();
  await click(row.getByRole("button", { name: "Checkout" }));
  await expect(row.getByText("Checkout")).toBeVisible();
  await click(row.getByRole("button", { name: "Completed" }));

  await expect(page.getByRole("listitem").filter({ hasText: lastName })).toHaveCount(0);
  expect(clicks, `arrival through checkout took ${clicks} clicks`).toBeLessThanOrEqual(5);
});
