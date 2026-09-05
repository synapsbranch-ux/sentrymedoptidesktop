import { api } from "./api";
import { useLoad } from "./hooks";

export interface ClinicIdentity {
  name: string;
  address: string;
  phone: string;
  email: string;
  nif?: string;
  timezone: string;
  currency: string;
}

export function useClinicIdentity() {
  const response = useLoad(() => api.get<{ settings: Record<string, unknown> }>("/settings"));
  return (response.data?.settings.clinic ?? {}) as Partial<ClinicIdentity>;
}
