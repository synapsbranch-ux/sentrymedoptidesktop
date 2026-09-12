// @vitest-environment jsdom
import * as React from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { LabOrderSheet } from "./lab-document";
import type { LabOrderDocument } from "../types";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function sheet(overrides: Partial<LabOrderDocument> = {}): LabOrderDocument {
  return {
    order: {
      id: "l1", kind: "optical", orderNumber: "LAB-2026-000004", status: "ordered", notes: "Mount with the patient's own case.",
      orderedAt: "2026-09-12T09:00:00Z", expectedAt: "2026-09-19T09:00:00Z", deliveredAt: "", receivedByName: "",
      lensType: "Progressive", material: "High index 1.67", coatings: ["Anti-reflective"], tint: "Photogray",
      treatments: ["Photochromic"], measurements: { pd: "63", fittingHeight: "22" }, costMinor: 150000, salePriceMinor: 260000, version: 1,
    },
    patient: { name: "Marie Joseph", medicalRecordNumber: "MRN-0007", phone: "+509 3000 0000", dateOfBirth: "1984-03-02" },
    prescription: {
      id: "rx1", prescriptionNumber: "RX-2026-000012", type: "spectacle",
      // The consultation composer stores powers under lowercase prefixed keys.
      od: { odsphere: "-2.25", odcylinder: "-0.75", odaxis: "175" },
      os: { sphere: "-1.75", cylinder: "-0.50", axis: "10" },
      details: { pd: "63" }, notes: "", issuedAt: "2026-09-11T10:00:00Z", expiresAt: "", doctor: "Dr. Pierre",
    },
    encounter: { id: "e1", encounterNumber: "ENC-2026-000031", visitReason: "Blurred vision", status: "finalized", date: "2026-09-11T09:00:00Z", doctor: "Dr. Pierre" },
    supplier: { id: "s1", company: "Atelier Optique Caraïbe", contactPerson: "Jean", phone: "+509 4000 0000", email: "", address: "Delmas 31" },
    frame: { id: "f1", sku: "FR-201", name: "Aviator Gold", brand: "Rayline", model: "A2" },
    lens: { id: "n1", sku: "LN-161", name: "Progressive 1.61", brand: "", model: "" },
    invoice: {
      id: "i1", invoiceNumber: "INV-2026-000006", currency: "HTG", status: "paid", date: "2026-09-11T11:00:00Z",
      totalMinor: 400000, paidMinor: 400000, balanceMinor: 0,
      lines: [{ description: "Aviator Gold", quantity: 1, unitPriceMinor: 180000, lineTotalMinor: 180000 }],
    },
    tests: [], imageIds: [], statusHistory: [],
    ...overrides,
  };
}

const wrap = (document: LabOrderDocument) => render(<I18nProvider><LabOrderSheet document={document} /></I18nProvider>);

describe("the sheet that leaves with an optical order", () => {
  it("carries the powers the workshop grinds from, whichever way they were stored", () => {
    wrap(sheet());
    // Prefixed keys from a consultation and flat keys from a standalone
    // prescription have to read the same on the paper.
    expect(screen.getByText("-2.25")).toBeTruthy();
    expect(screen.getByText("175")).toBeTruthy();
    expect(screen.getByText("-1.75")).toBeTruthy();
    expect(screen.getByText("10")).toBeTruthy();
  });

  it("names the tint, the treatments and the frame the patient chose", () => {
    wrap(sheet());
    expect(screen.getByText("Photogray")).toBeTruthy();
    expect(screen.getByText("Photochromic")).toBeTruthy();
    // The frame is named in its own block and again on the sale line.
    expect(screen.getAllByText(/Aviator Gold/).length).toBeGreaterThan(1);
    expect(screen.getByText("Rayline")).toBeTruthy();
  });

  it("ties the order to the consultation, the sale and the glazing company", () => {
    wrap(sheet());
    expect(screen.getByText("ENC-2026-000031")).toBeTruthy();
    expect(screen.getByText("INV-2026-000006")).toBeTruthy();
    expect(screen.getByText("Atelier Optique Caraïbe")).toBeTruthy();
  });

  it("says so plainly when a remake carries no prescription, rather than printing an empty grid", () => {
    wrap(sheet({ prescription: null }));
    expect(screen.getByText(/No prescription is attached/)).toBeTruthy();
    expect(screen.queryByText("OD")).toBeNull();
  });
});

describe("the sheet that leaves with a laboratory request", () => {
  const request = sheet({
    order: { ...sheet().order, kind: "medical", orderNumber: "LAB-2026-000005" },
    prescription: null, frame: null, lens: null, invoice: null,
    tests: [
      { id: "t1", label: "Fasting glucose", code: "GLU", specimen: "Blood", notes: "Fasting since midnight" },
      { id: "t2", label: "HbA1c", code: "", specimen: "Blood", notes: "" },
    ],
  });

  it("lists the examinations asked for", () => {
    wrap(request);
    expect(screen.getByText("Fasting glucose")).toBeTruthy();
    expect(screen.getByText("HbA1c")).toBeTruthy();
    expect(screen.getByText("Fasting since midnight")).toBeTruthy();
  });

  it("leaves out the lens and frame blocks a laboratory has no use for", () => {
    wrap(request);
    expect(screen.queryByText("Lens specification")).toBeNull();
    expect(screen.queryByText("Fitting measurements")).toBeNull();
    expect(screen.getByText("Laboratory")).toBeTruthy();
  });
});
