import * as React from "react";
import { toast } from "sonner";
import { Trash2 } from "lucide-react";
import { api, APIError } from "../api";
import { useLoad } from "../hooks";
import { cn } from "../lib";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Skeleton } from "./ui/data";
import { Field, Input, Select, Textarea } from "./ui/input";

interface Mark { x: number; y: number; shape: string; color: string; label: string; structure: string }
interface EyeState { annotations: Mark[]; notes: string; version: number }
type Diagrams = Record<"OD" | "OS", EyeState>;

const structures = ["cornea", "conjunctiva", "iris", "lens", "sclera", "eyelid", "vitreous", "retina", "macula", "optic_nerve", "other"];
const colors = [
  { value: "#dc2626", label: "Abnormal" },
  { value: "#d97706", label: "Watch" },
  { value: "#2563eb", label: "Note" },
  { value: "#16a34a", label: "Normal" },
];

export function EyeDiagramPanel({ encounterId, canEdit }: { encounterId: string; canEdit: boolean }) {
  const diagrams = useLoad(() => api.get<Diagrams>(`/encounters/${encounterId}/eye-diagrams`), [encounterId]);
  if (diagrams.loading) return <Skeleton className="h-96" />;
  if (!diagrams.data) return null;
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <EyeEditor eye="OD" title="Right eye (OD)" state={diagrams.data.OD} encounterId={encounterId} canEdit={canEdit} onSaved={diagrams.reload} />
      <EyeEditor eye="OS" title="Left eye (OS)" state={diagrams.data.OS} encounterId={encounterId} canEdit={canEdit} onSaved={diagrams.reload} />
    </div>
  );
}

function EyeEditor({ eye, title, state, encounterId, canEdit, onSaved }: { eye: "OD" | "OS"; title: string; state: EyeState; encounterId: string; canEdit: boolean; onSaved(): void }) {
  const [annotations, setAnnotations] = React.useState(state.annotations);
  const [notes, setNotes] = React.useState(state.notes);
  const [pending, setPending] = React.useState<{ x: number; y: number } | null>(null);
  const [label, setLabel] = React.useState("");
  const [structure, setStructure] = React.useState("cornea");
  const [color, setColor] = React.useState(colors[0].value);
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => { setAnnotations(state.annotations); setNotes(state.notes); }, [state]);

  const handleClick = (event: React.MouseEvent<SVGSVGElement>) => {
    if (!canEdit) return;
    const rect = event.currentTarget.getBoundingClientRect();
    setPending({
      x: Math.round(((event.clientX - rect.left) / rect.width) * 1000) / 10,
      y: Math.round(((event.clientY - rect.top) / rect.height) * 1000) / 10,
    });
    setLabel("");
  };

  const addMark = () => {
    if (!pending || !label.trim()) return;
    setAnnotations([...annotations, { x: pending.x, y: pending.y, shape: "dot", color, label: label.trim(), structure }]);
    setPending(null);
    setLabel("");
  };

  const save = async () => {
    setSaving(true);
    try {
      const result = await api.put<{ version: number }>(`/encounters/${encounterId}/eye-diagrams/${eye}`, { annotations, notes, version: state.version });
      toast.success(`${title} diagram saved`);
      onSaved();
      return result;
    } catch (reason) {
      if (reason instanceof APIError && reason.isConflict) toast.error("This diagram changed since it was opened. Reload before saving.");
      else toast.error(reason instanceof Error ? reason.message : "Could not save the eye diagram");
    } finally {
      setSaving(false);
    }
  };

  const dirty = JSON.stringify(annotations) !== JSON.stringify(state.annotations) || notes !== state.notes;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{canEdit ? "Click the diagram to mark a finding." : "Read-only findings from this consultation."}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        <svg
          viewBox="0 0 100 100"
          className={cn("w-full rounded-md border bg-zinc-50", canEdit && "cursor-crosshair")}
          onClick={handleClick}
          role="img"
          aria-label={`${title} schematic diagram`}
        >
          <path d="M4,42 Q50,10 96,42 Q50,74 4,42 Z" fill="white" stroke="#27272a" strokeWidth="1.5" />
          <circle cx="50" cy="42" r="19" fill="#aecbdf" stroke="#27272a" strokeWidth="1" />
          <circle cx="50" cy="42" r="8.5" fill="#111827" />
          <circle cx="46" cy="38" r="2.5" fill="white" opacity="0.85" />
          {annotations.map((mark, index) => (
            <g key={index} onClick={(event) => { if (canEdit) { event.stopPropagation(); setAnnotations(annotations.filter((_, i) => i !== index)); } }}>
              <circle cx={mark.x} cy={mark.y} r={2.2} fill={mark.color} stroke="white" strokeWidth="0.6" className={canEdit ? "cursor-pointer" : undefined} />
            </g>
          ))}
          {pending && <circle cx={pending.x} cy={pending.y} r={2.2} fill="none" stroke="#111" strokeDasharray="1,1" strokeWidth="0.6" />}
        </svg>
        {annotations.length > 0 && (
          <ul className="grid gap-1 text-xs">
            {annotations.map((mark, index) => (
              <li key={index} className="flex items-center gap-2">
                <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: mark.color }} />
                <span className="flex-1">{mark.label} <span className="text-zinc-400">· {mark.structure.replaceAll("_", " ")}</span></span>
                {canEdit && (
                  <button type="button" aria-label={`Remove ${mark.label}`} onClick={() => setAnnotations(annotations.filter((_, i) => i !== index))}>
                    <Trash2 className="h-3 w-3 text-zinc-400 hover:text-red-700" />
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
        {pending && canEdit && (
          <div className="grid gap-2 rounded-md border border-dashed p-3">
            <div className="grid gap-2 sm:grid-cols-2">
              <Field label="Finding"><Input autoFocus value={label} onChange={(event) => setLabel(event.target.value)} placeholder="e.g. nasal pterygium" /></Field>
              <Field label="Structure">
                <Select value={structure} onChange={(event) => setStructure(event.target.value)}>
                  {structures.map((item) => <option key={item} value={item}>{item.replaceAll("_", " ")}</option>)}
                </Select>
              </Field>
            </div>
            <div className="flex items-center gap-2">
              {colors.map((swatch) => (
                <button
                  key={swatch.value}
                  type="button"
                  title={swatch.label}
                  onClick={() => setColor(swatch.value)}
                  className={cn("h-6 w-6 rounded-full border-2", color === swatch.value ? "border-black" : "border-transparent")}
                  style={{ backgroundColor: swatch.value }}
                />
              ))}
              <div className="ml-auto flex gap-2">
                <Button type="button" size="sm" variant="ghost" onClick={() => setPending(null)}>Cancel</Button>
                <Button type="button" size="sm" onClick={addMark} disabled={!label.trim()}>Add</Button>
              </div>
            </div>
          </div>
        )}
        <Field label="Notes">
          <Textarea disabled={!canEdit} value={notes} onChange={(event) => setNotes(event.target.value)} />
        </Field>
        {canEdit && (
          <div className="flex justify-end">
            <Button size="sm" disabled={saving || !dirty} onClick={save}>{saving ? "Saving…" : "Save diagram"}</Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
