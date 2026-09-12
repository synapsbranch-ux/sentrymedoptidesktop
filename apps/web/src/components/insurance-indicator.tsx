import * as React from "react";
import { CheckCircle2, ShieldAlert, ShieldOff } from "lucide-react";
import { api } from "../api";
import { useLoad } from "../hooks";
import type { PatientPolicy } from "../types";

/**
 * A compact, one-line insurance indicator for screens where coverage matters
 * but should not dominate — an appointment or consultation header. It is
 * purely informational and never blocks anything: medical care is never
 * withheld for missing, expired or unverified insurance, only flagged.
 */
export function InsuranceIndicator({ patientId }: { patientId: string }) {
  const policies = useLoad(() => api.get<{ items: PatientPolicy[] }>(`/insurance/policies?patientId=${encodeURIComponent(patientId)}`), [patientId]);
  if (!patientId || policies.loading || policies.error) return null;
  const primary = policies.data?.items.find((policy) => policy.isPrimary && policy.isActive) ?? policies.data?.items.find((policy) => policy.isActive);
  if (!primary) return null;
  if (primary.expired || primary.verificationStatus === "expired")
    return <span className="inline-flex items-center gap-1 text-xs font-medium text-red-700"><ShieldOff className="h-3.5 w-3.5 shrink-0" />Insurance: {primary.payerName} · Expired</span>;
  if (primary.verificationStatus === "verified")
    return <span className="inline-flex items-center gap-1 text-xs font-medium text-emerald-700"><CheckCircle2 className="h-3.5 w-3.5 shrink-0" />Insurance: {primary.payerName} · Verified</span>;
  return <span className="inline-flex items-center gap-1 text-xs font-medium text-amber-700"><ShieldAlert className="h-3.5 w-3.5 shrink-0" />Insurance: {primary.payerName} · Verification required</span>;
}
