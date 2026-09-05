export type Role = "doctor" | "nurse";
export interface User { id: string; username: string; email?: string; displayName: string; role: Role }
export interface APIErrorBody { code: string; message: string; details?: Record<string, unknown> }
export interface Patient {
  id: string; medicalRecordNumber: string; firstName: string; middleName: string; lastName: string; preferredName: string;
  sex: string; dateOfBirth: string; phone: string; alternatePhone: string; email: string; address: string; city: string;
  occupation: string; employer: string; preferredLanguage: string; communicationPreference: string; referralSource: string;
  referringProvider: string; notes: string; tags: string[]; version: number; createdAt: string; updatedAt: string; updatedBy: string;
}
export interface Appointment { id: string; patientId: string; medicalRecordNumber: string; patientName: string; practitionerId: string; practitionerName: string; startsAt: string; durationMinutes: number; type: string; reason: string; notes: string; status: string; version: number }
export interface QueueEntry { id: string; patientId: string; medicalRecordNumber: string; patientName: string; appointmentId: string; encounterId: string; assignedDoctorId: string; assignedDoctorName: string; arrivedAt: string; stage: string; priority: number; version: number; updatedAt: string }
export interface EncounterSummary { id: string; encounterNumber: string; patientId: string; medicalRecordNumber: string; patientName: string; doctorId: string; doctorName: string; visitReason: string; chiefComplaint: string; assessment: string; status: "draft" | "finalized"; finalizedAt: string; version: number; createdAt: string; updatedAt: string }
export interface InventoryItem { id: string; sku: string; barcode: string; category: string; name: string; brand: string; model: string; attributes: Record<string, unknown>; supplierId: string; costMinor: number; salePriceMinor: number; currency: string; quantity: number; reorderLevel: number; trackStock: boolean; lowStock: boolean; version: number; updatedAt: string }
export interface Invoice { id: string; invoiceNumber: string; patientId: string; patientName: string; status: string; currency: string; exchangeRate: string; subtotalMinor: number; discountMinor: number; taxMinor: number; totalMinor: number; paidMinor: number; balanceMinor: number; dueAt: string; version: number; createdAt: string; updatedAt: string }
export interface LabOrder { id: string; orderNumber: string; patientId: string; patientName: string; prescriptionId: string; invoiceId: string; supplierName: string; frameName: string; lensName: string; lensType: string; material: string; coatings: string[]; treatments: string[]; measurements: Record<string, unknown>; notes: string; orderedAt: string; expectedAt: string; status: string; deliveredAt: string; version: number; createdAt: string; updatedAt: string }

