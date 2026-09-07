import * as React from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";
import { useAuth } from "./auth";
import { AppShell } from "./components/app-shell";
import { LoginScreen, SetupScreen } from "./components/auth-screens";
import { RealtimeProvider } from "./realtime";
import { PublicDisplayPage } from "./pages/public-display";
import { KioskPage } from "./pages/kiosk";
import { useI18n } from "./i18n";

const AuditPage = React.lazy(() => import("./pages/audit").then((module) => ({ default: module.AuditPage })));
const BillingPage = React.lazy(() => import("./pages/billing").then((module) => ({ default: module.BillingPage })));
const ClinicalPage = React.lazy(() => import("./pages/clinical").then((module) => ({ default: module.ClinicalPage })));
const DashboardPage = React.lazy(() => import("./pages/dashboard").then((module) => ({ default: module.DashboardPage })));
const FinancePage = React.lazy(() => import("./pages/finance").then((module) => ({ default: module.FinancePage })));
const InventoryPage = React.lazy(() => import("./pages/inventory").then((module) => ({ default: module.InventoryPage })));
const InsurancePage = React.lazy(() => import("./pages/insurance").then((module) => ({ default: module.InsurancePage })));
const LabPage = React.lazy(() => import("./pages/lab").then((module) => ({ default: module.LabPage })));
const PatientsPage = React.lazy(() => import("./pages/patients").then((module) => ({ default: module.PatientsPage })));
const POSPage = React.lazy(() => import("./pages/pos").then((module) => ({ default: module.POSPage })));
const PurchasingPage = React.lazy(() => import("./pages/purchasing").then((module) => ({ default: module.PurchasingPage })));
const DocumentsPage = React.lazy(() => import("./pages/records").then((module) => ({ default: module.DocumentsPage })));
const PrescriptionsPage = React.lazy(() => import("./pages/records").then((module) => ({ default: module.PrescriptionsPage })));
const ReportsPage = React.lazy(() => import("./pages/reports").then((module) => ({ default: module.ReportsPage })));
const SchedulePage = React.lazy(() => import("./pages/schedule").then((module) => ({ default: module.SchedulePage })));
const VisionTestPage = React.lazy(() => import("./pages/vision-test").then((module) => ({ default: module.VisionTestPage })));
const VisionDisplayPage = React.lazy(() => import("./pages/vision-display").then((module) => ({ default: module.VisionDisplayPage })));
const SystemPage = React.lazy(() => import("./pages/system").then((module) => ({ default: module.SystemPage })));
const StockTakesPage = React.lazy(() => import("./pages/stock-takes").then((module) => ({ default: module.StockTakesPage })));

function DoctorOnly({ children }: { children: React.ReactNode }) {
  const { user } = useAuth();
  return user?.role === "doctor" ? children : <Navigate to="/" replace />;
}

export default function App() {
  const auth = useAuth();
  const { t } = useI18n();
  if (window.location.pathname === "/display") return <PublicDisplayPage />;
  if (window.location.pathname === "/kiosk") return <KioskPage />;
  if (auth.loading)
    return (
      <div className="grid min-h-screen place-items-center bg-white">
        <div className="text-center">
          <div className="mx-auto h-9 w-9 animate-pulse rounded-lg bg-black" />
          <p className="mt-4 text-sm text-zinc-500">
            {t("Connecting to clinic server…")}
          </p>
        </div>
      </div>
    );
  if (auth.setupRequired)
    return (
      <>
        <SetupScreen />
        <Toaster richColors position="bottom-right" />
      </>
    );
  if (!auth.user)
    return (
      <>
        <LoginScreen />
        <Toaster richColors position="bottom-right" />
      </>
    );
  return (
    <RealtimeProvider>
      <React.Suspense fallback={<div className="page"><div className="h-8 w-48 animate-pulse rounded bg-[var(--muted)]" /><div className="mt-6 h-80 animate-pulse rounded-[var(--radius)] bg-[var(--muted)]" /></div>}>
      <Routes>
        <Route path="vision-display/:id" element={<VisionDisplayPage />} />
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="patients" element={<PatientsPage />} />
          <Route path="schedule" element={<SchedulePage />} />
          <Route path="clinical" element={<ClinicalPage />} />
          <Route path="vision-test" element={<VisionTestPage />} />
          <Route path="prescriptions" element={<PrescriptionsPage />} />
          <Route path="documents" element={<DocumentsPage />} />
          <Route path="inventory" element={<InventoryPage />} />
          <Route path="stock-takes" element={<StockTakesPage />} />
          <Route
            path="purchasing"
            element={
              <DoctorOnly>
                <PurchasingPage />
              </DoctorOnly>
            }
          />
          <Route path="pos" element={<POSPage />} />
          <Route path="billing" element={<BillingPage />} />
          <Route path="insurance" element={<InsurancePage />} />
          <Route path="lab" element={<LabPage />} />
          <Route
            path="finance"
            element={
              <DoctorOnly>
                <FinancePage />
              </DoctorOnly>
            }
          />
          <Route
            path="reports"
            element={
              <DoctorOnly>
                <ReportsPage />
              </DoctorOnly>
            }
          />
          <Route
            path="system"
            element={
              <DoctorOnly>
                <SystemPage />
              </DoctorOnly>
            }
          />
          <Route
            path="audit"
            element={
              <DoctorOnly>
                <AuditPage />
              </DoctorOnly>
            }
          />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
      </React.Suspense>
      <Toaster richColors position="bottom-right" closeButton />
    </RealtimeProvider>
  );
}
