import * as React from "react";
import { api } from "./api";
import { useLoad } from "./hooks";
import type { DocumentPaper, ReceiptWidth } from "./components/printing";
import { desktopBridge } from "./native";

export const DEFAULT_CLINIC_NAME = "Clinique Le Bon Spécialiste";
export const DEFAULT_CLINIC_LOGO_URL = "/api/v1/public/branding/logo?v=clinique-branding-v2";

export interface PublicBranding {
  name: string;
  logoUrl: string;
}

export function usePublicBranding(revision = 0) {
  const [branding, setBranding] = React.useState<PublicBranding>({
    name: DEFAULT_CLINIC_NAME,
    logoUrl: DEFAULT_CLINIC_LOGO_URL,
  });
  React.useEffect(() => {
    let active = true;
    api.get<Partial<PublicBranding>>("/public/branding")
      .then((value) => {
        if (!active) return;
        const name = value.name?.trim() || DEFAULT_CLINIC_NAME;
        setBranding({ name, logoUrl: value.logoUrl || DEFAULT_CLINIC_LOGO_URL });
        document.title = name;
        void desktopBridge()?.SetWindowTitle(name);
      })
      .catch(() => { document.title = DEFAULT_CLINIC_NAME; });
    return () => { active = false; };
  }, [revision]);
  return branding;
}

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
