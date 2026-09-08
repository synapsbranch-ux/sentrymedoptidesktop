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

test("chart drafts survive consultation tabs and require explicit conflict review", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();
  const lastName = `Chart-${Date.now()}`;
  const patient = await post<{id:string}>(page,"/patients",{firstName:"Test",lastName,tags:[]});
  const visit = await post<{id:string}>(page,"/encounters",{patientId:patient.id,visitReason:"Chart review"});
  await page.goto("/clinical");
  await page.getByRole("button",{name:new RegExp(lastName)}).click();
  const dialog=page.getByRole("dialog");
  await dialog.getByRole("button",{name:"Charts",exact:true}).click();
  await dialog.getByRole("button",{name:"Fundus",exact:true}).click();
  await dialog.getByRole("textbox",{name:"Right fundus (OD) notes",exact:true}).fill("My retinal observation");
  await dialog.getByLabel("Examination status").first().selectOption("findings");
  await dialog.getByRole("button",{name:"Anterior segment",exact:true}).click();
  await expect(dialog.getByRole("textbox",{name:"Right eye (OD) notes",exact:true})).toHaveValue("");
  await dialog.getByRole("button",{name:"Fundus",exact:true}).click();
  await expect(dialog.getByRole("textbox",{name:"Right fundus (OD) notes",exact:true})).toHaveValue("My retinal observation");
  await dialog.getByRole("button",{name:"Doctor exam",exact:true}).click();
  await dialog.getByRole("button",{name:"Charts",exact:true}).click();
  await dialog.getByRole("button",{name:"Fundus",exact:true}).click();
  await expect(dialog.getByRole("textbox",{name:"Right fundus (OD) notes",exact:true})).toHaveValue("My retinal observation");
  page.once("dialog",confirmation=>confirmation.dismiss());
  await dialog.getByRole("button",{name:"Close",exact:true}).click();
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button",{name:"Save chart",exact:true}).first().click();
  await expect(dialog.getByRole("button",{name:"Save chart",exact:true}).first()).toBeDisabled();
  const path=`/encounters/${visit.id}/eye-diagrams/fundus/OD`;
  await put(page,path,{version:1,annotations:[],notes:"Other device observation",examStatus:"findings"});
  await dialog.getByRole("textbox",{name:"Right fundus (OD) notes",exact:true}).fill("Reviewed local observation");
  await dialog.getByRole("button",{name:"Save chart",exact:true}).first().click();
  await expect(dialog.getByText("Another version exists. Compare before saving.")).toBeVisible();
  await expect(dialog.getByText("Other device observation",{exact:true})).toBeVisible();
  await expect(dialog.getByRole("textbox",{name:"Right fundus (OD) notes",exact:true})).toHaveValue("Reviewed local observation");
  page.once("dialog",confirmation=>confirmation.accept());
  await dialog.getByRole("button",{name:"I reviewed it — keep my draft for the next save"}).click();
  await dialog.getByRole("button",{name:"Save chart",exact:true}).first().click();
  await expect(dialog.getByRole("button",{name:"Save chart",exact:true}).first()).toBeDisabled();
  await dialog.getByRole("button",{name:"Chart history and finding follow-up"}).click();
  await expect(dialog.locator("summary")).toHaveCount(3);
  const chartRegion=dialog.getByRole("region",{name:"Clinical charts"});
  expect(await chartRegion.evaluate(element=>element.scrollWidth<=element.clientWidth+1)).toBe(true);
  const history=await json<{items:Array<{notes:string;version:number}>}>(await page.request.get(`/api/v1${path}/history`));
  expect(history.items.map(item=>item.notes)).toEqual(["Reviewed local observation","Other device observation","My retinal observation"]);
  await page.screenshot({path:"test-results/chart-history-mobile.png",fullPage:true});
});

test("mobile clinic flow persists from arrival through optical delivery", async ({ page }) => {
  const suffix = Date.now().toString();
  const patientName = `Marie Joseph ${suffix}`;

  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();

  await page.getByRole("navigation", { name: "Primary navigation" }).getByRole("link", { name: "Patients" }).click();
  await page.getByRole("button", { name: "New patient" }).click();
  await page.getByLabel("First name").fill("Marie");
  await page.getByLabel("Last name").fill(`Joseph ${suffix}`);
  await page.getByRole("textbox", { name: "Phone", exact: true }).fill(`509${suffix.slice(-8)}`);
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
  await page.getByRole("searchbox", { name: "Search patients" }).fill(suffix);
  await page.getByRole("button", { name: new RegExp(patientName) }).click();
  await expect(page.getByRole("heading", { name: patientName })).toBeVisible();
  const patientRecord = page.getByRole("complementary", { name: "Patient record" });
  for (const label of ["Appointment", "Consultation", "Prescription", "Invoice", "Payment", "Lab order"]) {
    await expect(patientRecord.getByText(new RegExp(`^${label}`)).first()).toBeVisible();
  }
});

test("mobile navigation, dialogs and public display remain operable", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox", { name: "Password", exact: true }).fill("Doctor-Development-Only-2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: /Good day/ })).toBeVisible();

  await page.getByRole("button", { name: "Open navigation" }).click();
  const drawerNavigation = page.getByRole("navigation", { name: "Clinic modules" }).last();
  await expect(drawerNavigation).toBeVisible();
  await drawerNavigation.getByRole("link", { name: "System" }).scrollIntoViewIfNeeded();
  await expect(drawerNavigation.getByRole("link", { name: "System" })).toBeVisible();
  await page.getByRole("button", { name: "Close navigation" }).last().click();

  await page.getByRole("navigation", { name: "Primary navigation" }).getByRole("link", { name: "Patients" }).click();
  await page.getByRole("button", { name: "New patient" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const bounds = await dialog.boundingBox();
  const viewport = page.viewportSize();
  expect(bounds).not.toBeNull();
  expect(viewport).not.toBeNull();
  expect(bounds!.y).toBeGreaterThanOrEqual(0);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport!.height + 1);
  await expect(page.getByRole("button", { name: "Close" })).toBeVisible();
  await page.getByRole("button", { name: "Close" }).click();

  const settings = await json<{ settings: Record<string, unknown>; versions: Record<string, number> }>(await page.request.get("/api/v1/settings"));
  await put(page, "/settings/public_display", { value: { enabled: true, privacyMode: "ticket_only", showAppointments: true, announcement: "Please watch for your queue number." }, version: settings.versions.public_display });
  await page.goto("/display");
  await expect(page.getByRole("heading", { name: "Current progress" })).toBeVisible();
  await expect(page.getByText("Patient flow · Live clinic display")).toBeVisible();
});
