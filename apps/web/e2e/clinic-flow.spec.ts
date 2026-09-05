import { expect, test, type APIResponse, type Page } from "@playwright/test";

async function json<T>(response: APIResponse): Promise<T> {
  expect(response.ok(), `${response.status()} ${response.url()}: ${await response.text()}`).toBeTruthy();
  return response.json() as Promise<T>;
}

async function post<T>(page: Page, path: string, data: unknown): Promise<T> {
  return json<T>(await page.request.post(`/api/v1${path}`, { data }));
}

async function patch<T>(page: Page, path: string, data: unknown): Promise<T> {
  return json<T>(await page.request.patch(`/api/v1${path}`, { data }));
}

async function put<T>(page: Page, path: string, data: unknown): Promise<T> {
  return json<T>(await page.request.put(`/api/v1${path}`, { data }));
}

test("mobile clinic flow persists from arrival through optical delivery", async ({ page }) => {
  const suffix = Date.now().toString();
  const patientName = `Marie Joseph ${suffix}`;

  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByLabel("Password").fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();

  await page.getByRole("link", { name: "Patients" }).click();
  await page.getByRole("button", { name: "New patient" }).click();
  await page.getByLabel("First name").fill("Marie");
  await page.getByLabel("Last name").fill(`Joseph ${suffix}`);
  await page.getByLabel("Phone").fill(`509${suffix.slice(-8)}`);
  await page.getByRole("button", { name: "Create patient" }).click();
  await expect(page.getByRole("heading", { name: patientName })).toBeVisible();

  const patients = await json<{ items: Array<{ id: string }> }>(await page.request.get(`/api/v1/patients?q=${encodeURIComponent(suffix)}`));
  const patientId = patients.items[0].id;
  const startsAt = new Date(Date.now() + 3_600_000).toISOString();
  const appointment = await post<{ id: string }>(page, "/appointments", { patientId, practitionerId: "", startsAt, durationMinutes: 30, type: "eye_exam", reason: "Blurred distance vision", notes: "E2E acceptance flow" });
  await post(page, "/queue/check-in", { patientId, appointmentId: appointment.id, assignedDoctorId: "", priority: 0 });

  const encounter = await post<{ id: string }>(page, "/encounters", { patientId, appointmentId: appointment.id, visitReason: "Comprehensive eye examination", chiefComplaint: "Blurred distance vision", hpi: "", assessment: "", treatmentPlan: "", followUp: "" });
  await put(page, `/encounters/${encounter.id}/pretest`, { chiefComplaint: "Blurred distance vision", vitals: {}, visualAcuity: { odDistanceVA: "20/40", osDistanceVA: "20/30" }, autorefraction: { odSphere: "-1.50", osSphere: "-1.25" }, keratometry: {}, iop: { odIOP: "17", osIOP: "16", method: "NCT" }, pupils: "PERRLA", eom: "Full", coverTest: "Ortho", confrontationFields: "Full", colorVision: "Normal", stereopsis: "", pachymetry: {}, lensometry: {}, complete: true, version: 1 });
  await put(page, `/encounters/${encounter.id}`, { patientId, appointmentId: appointment.id, visitReason: "Comprehensive eye examination", chiefComplaint: "Blurred distance vision", hpi: "Progressive blur", assessment: "Myopia OU", treatmentPlan: "Spectacle correction", followUp: "12 months", version: 1 });
  await put(page, `/encounters/${encounter.id}/sections/subjective_refraction`, { data: { odSphere: "-1.50", odCylinder: "-0.50", odAxis: "090", osSphere: "-1.25", osCylinder: "-0.25", osAxis: "080", pd: "62" }, version: 0 });
  await post(page, `/encounters/${encounter.id}/diagnoses`, { diagnosis: "Myopia, bilateral", code: "H52.13", laterality: "OU", notes: "", primary: true });
  const prescription = await post<{ id: string }>(page, "/prescriptions", { patientId, encounterId: encounter.id, type: "spectacle", od: { sphere: "-1.50", cylinder: "-0.50", axis: "090" }, os: { sphere: "-1.25", cylinder: "-0.25", axis: "080" }, details: { pd: "62", lensRecommendation: "Anti-reflective" }, notes: "Distance wear", expiresAt: "" });
  await post(page, `/encounters/${encounter.id}/finalize`, { version: 2 });

  const frame = await post<{ id: string }>(page, "/inventory", { sku: `FRAME-${suffix}`, barcode: "", category: "frame", name: "Acetate optical frame", brand: "Sentry", model: "Classic", attributes: { color: "black", eyeSize: "52", bridge: "18", temple: "140" }, supplierId: "", costMinor: 5000, salePriceMinor: 10000, currency: "HTG", quantity: 2, reorderLevel: 1, trackStock: true });
  const sale = await post<{ invoiceId: string; balanceMinor: number }>(page, "/pos/checkout", { invoice: { patientId, currency: "HTG", exchangeRate: "1", discountMinor: 0, taxMinor: 0, dueAt: "", notes: "Optical sale", items: [{ inventoryItemId: frame.id, description: "Acetate optical frame and lenses", quantity: 1, unitPriceMinor: 15000, discountMinor: 0, taxMinor: 0 }] }, payment: { paymentMethodId: "pm_cash", registerSessionId: "", amountMinor: 5000, currency: "HTG", exchangeRate: "1", reference: "", notes: "Deposit" } });
  expect(sale.balanceMinor).toBe(10000);

  const order = await post<{ id: string; version: number }>(page, "/lab-orders", { patientId, prescriptionId: prescription.id, invoiceId: sale.invoiceId, supplierId: "", frameItemId: frame.id, lensItemId: "", lensType: "single_vision", material: "1.56", coatings: ["anti_reflective"], tint: "", treatments: [], measurements: { pd: "62", fittingHeight: "20" }, notes: "Acceptance flow", expectedAt: new Date(Date.now() + 86_400_000).toISOString(), costMinor: 6000, salePriceMinor: 15000 });
  let version = order.version;
  for (const status of ["ordered", "at_lab", "received", "edging_mounting", "quality_control"]) {
    const changed = await patch<{ version: number }>(page, `/lab-orders/${order.id}/status`, { status, notes: "", receivedByName: "", version });
    version = changed.version;
  }
  await post(page, `/lab-orders/${order.id}/quality-control`, { prescriptionVerified: true, powerVerified: true, axisVerified: true, frameCondition: true, lensCondition: true, fittingVerified: true, finalCleaning: true, notes: "Verified" });
  version = (await patch<{ version: number }>(page, `/lab-orders/${order.id}/status`, { status: "ready", notes: "", receivedByName: "", version })).version;
  await patch(page, `/lab-orders/${order.id}/status`, { status: "delivered", notes: "Fitted", receivedByName: "Marie Joseph", version });

  await page.reload();
  await page.getByPlaceholder("Name, medical record number, phone, email…").fill(suffix);
  await page.getByRole("button", { name: new RegExp(patientName) }).click();
  await expect(page.getByRole("heading", { name: patientName })).toBeVisible();
  for (const label of ["Appointment", "Consultation", "Prescription", "Invoice", "Payment", "Lab order"]) {
    await expect(page.getByText(new RegExp(`^${label}`)).first()).toBeVisible();
  }
});
