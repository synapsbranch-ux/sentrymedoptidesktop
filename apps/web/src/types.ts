export type Role = "doctor" | "nurse";
export interface User { id: string; username: string; email?: string; displayName: string; role: Role }
export interface APIErrorBody { code: string; message: string; details?: Record<string, unknown> }
export interface Patient {
  id: string; medicalRecordNumber: string; firstName: string; middleName: string; lastName: string; preferredName: string;
  sex: string; dateOfBirth: string; phone: string; alternatePhone: string; email: string; address: string; city: string;
  occupation: string; employer: string; preferredLanguage: string; communicationPreference: string; referralSource: string;
  referringProvider: string; civilStatus: string; religion: string; religionOther: string;
  notes: string; tags: string[]; version: number; createdAt: string; updatedAt: string; updatedBy: string;
}
export interface Appointment { id: string; patientId: string; medicalRecordNumber: string; patientName: string; practitionerId: string; practitionerName: string; startsAt: string; durationMinutes: number; type: string; reason: string; notes: string; status: string; version: number }
export interface QueueEntry { id: string; patientId: string; medicalRecordNumber: string; patientName: string; phone: string; visitReason: string; appointmentId: string; encounterId: string; assignedDoctorId: string; assignedDoctorName: string; arrivedAt: string; stage: string; stageEnteredAt: string; estimatedWaitMinutes: number | null; waitEstimateSamples: number; priority: number; source: string; version: number; updatedAt: string }
export interface EncounterSummary { id: string; encounterNumber: string; patientId: string; medicalRecordNumber: string; patientName: string; doctorId: string; doctorName: string; visitReason: string; chiefComplaint: string; assessment: string; status: "draft" | "finalized"; finalizedAt: string; version: number; createdAt: string; updatedAt: string }
export interface InventoryItem { id: string; sku: string; barcode: string; category: string; name: string; brand: string; model: string; attributes: Record<string, unknown>; supplierId: string; costMinor: number; salePriceMinor: number; currency: string; quantity: number; reorderLevel: number; trackStock: boolean; procedureCode: string; durationMinutes: number; bookable: boolean; lowStock: boolean; version: number; updatedAt: string }
export interface Supplier { id: string; company: string; contactPerson: string; phone: string; email: string; address: string; notes: string; productCount: number; version: number; updatedAt: string }
export interface PurchaseOrderSummary { id: string; orderNumber: string; supplierId: string; supplierName: string; status: "draft" | "sent" | "partial" | "received" | "cancelled"; currency: string; expectedAt: string; notes: string; version: number; createdAt: string; updatedAt: string; lineCount: number; quantityOrdered: number; quantityReceived: number; totalMinor: number }
export interface PurchaseOrderLine { id: string; inventoryItemId: string; sku: string; name: string; quantityOrdered: number; quantityReceived: number; remainingQuantity: number; unitCostMinor: number }
export interface PurchaseOrder extends PurchaseOrderSummary { items: PurchaseOrderLine[] }
export interface StockTakeSummary { id: string; stockTakeNumber: string; status: "in_progress" | "completed" | "cancelled"; category: string; notes: string; version: number; startedAt: string; completedAt: string; createdBy: string; itemCount: number; countedCount: number; differenceCount: number }
export interface StockTakeLine { id: string; inventoryItemId: string; sku: string; name: string; category: string; expectedQuantity: number; countedQuantity: number | null; difference: number | null; reason: string; version: number; countedAt: string; countedBy: string }
export interface StockTake extends Omit<StockTakeSummary, "itemCount" | "countedCount" | "differenceCount"> { items: StockTakeLine[] }
export interface Invoice { id: string; invoiceNumber: string; patientId: string; patientName: string; status: string; currency: string; exchangeRate: string; subtotalMinor: number; discountMinor: number; taxMinor: number; totalMinor: number; paidMinor: number; balanceMinor: number; dueAt: string; version: number; createdAt: string; updatedAt: string }

export interface InvoiceInsuranceClaimSummary { id: string; payerName: string; status: string; payerPortionMinor: number; paidMinor: number }
export interface InvoiceInsurance { insuranceClaims: InvoiceInsuranceClaimSummary[]; insuranceExpectedMinor: number; insuranceReceivedMinor: number; patientResponsibilityMinor: number }

export type VerificationStatus = "not_verified" | "pending_verification" | "verified" | "expired" | "rejected";

export interface Payer {
  id: string; name: string; contactName: string; phone: string; email: string; address: string;
  website: string; notes: string; acceptedCoverage: string; billingInfo: string; claimInstructions: string;
  defaultCoveragePercent: number; active: boolean; version: number; createdAt: string; updatedAt: string;
}

