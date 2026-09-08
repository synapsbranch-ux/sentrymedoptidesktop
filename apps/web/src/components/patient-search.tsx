import * as React from "react";
import { Search, X } from "lucide-react";
import { api } from "../api";
import { useDebouncedValue, useLoad } from "../hooks";
import type { Patient } from "../types";
import { Button } from "./ui/button";
import { Input, Select } from "./ui/input";
import { useI18n } from "../i18n";

/**
 * B1: every patient lookup in the application goes through the server, one page
 * at a time. Nothing loads the patients table in full, which is what made the
 * old `/patients?limit=100` dropdown unusable past a few thousand records.
 */

export interface PatientSearchResult extends Patient {
  lastVisitAt: string;
}

export interface PatientSearchPage {
  items: PatientSearchResult[];
  page: number;
  limit: number;
  total: number;
  hasMore: boolean;
}

export interface PatientFilters {
  sex: string;
  ageMin: string;
  ageMax: string;
  civilStatus: string;
  insurance: string;
  practitioner: string;
  lastVisitFrom: string;
  lastVisitUntil: string;
  status: string;
}

export const emptyPatientFilters: PatientFilters = {
  sex: "", ageMin: "", ageMax: "", civilStatus: "", insurance: "", practitioner: "",
  lastVisitFrom: "", lastVisitUntil: "", status: "active",
};

export function patientSearchQuery(query: string, filters: PatientFilters, page: number, limit: number) {
  const parameters = new URLSearchParams({ page: String(page), limit: String(limit) });
  if (query.trim()) parameters.set("q", query.trim());
  for (const [key, value] of Object.entries(filters)) if (value) parameters.set(key, value);
  return `/patients?${parameters.toString()}`;
}

export function activeFilterCount(filters: PatientFilters) {
  return Object.entries(filters).filter(([key, value]) => value && !(key === "status" && value === "active")).length;
}

export function patientAge(dateOfBirth: string) {
  if (!dateOfBirth) return null;
  const born = new Date(dateOfBirth);
  if (Number.isNaN(born.getTime())) return null;
  const now = new Date();
  let age = now.getFullYear() - born.getFullYear();
  const monthDelta = now.getMonth() - born.getMonth();
  if (monthDelta < 0 || (monthDelta === 0 && now.getDate() < born.getDate())) age -= 1;
  return age >= 0 && age < 150 ? age : null;
}

interface FilterOptions { insurers: { value: string; label: string }[]; practitioners: { value: string; label: string }[] }

