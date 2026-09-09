import { api } from "./api";
import { useLoad } from "./hooks";
import type { DocumentPaper, ReceiptWidth } from "./components/printing";

export interface ClinicIdentity {
  name: string;
  address: string;
  phone: string;
  email: string;
  nif?: string;
  timezone: string;
  currency: string;
}

export interface PrintingPreferences {
  /** Paper for invoices, prescriptions, lab requisitions and contracts. */
  documentPaper: DocumentPaper;
  /** Roll width for till receipts. */
  receiptWidth: ReceiptWidth;
}

const defaultPrinting: PrintingPreferences = { documentPaper: "A4", receiptWidth: "80mm" };

/**
 * The two print paths need the clinic's paper choices from plain functions as
 * well as from components, so the loaded preferences are cached here rather than
 * being threaded through every caller.
 */
let printingPreferences: PrintingPreferences = defaultPrinting;
export const clinicPrinting = () => printingPreferences;

export function usePrintingPreferences() {
  const response = useLoad(() => api.get<{ settings: Record<string, unknown> }>("/settings"));
  const stored = (response.data?.settings.printing ?? {}) as Partial<PrintingPreferences>;
  const resolved: PrintingPreferences = {
    documentPaper: stored.documentPaper === "Letter" ? "Letter" : "A4",
    receiptWidth: stored.receiptWidth === "58mm" ? "58mm" : "80mm",
  };
  printingPreferences = resolved;
  return resolved;
}

export function useClinicIdentity() {
  const response = useLoad(() => api.get<{ settings: Record<string, unknown> }>("/settings"));
  return (response.data?.settings.clinic ?? {}) as Partial<ClinicIdentity>;
}