export interface PatientPolicy {
  id: string; patientId: string; payerId: string; payerName: string; memberNumber: string; policyNumber: string;
  groupNumber: string; subscriberName: string; relationshipToSubscriber: string; authorization: string;
  coveragePercent: number; effectiveDate: string; expirationDate: string; coverageNotes: string; notes: string;
  verificationStatus: VerificationStatus; verifiedBy: string; verifiedAt: string; verificationReference: string;
  verificationContact: string; verificationNotes: string;
  isPrimary: boolean; isActive: boolean; expired: boolean; version: number; createdAt: string; updatedAt: string;
}

export interface InsuranceCard { id: string; side: "front" | "back"; mediaType: string; sizeBytes: number; uploadedAt: string; uploadedBy: string }

export type InsuranceDocumentStatus = "required" | "requested" | "received" | "completed" | "submitted" | "rejected" | "expired";
export interface InsuranceDocument {
  id: string; payerId: string; patientId: string; claimId: string; patientInsuranceId: string;
  documentType: string; status: InsuranceDocumentStatus; hasFile: boolean; displayName: string; mediaType: string; sizeBytes: number;
  existingDocumentId: string; requestedBy: string; requestedAt: string; receivedAt: string; expirationDate: string; notes: string;
  version: number; createdAt: string; updatedAt: string;
}

export interface InsuranceClaimDetail {
  id: string; medicalRecordNumber: string; patientName: string; patientId: string; payerName: string; payerId: string;
  invoiceNumber: string; invoiceId: string; invoiceItemId: string; encounterId: string; appointmentId: string; prescriptionId: string;
  authorization: string; memberNumber: string; policyNumber: string; currency: string; exchangeRate: string;
  claimAmountMinor: number; patientPortionMinor: number; payerPortionMinor: number; approvedAmountMinor: number | null; partiallyApproved: boolean;
  paidMinor: number; outstandingMinor: number; status: string; submittedAt: string; responseDate: string; notes: string;
  version: number; createdAt: string; updatedAt: string; documents: InsuranceDocument[];
}
/** An order leaving the clinic: glasses to a glazing company, or exams to a laboratory. */
export type LabOrderKind = "optical" | "medical";
export type LabOrderStatus = "draft" | "ordered" | "at_lab" | "received" | "edging_mounting" | "quality_control" | "ready" | "delivered" | "cancelled";
export interface LabOrder { id: string; kind: LabOrderKind; orderNumber: string; patientId: string; medicalRecordNumber: string; patientName: string; prescriptionId: string; invoiceId: string; supplierId: string; supplierName: string; frameItemId: string; frameName: string; lensItemId: string; lensName: string; lensType: string; material: string; coatings: string[]; tint: string; treatments: string[]; measurements: Record<string, unknown>; notes: string; orderedAt: string; expectedAt: string; costMinor: number; salePriceMinor: number; status: LabOrderStatus; deliveredAt: string; version: number; createdAt: string; updatedAt: string }

export interface LabOrderTest { id?: string; label: string; code: string; specimen: string; notes: string }
/** Everything one order's printed document needs, assembled by the server. */
export interface LabOrderDocument {
  order: { id: string; kind: LabOrderKind; orderNumber: string; status: LabOrderStatus; notes: string; orderedAt: string; expectedAt: string; deliveredAt: string; receivedByName: string; lensType: string; material: string; coatings: string[]; tint: string; treatments: string[]; measurements: Record<string, string>; costMinor: number; salePriceMinor: number; version: number };
  patient: { name: string; medicalRecordNumber: string; phone: string; dateOfBirth: string };
  prescription: { id: string; prescriptionNumber: string; type: string; od: Record<string, string>; os: Record<string, string>; details: Record<string, string>; notes: string; issuedAt: string; expiresAt: string; doctor: string } | null;
  encounter: { id: string; encounterNumber: string; visitReason: string; status: string; date: string; doctor: string } | null;
  supplier: { id: string; company: string; contactPerson: string; phone: string; email: string; address: string } | null;
  frame: { id: string; sku: string; name: string; brand: string; model: string } | null;
  lens: { id: string; sku: string; name: string; brand: string; model: string } | null;
  invoice: { id: string; invoiceNumber: string; currency: string; status: string; date: string; totalMinor: number; paidMinor: number; balanceMinor: number; lines: { description: string; quantity: number; unitPriceMinor: number; lineTotalMinor: number }[] } | null;
  tests: LabOrderTest[];
  imageIds: string[];
  statusHistory: { fromStatus: string; toStatus: string; notes: string; changedAt: string; changedBy: string }[];
}
