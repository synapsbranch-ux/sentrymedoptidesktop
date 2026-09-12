import * as React from "react";
import { Pencil, Plus, Trash2, X } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useLoad } from "../hooks";
import { useRealtime } from "../realtime";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Badge, EmptyState, ErrorState, Skeleton } from "./ui/data";
import { Field, Input, Select } from "./ui/input";

/**
 * D2: the clinic's own lists are edited here rather than compiled into the
 * application — appointment reasons, prescription items, and what a glazing
 * company or a laboratory has to be told. Retiring an entry deactivates it, so a
 * record that already refers to it keeps its meaning.
 */

export type CatalogName = "appointment_reason" | "prescription_item" | "lens_type" | "lens_material" | "lens_coating" | "lens_tint" | "lens_treatment" | "lab_test";

export interface CatalogEntry {
  id: string;
  catalog: CatalogName;
  label: string;
  details: Record<string, string>;
  sortOrder: number;
  active: boolean;
  version: number;
  updatedAt: string;
}

/** The extra fields a prescription item carries into a prescription. */
export const prescriptionItemFields = ["strength", "dosage", "frequency", "route", "duration", "instructions"] as const;

export function useCatalog(catalog: CatalogName, options: { includeInactive?: boolean } = {}) {
  const { revision } = useRealtime();
  const suffix = options.includeInactive ? "?includeInactive=true" : "";
  return useLoad(() => api.get<{ items: CatalogEntry[] }>(`/catalogs/${catalog}${suffix}`), [catalog, suffix, revision]);
}

