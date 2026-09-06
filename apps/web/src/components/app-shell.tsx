import * as React from "react";
import {
  Activity,
  Archive,
  BarChart3,
  Boxes,
  CalendarDays,
  ClipboardCheck,
  ClipboardList,
  FileText,
  FlaskConical,
  Glasses,
  LogOut,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  Receipt,
  Search,
  Settings,
  ShieldCheck,
  ShoppingCart,
  Truck,
  Users,
  WalletCards,
  X,
} from "lucide-react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { useRealtime } from "../realtime";
import { cn } from "../lib";
import { CommandPalette } from "./command-palette";
import { Button } from "./ui/button";

const nav = [
  {
    section: "Overview",
    items: [
      { to: "/", label: "Dashboard", icon: Activity },
      { to: "/patients", label: "Patients", icon: Users },
      { to: "/schedule", label: "Appointments", icon: CalendarDays },
      { to: "/clinical", label: "Consultations", icon: ClipboardList },
    ],
  },
  {
    section: "Clinical",
    items: [
      { to: "/prescriptions", label: "Prescriptions", icon: Glasses },
      { to: "/lab", label: "Optical / Lab", icon: FlaskConical },
      { to: "/documents", label: "Documents", icon: FileText },
    ],
  },
  {
    section: "Operations",
    items: [
      { to: "/inventory", label: "Inventory", icon: Boxes },
      { to: "/stock-takes", label: "Stock takes", icon: ClipboardCheck },
      { to: "/purchasing", label: "Purchasing", icon: Truck, doctorOnly: true },
      { to: "/pos", label: "Point of Sale", icon: ShoppingCart },
      { to: "/billing", label: "Billing", icon: Receipt },
    ],
  },
  {
    section: "Finance",
    items: [
      { to: "/insurance", label: "Insurance", icon: ShieldCheck },
      { to: "/finance", label: "Finance", icon: WalletCards, doctorOnly: true },
      { to: "/reports", label: "Reports", icon: BarChart3, doctorOnly: true },
    ],
  },
  {
    section: "System",
    doctorOnly: true,
    items: [
      { to: "/system", label: "System", icon: Settings },
      { to: "/audit", label: "Audit log", icon: Archive },
    ],
  },
];

