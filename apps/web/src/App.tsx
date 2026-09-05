import { Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";
import { useAuth } from "./auth";
import { AppShell } from "./components/app-shell";
import { LoginScreen, SetupScreen } from "./components/auth-screens";
import { RealtimeProvider } from "./realtime";
import { AuditPage } from "./pages/audit";
import { BillingPage } from "./pages/billing";
import { ClinicalPage } from "./pages/clinical";
import { DashboardPage } from "./pages/dashboard";
import { FinancePage } from "./pages/finance";
import { InventoryPage } from "./pages/inventory";
import { LabPage } from "./pages/lab";
import { PatientsPage } from "./pages/patients";
import { POSPage } from "./pages/pos";
import { DocumentsPage, PrescriptionsPage } from "./pages/records";
import { ReportsPage } from "./pages/reports";
import { SchedulePage } from "./pages/schedule";
import { SystemPage } from "./pages/system";

function DoctorOnly({ children }: { children: React.ReactNode }) {
  const { user } = useAuth();
  return user?.role === "doctor" ? children : <Navigate to="/" replace />;
}

export default function App() {
  const auth = useAuth();
  if (auth.loading) return <div className="grid min-h-screen place-items-center bg-white"><div className="text-center"><div className="mx-auto h-9 w-9 animate-pulse rounded-lg bg-black" /><p className="mt-4 text-sm text-zinc-500">Connecting to clinic server…</p></div></div>;
  if (auth.setupRequired) return <><SetupScreen /><Toaster richColors position="bottom-right" /></>;
  if (!auth.user) return <><LoginScreen /><Toaster richColors position="bottom-right" /></>;
  return <RealtimeProvider><Routes><Route element={<AppShell />}><Route index element={<DashboardPage />} /><Route path="patients" element={<PatientsPage />} /><Route path="schedule" element={<SchedulePage />} /><Route path="clinical" element={<ClinicalPage />} /><Route path="prescriptions" element={<PrescriptionsPage />} /><Route path="documents" element={<DocumentsPage />} /><Route path="inventory" element={<InventoryPage />} /><Route path="pos" element={<POSPage />} /><Route path="billing" element={<BillingPage />} /><Route path="lab" element={<LabPage />} /><Route path="finance" element={<DoctorOnly><FinancePage /></DoctorOnly>} /><Route path="reports" element={<DoctorOnly><ReportsPage /></DoctorOnly>} /><Route path="system" element={<DoctorOnly><SystemPage /></DoctorOnly>} /><Route path="audit" element={<DoctorOnly><AuditPage /></DoctorOnly>} /><Route path="*" element={<Navigate to="/" replace />} /></Route></Routes><Toaster richColors position="bottom-right" closeButton /></RealtimeProvider>;
}

