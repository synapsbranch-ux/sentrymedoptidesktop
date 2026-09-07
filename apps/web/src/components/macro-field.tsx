import * as React from "react";
import { toast } from "sonner";
import { Sparkles, Trash2 } from "lucide-react";
import { api } from "../api";
import { APIError } from "../api";
import { useLoad } from "../hooks";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "./ui/dialog";
import { EmptyState, Skeleton } from "./ui/data";
import { Field, Input, Select, Textarea } from "./ui/input";

interface Macro { id: string; label: string; category: string; body: string; version: number }

const categoryLabels: Record<string, string> = {
  general: "General",
  chief_complaint: "Chief complaint",
  hpi: "History of present illness",
  assessment: "Assessment",
  treatment_plan: "Treatment plan",
  follow_up: "Follow-up",
};

export function MacroTextarea({ label, category, value, onChange, disabled, rows }: { label: string; category: keyof typeof categoryLabels; value: string; onChange(value: string): void; disabled?: boolean; rows?: number }) {
  const [open, setOpen] = React.useState(false);
  return (
    <Field label={label}>
      <div className="grid gap-1.5">
        <Textarea rows={rows} disabled={disabled} value={value} onChange={(event) => onChange(event.target.value)} />
        {!disabled && (
          <div className="flex justify-end">
            <Dialog open={open} onOpenChange={setOpen}>
              <DialogTrigger asChild>
                <Button type="button" size="sm" variant="ghost">
                  <Sparkles className="h-3.5 w-3.5" />
                  Insert template
                </Button>
              </DialogTrigger>
              <MacroPickerDialog
                category={category}
                onInsert={(body) => {
                  onChange(value ? `${value}\n${body}` : body);
                  setOpen(false);
                }}
              />
            </Dialog>
          </div>
        )}
      </div>
    </Field>
  );
}

function MacroPickerDialog({ category, onInsert }: { category: string; onInsert(body: string): void }) {
  const [showAll, setShowAll] = React.useState(false);
  const macros = useLoad(() => api.get<{ items: Macro[] }>(`/macros${showAll ? "" : `?category=${category}`}`), [category, showAll]);
  const [creating, setCreating] = React.useState(false);
  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Charting templates</DialogTitle>
        <DialogDescription>Reusable text snippets your clinic can insert into consultation notes.</DialogDescription>
      </DialogHeader>
      <div className="flex items-center justify-between">
        <label className="flex items-center gap-2 text-xs text-zinc-500">
          <input type="checkbox" checked={showAll} onChange={(event) => setShowAll(event.target.checked)} />
          Show all categories
        </label>
        <Button size="sm" variant="outline" type="button" onClick={() => setCreating((value) => !value)}>
          {creating ? "Cancel" : "New template"}
        </Button>
      </div>
      {creating && <NewMacroForm defaultCategory={category} onSaved={() => { setCreating(false); macros.reload(); }} />}
      <div className="grid max-h-96 gap-2 overflow-y-auto">
        {macros.loading ? (
          <Skeleton className="h-32" />
        ) : macros.data?.items.length ? (
          macros.data.items.map((macro) => (
            <div key={macro.id} className="rounded-md border p-3 text-sm">
              <div className="flex items-start justify-between gap-2">
                <div>
                  <div className="font-semibold">{macro.label}</div>
                  <div className="text-xs text-zinc-500">{categoryLabels[macro.category] ?? macro.category}</div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button size="sm" onClick={() => onInsert(macro.body)}>Insert</Button>
                  <Button
                    aria-label={`Delete ${macro.label}`}
                    size="icon"
                    variant="ghost"
                    type="button"
                    onClick={async () => {
                      try {
                        await api.delete(`/macros/${macro.id}?version=${macro.version}`);
                        macros.reload();
                      } catch (reason) {
                        toast.error(reason instanceof Error ? reason.message : "Could not delete template");
                      }
                    }}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
              <p className="mt-2 whitespace-pre-wrap text-zinc-600">{macro.body}</p>
            </div>
          ))
        ) : (
          <EmptyState title="No templates yet" description="Create one below to reuse it across consultations." />
        )}
      </div>
    </DialogContent>
  );
}

function NewMacroForm({ defaultCategory, onSaved }: { defaultCategory: string; onSaved(): void }) {
  const [form, setForm] = React.useState({ label: "", category: defaultCategory, body: "" });
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      await api.post("/macros", form);
      toast.success("Template created");
      onSaved();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Could not create template");
    } finally {
      setSaving(false);
    }
  };
  return (
    <form className="grid gap-3 rounded-md border border-dashed p-3" onSubmit={submit}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Label">
          <Input required value={form.label} onChange={(event) => setForm({ ...form, label: event.target.value })} />
        </Field>
        <Field label="Category">
          <Select value={form.category} onChange={(event) => setForm({ ...form, category: event.target.value })}>
            {Object.entries(categoryLabels).map(([value, text]) => (
              <option key={value} value={value}>{text}</option>
            ))}
          </Select>
        </Field>
      </div>
      <Field label="Body">
        <Textarea required value={form.body} onChange={(event) => setForm({ ...form, body: event.target.value })} />
      </Field>
      <DialogFooter>
        <Button type="submit" disabled={saving}>{saving ? "Saving…" : "Save template"}</Button>
      </DialogFooter>
    </form>
  );
}