export function AppShell() {
  const { user, signOut } = useAuth();
  const realtime = useRealtime();
  const navigate = useNavigate();
  const [drawer, setDrawer] = React.useState(false);
  const [sidebarOpen, setSidebarOpen] = React.useState(true);
  const [search, setSearch] = React.useState(false);
  const logout = async () => {
    await signOut();
    navigate("/");
  };
  const links = nav
    .filter((group) => !group.doctorOnly || user?.role === "doctor")
    .map((group) => ({
      ...group,
      items: group.items.filter(
        (item) =>
          !("doctorOnly" in item && item.doctorOnly) || user?.role === "doctor",
      ),
    }));
  const sidebar = (
    <>
      <div className="flex h-16 items-center gap-3 border-b border-zinc-200 px-4">
        <div className="grid h-9 w-9 place-items-center rounded-lg bg-black text-white">
          <Glasses className="h-5 w-5" />
        </div>
        <div>
          <div className="font-bold leading-none">SentryMed Opti</div>
          <div className="mt-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">
            Clinic Management
          </div>
        </div>
      </div>
      <nav className="flex-1 overflow-y-auto p-3">
        {links.map((group) => (
          <div className="mb-5" key={group.section}>
            <div className="mb-1 px-3 text-[10px] font-bold uppercase tracking-[.15em] text-zinc-400">
              {group.section}
            </div>
            {group.items.map(({ to, label, icon: Icon }) => (
              <NavLink
                key={to}
                to={to}
                end={to === "/"}
                onClick={() => setDrawer(false)}
                className={({ isActive }) =>
                  cn(
                    "mb-0.5 flex h-10 items-center gap-3 rounded-md px-3 text-[13px] font-medium text-zinc-600 hover:bg-zinc-100 hover:text-black",
                    isActive && "bg-zinc-100 font-semibold text-black",
                  )
                }
              >
                <Icon className="h-4 w-4" />
                {label}
              </NavLink>
            ))}
          </div>
        ))}
      </nav>
      <div className="border-t border-zinc-200 bg-zinc-50 p-3">
        <div className="flex items-center gap-3 rounded-md p-2">
          <div className="grid h-9 w-9 place-items-center rounded-full bg-zinc-200 text-xs font-bold">
            {user?.displayName
              .split(" ")
              .map((part) => part[0])
              .slice(0, 2)
              .join("")}
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-semibold">
              {user?.displayName}
            </div>
            <div className="text-xs capitalize text-zinc-500">{user?.role}</div>
          </div>
          <Button
            aria-label="Sign out"
            size="icon"
            variant="ghost"
            onClick={logout}
          >
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </>
  );
  return (
    <div className="isolate flex min-h-screen bg-white">
      <aside className={cn("no-print fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r border-zinc-200 bg-white transition-transform duration-200 lg:flex", !sidebarOpen && "lg:-translate-x-full")}>
        {sidebar}
      </aside>
      {drawer && (
        <div className="no-print fixed inset-0 z-50 lg:hidden">
          <button
            aria-label="Close navigation"
            className="absolute inset-0 bg-black/40"
            onClick={() => setDrawer(false)}
          />
          <aside className="relative flex h-full w-[85%] max-w-80 flex-col bg-white shadow-xl">
            {sidebar}
            <button
              aria-label="Close navigation"
              className="absolute right-3 top-3 rounded p-2 hover:bg-zinc-100"
              onClick={() => setDrawer(false)}
            >
              <X className="h-5 w-5" />
            </button>
          </aside>
        </div>
      )}
      <div className={cn("relative z-0 min-w-0 flex-1 transition-[padding] duration-200", sidebarOpen && "lg:pl-64")}>
        <header className="no-print sticky top-0 z-20 flex h-16 items-center gap-3 border-b border-zinc-200 bg-white/95 px-4 backdrop-blur lg:px-8">
          <Button
            className="lg:hidden"
            size="icon"
            variant="ghost"
            onClick={() => setDrawer(true)}
          >
            <Menu className="h-5 w-5" />
          </Button>
          <Button
            aria-label={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
            className="hidden lg:inline-flex"
            size="icon"
            variant="ghost"
            onClick={() => setSidebarOpen((open) => !open)}
          >
            {sidebarOpen ? <PanelLeftClose className="h-5 w-5" /> : <PanelLeftOpen className="h-5 w-5" />}
          </Button>
          <button
            onClick={() => setSearch(true)}
            className="flex h-10 min-w-0 flex-1 items-center gap-2 rounded-md border border-zinc-200 bg-zinc-50 px-3 text-sm text-zinc-500 sm:max-w-sm"
          >
            <Search className="h-4 w-4" />
            <span className="truncate">Search patients, invoices, orders…</span>
            <kbd className="ml-auto hidden rounded border bg-white px-1.5 py-0.5 font-mono text-[10px] sm:block">
              Ctrl K
            </kbd>
          </button>
          <div className="ml-auto flex items-center gap-2 text-xs text-zinc-500">
            <span
              className={cn(
                "h-2 w-2 rounded-full",
                realtime.connected ? "bg-emerald-600" : "bg-amber-500",
              )}
            />
            <span className="hidden sm:inline">
              {realtime.connected ? "Live" : "Reconnecting"}
            </span>
          </div>
        </header>
        <main className="pb-20 lg:pb-0">
          <Outlet />
        </main>
      </div>
      <nav
        aria-label="Primary navigation"
        className="no-print safe-bottom pointer-events-auto fixed inset-x-0 bottom-0 z-50 grid grid-cols-5 border-t border-zinc-200 bg-white px-2 pt-2 shadow-[0_-4px_12px_rgba(0,0,0,0.04)] lg:hidden"
      >
        {[
          { to: "/", label: "Today", icon: Activity },
          { to: "/patients", label: "Patients", icon: Users },
          { to: "/schedule", label: "Queue", icon: CalendarDays },
          { to: "/clinical", label: "Clinical", icon: ClipboardList },
          { to: "/pos", label: "POS", icon: ShoppingCart },
        ].map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === "/"}
            className={({ isActive }) =>
              cn(
                "relative z-10 flex min-h-12 flex-col items-center gap-1 text-[10px] font-semibold text-zinc-500",
                isActive && "text-black",
              )
            }
          >
            <Icon className="h-5 w-5" />
            {label}
          </NavLink>
        ))}
      </nav>
      <CommandPalette open={search} onOpenChange={setSearch} />
    </div>
  );
}