export function PatientFilterBar({ filters, onChange, civilStatusOptions }: {
  filters: PatientFilters;
  onChange(filters: PatientFilters): void;
  civilStatusOptions?: { value: string; label: string }[];
}) {
  const { t } = useI18n();
  const options = useLoad(() => api.get<FilterOptions>("/patients/filter-options"), []);
  const set = (key: keyof PatientFilters) => (event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    onChange({ ...filters, [key]: event.target.value });
  return (
    <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-2 xl:grid-cols-4">
      <label className="grid gap-1 text-xs font-semibold">{t("Sex")}
        <Select value={filters.sex} onChange={set("sex")}>
          <option value="">{t("Any")}</option><option value="female">{t("Female")}</option><option value="male">{t("Male")}</option><option value="other">{t("Other")}</option>
        </Select>
      </label>
      <div className="grid gap-1 text-xs font-semibold">{t("Age range")}
        <div className="flex items-center gap-2">
          <Input aria-label={t("Minimum age")} inputMode="numeric" placeholder="0" value={filters.ageMin} onChange={set("ageMin")} />
          <span aria-hidden>–</span>
          <Input aria-label={t("Maximum age")} inputMode="numeric" placeholder="120" value={filters.ageMax} onChange={set("ageMax")} />
        </div>
      </div>
      {civilStatusOptions && (
        <label className="grid gap-1 text-xs font-semibold">{t("Civil status")}
          <Select value={filters.civilStatus} onChange={set("civilStatus")}>
            <option value="">{t("Any")}</option>
            {civilStatusOptions.map((option) => <option key={option.value} value={option.value}>{t(option.label)}</option>)}
          </Select>
        </label>
      )}
      <label className="grid gap-1 text-xs font-semibold">{t("Insurance")}
        <Select value={filters.insurance} onChange={set("insurance")}>
          <option value="">{t("Any")}</option>
          {options.data?.insurers.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select>
      </label>
      <label className="grid gap-1 text-xs font-semibold">{t("Practitioner")}
        <Select value={filters.practitioner} onChange={set("practitioner")}>
          <option value="">{t("Any")}</option>
          {options.data?.practitioners.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select>
      </label>
      <label className="grid gap-1 text-xs font-semibold">{t("Last visit from")}
        <Input type="date" value={filters.lastVisitFrom} onChange={set("lastVisitFrom")} />
      </label>
      <label className="grid gap-1 text-xs font-semibold">{t("Last visit until")}
        <Input type="date" value={filters.lastVisitUntil} onChange={set("lastVisitUntil")} />
      </label>
      <label className="grid gap-1 text-xs font-semibold">{t("Record status")}
        <Select value={filters.status} onChange={set("status")}>
          <option value="active">{t("Active")}</option><option value="archived">{t("Archived")}</option><option value="all">{t("Active and archived")}</option>
        </Select>
      </label>
      <div className="flex items-end">
        <Button size="sm" variant="ghost" onClick={() => onChange(emptyPatientFilters)} disabled={activeFilterCount(filters) === 0}>
          <X className="h-3.5 w-3.5" />{t("Clear filters")}
        </Button>
      </div>
    </div>
  );
}

/**
 * A patient picker that queries the server as the user types. Replaces the
 * `<select>` that fetched the first 100 patients and could not find anybody else.
 */
export function PatientPicker({ value, onChange, required, placeholder = "Search by name, file number, phone or date of birth…" }: {
  value: string;
  onChange(patientId: string, patient?: PatientSearchResult): void;
  required?: boolean;
  placeholder?: string;
}) {
  const { t } = useI18n();
  const [query, setQuery] = React.useState("");
  const [open, setOpen] = React.useState(false);
  const [selected, setSelected] = React.useState<PatientSearchResult | null>(null);
  const debounced = useDebouncedValue(query, 300);
  const results = useLoad(
    () => (debounced.trim().length >= 2
      ? api.get<PatientSearchPage>(patientSearchQuery(debounced, emptyPatientFilters, 1, 10))
      : Promise.resolve({ items: [], page: 1, limit: 10, total: 0, hasMore: false } as PatientSearchPage)),
    [debounced],
  );

  React.useEffect(() => { if (!value) setSelected(null); }, [value]);

  if (value && selected) {
    return (
      <div className="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--input)] bg-[var(--card)] p-2">
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold">{selected.firstName} {selected.lastName}</div>
          <div className="truncate font-mono text-xs text-[var(--muted-foreground)]">{selected.medicalRecordNumber} · {selected.dateOfBirth || "—"} · {selected.phone || "—"}</div>
        </div>
        <Button aria-label={t("Change patient")} size="icon" variant="ghost" onClick={() => { setSelected(null); onChange(""); setQuery(""); }}><X className="h-4 w-4" /></Button>
      </div>
    );
  }

  return (
    <div className="relative">
      <Search aria-hidden className="pointer-events-none absolute left-3 top-3.5 h-4 w-4 text-[var(--muted-foreground)]" />
      <Input
        className="pl-10"
        role="combobox"
        aria-expanded={open}
        aria-label={t("Patient")}
        autoComplete="off"
        required={required && !value}
        placeholder={placeholder}
        value={query}
        onChange={(event) => { setQuery(event.target.value); setOpen(true); }}
        onFocus={() => setOpen(true)}
        onBlur={() => window.setTimeout(() => setOpen(false), 150)}
      />
      {open && debounced.trim().length >= 2 && (
        <div role="listbox" className="absolute z-30 mt-1 max-h-64 w-full overflow-y-auto rounded-md border bg-[var(--card)] shadow-lg">
          {results.loading && <p className="px-3 py-2 text-sm text-[var(--muted-foreground)]">{t("Searching…")}</p>}
          {!results.loading && results.data?.items.length === 0 && <p className="px-3 py-2 text-sm text-[var(--muted-foreground)]">{t("No matching patient.")}</p>}
          {results.data?.items.map((patient) => (
            <button
              key={patient.id}
              type="button"
              role="option"
              aria-selected={patient.id === value}
              className="block w-full px-3 py-2 text-left text-sm hover:bg-[var(--muted)]"
              // mousedown only stops the input's blur from closing the list; the
              // selection happens on click so that keyboard and assistive-technology
              // activation, which never produces a mousedown, still works.
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => { setSelected(patient); onChange(patient.id, patient); setOpen(false); }}
            >
              <span className="font-semibold">{patient.firstName} {patient.lastName}</span>
              <span className="ml-2 font-mono text-xs text-[var(--muted-foreground)]">{patient.medicalRecordNumber}</span>
              <div className="font-mono text-xs text-[var(--muted-foreground)]">{patient.dateOfBirth || "—"} · {patient.phone || "—"}</div>
            </button>
          ))}
          {results.data?.hasMore && <p className="px-3 py-2 text-xs text-[var(--muted-foreground)]">{t("Keep typing to narrow these results.")}</p>}
        </div>
      )}
    </div>
  );
}