export function CatalogManager({ catalog, title, description, detailFields }: {
  catalog: CatalogName;
  title: string;
  description: string;
  detailFields?: readonly string[];
}) {
  const entries = useCatalog(catalog, { includeInactive: true });
  const [editing, setEditing] = React.useState<CatalogEntry | null>(null);
  const [adding, setAdding] = React.useState(false);

  const retire = async (entry: CatalogEntry) => {
    try {
      await api.delete(`/catalogs/${catalog}/${entry.id}`);
      toast.success(`${entry.label} retired`);
      entries.reload();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Could not retire the entry");
    }
  };

  const restore = async (entry: CatalogEntry) => {
    try {
      await api.put(`/catalogs/${catalog}/${entry.id}`, { label: entry.label, details: entry.details, sortOrder: entry.sortOrder, active: true, version: entry.version });
      toast.success(`${entry.label} restored`);
      entries.reload();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Could not restore the entry");
    }
  };

  return (
    <Card>
      <CardHeader className="flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div><CardTitle>{title}</CardTitle><CardDescription>{description}</CardDescription></div>
        <Button size="sm" onClick={() => { setAdding(true); setEditing(null); }}><Plus className="h-3.5 w-3.5" />Add entry</Button>
      </CardHeader>
      <CardContent className="grid gap-3">
        {(adding || editing) && (
          <CatalogEntryForm
            catalog={catalog}
            entry={editing}
            detailFields={detailFields}
            onClose={() => { setAdding(false); setEditing(null); }}
            onSaved={() => { setAdding(false); setEditing(null); entries.reload(); }}
          />
        )}
        {entries.loading ? <Skeleton className="h-40" /> : entries.error ? <ErrorState message={entries.error.message} retry={entries.reload} /> : entries.data?.items.length ? (
          <ul className="divide-y">
            {entries.data.items.map((entry) => (
              <li key={entry.id} className="flex flex-wrap items-center gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className={`font-semibold ${entry.active ? "" : "text-zinc-400 line-through"}`}>{entry.label}</span>
                    {!entry.active && <Badge>Retired</Badge>}
                  </div>
                  {Object.keys(entry.details).length > 0 && (
                    <div className="mt-0.5 text-xs text-zinc-500">
                      {Object.entries(entry.details).map(([key, value]) => `${key}: ${value}`).join(" · ")}
                    </div>
                  )}
                </div>
                {entry.active ? (
                  <>
                    <Button aria-label={`Edit ${entry.label}`} size="icon" variant="ghost" onClick={() => { setEditing(entry); setAdding(false); }}><Pencil className="h-4 w-4" /></Button>
                    <Button aria-label={`Retire ${entry.label}`} size="icon" variant="ghost" onClick={() => retire(entry)}><Trash2 className="h-4 w-4" /></Button>
                  </>
                ) : (
                  <Button size="sm" variant="outline" onClick={() => restore(entry)}>Restore</Button>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <EmptyState
            title="This catalog is empty"
            description="Add the entries the clinic uses. They take effect immediately, with no new release."
            action={<Button onClick={() => setAdding(true)}><Plus className="h-4 w-4" />Add entry</Button>}
          />
        )}
      </CardContent>
    </Card>
  );
}

function CatalogEntryForm({ catalog, entry, detailFields, onSaved, onClose }: {
  catalog: CatalogName;
  entry: CatalogEntry | null;
  detailFields?: readonly string[];
  onSaved(): void;
  onClose(): void;
}) {
  const [label, setLabel] = React.useState(entry?.label ?? "");
  const [sortOrder, setSortOrder] = React.useState(entry?.sortOrder ?? 0);
  const [details, setDetails] = React.useState<Record<string, string>>(entry?.details ?? {});
  const [saving, setSaving] = React.useState(false);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      const body = { label, sortOrder, details, ...(entry ? { active: entry.active, version: entry.version } : {}) };
      if (entry) await api.put(`/catalogs/${catalog}/${entry.id}`, body);
      else await api.post(`/catalogs/${catalog}`, body);
      toast.success(entry ? "Entry updated" : "Entry added");
      onSaved();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Could not save the entry");
    } finally {
      setSaving(false);
    }
  };

  return (
    <form className="grid gap-3 rounded-lg border bg-[var(--muted)] p-4" onSubmit={submit}>
      <div className="flex items-center justify-between">
        <strong className="text-sm">{entry ? "Edit entry" : "New entry"}</strong>
        <Button aria-label="Cancel" size="icon" variant="ghost" onClick={onClose} type="button"><X className="h-4 w-4" /></Button>
      </div>
      <div className="grid gap-3 sm:grid-cols-[1fr_7rem]">
        <Field label="Label"><Input autoFocus required maxLength={120} value={label} onChange={(event) => setLabel(event.target.value)} /></Field>
        <Field label="Order" hint="Lower first"><Input type="number" value={sortOrder} onChange={(event) => setSortOrder(Number(event.target.value))} /></Field>
      </div>
      {detailFields && detailFields.length > 0 && (
        <div className="grid gap-3 sm:grid-cols-2">
          {detailFields.map((field) => (
            <Field key={field} label={field.charAt(0).toUpperCase() + field.slice(1)} hint="Optional default">
              <Input value={details[field] ?? ""} onChange={(event) => setDetails({ ...details, [field]: event.target.value })} />
            </Field>
          ))}
        </div>
      )}
      <div className="flex justify-end"><Button type="submit" size="sm" disabled={saving}>{saving ? "Saving…" : "Save entry"}</Button></div>
    </form>
  );
}

/**
 * A text field backed by a catalog. The clinic's entries are offered as
 * suggestions while anything else can still be typed, so adopting a catalog
 * never blocks a reason or a medication that is not in it yet.
 */
export function CatalogInput({ catalog, value, onChange, listId, placeholder }: {
  catalog: CatalogName;
  value: string;
  onChange(value: string): void;
  listId: string;
  placeholder?: string;
}) {
  const entries = useCatalog(catalog);
  return (
    <>
      <Input list={listId} value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
      <datalist id={listId}>
        {entries.data?.items.map((entry) => <option key={entry.id} value={entry.label} />)}
      </datalist>
    </>
  );
}

/**
 * Picks a catalog entry to prefill a form. When the catalog is empty it says so
 * and points at where to fill it, rather than showing an inert dropdown.
 */
export function CatalogPicker({ catalog, label, hint, onSelect }: {
  catalog: CatalogName;
  label: string;
  hint?: string;
  onSelect(entry: CatalogEntry): void;
}) {
  const entries = useCatalog(catalog);
  const items = entries.data?.items ?? [];
  if (!entries.loading && items.length === 0) {
    return <p className="rounded-md border border-dashed p-3 text-xs text-[var(--muted-foreground)]">
      The clinic prescription catalog is empty. A doctor can fill it in System → Catalogs; until then, type the prescription below.
    </p>;
  }
  return (
    <Field label={label} hint={hint}>
      <Select
        value=""
        onChange={(event) => {
          const entry = items.find((candidate) => candidate.id === event.target.value);
          if (entry) onSelect(entry);
        }}
      >
        <option value="">Choose from the catalog…</option>
        {items.map((entry) => <option key={entry.id} value={entry.id}>{entry.label}</option>)}
      </Select>
    </Field>
  );
}
