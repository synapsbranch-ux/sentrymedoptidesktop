import * as React from "react";
import { ChartSaveNotice, useChartSave, type ChartState, type Mark } from "./chart-drafts";
import { ChartHistoryPanel } from "./chart-history";
import { Trash2 } from "lucide-react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { cn } from "../lib";
import { AnteriorBackdrop, FundusBackdrop, fieldCells, fundusClockHour, motilityCells, type ChartCell, type Eye } from "./chart-backdrops";
import { AmslerSurface } from "./vision-surfaces";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Badge, EmptyState, ErrorState, Skeleton } from "./ui/data";
import { Field, Input, Select, Textarea } from "./ui/input";

type ChartType = "anterior" | "fundus" | "field" | "amsler" | "motility";
type Charts = Record<ChartType, Partial<Record<Eye, ChartState>>>;
interface ChartsResponse {
  charts: Charts;
  previousByChart?: Record<string, Record<string, { encounterNumber: string; date: string; chart: ChartState }>>;
  previous: { encounterNumber: string; date: string; charts: Charts } | null;
}

const emptyChart: ChartState = { annotations: [], notes: "", version: 0 };

const structures = ["cornea", "conjunctiva", "iris", "lens", "sclera", "eyelid", "vitreous", "retina", "macula", "optic_nerve", "other"];
const fundusStructures = ["optic_disc", "cup_disc_ratio", "macula", "vessels", "retina", "periphery", "haemorrhage", "exudate", "drusen", "other"];
const colors = [
  { value: "#dc2626", label: "Abnormal" },
  { value: "#d97706", label: "Watch" },
  { value: "#2563eb", label: "Note" },
  { value: "#16a34a", label: "Normal" },
];

interface Grade { value: string; label: string; color: string }
const fieldGrades: Grade[] = [
  { value: "full", label: "Full", color: "#16a34a" },
  { value: "reduced", label: "Reduced", color: "#d97706" },
  { value: "absent", label: "Absent", color: "#dc2626" },
];
const motilityGrades: Grade[] = [
  { value: "0", label: "0 Normal", color: "#16a34a" },
  { value: "-1", label: "−1", color: "#d97706" },
  { value: "-2", label: "−2", color: "#d97706" },
  { value: "-3", label: "−3", color: "#dc2626" },
  { value: "-4", label: "−4", color: "#dc2626" },
  { value: "+1", label: "+1", color: "#2563eb" },
  { value: "+2", label: "+2", color: "#2563eb" },
  { value: "+3", label: "+3", color: "#7c3aed" },
  { value: "+4", label: "+4", color: "#7c3aed" },
];

const chartTabs: { type: ChartType; label: string; description: string }[] = [
  { type: "anterior", label: "Anterior segment", description: "Click the eye to mark a finding on the lids, cornea, iris or lens." },
  { type: "fundus", label: "Fundus", description: "Click the retina to record a finding with automatic clock-hour notation." },
  { type: "field", label: "Confrontation fields", description: "Tap a zone to cycle full, reduced or absent; toggle the grey N−1 overlay." },
  { type: "amsler", label: "Amsler", description: "What the patient traced during the vision test, with the previous visit behind it." },
  { type: "motility", label: "Motility & cover test", description: "Grade the 3×3 positions from −4 underaction to +4 overaction and record cover testing." },
];

export function ClinicalChartsPanel({ encounterId, canEdit }: { encounterId: string; canEdit: boolean }) {
  return <ChartsEditor encounterId={encounterId} canEdit={canEdit} />;
}

function ChartsEditor({ encounterId, canEdit }: { encounterId: string; canEdit: boolean }) {
  const charts = useLoad(() => api.get<ChartsResponse>(`/encounters/${encounterId}/eye-diagrams`), [encounterId]);
  const [chartType, setChartType] = React.useState<ChartType>("anterior");

  if (charts.loading && !charts.data) return <Skeleton className="h-96" />;
  if (charts.error && !charts.data) return <ErrorState message={charts.error.message} retry={charts.reload} />;
  if (!charts.data) return null;
  const current = charts.data.charts;
  const active = chartTabs.find((tab) => tab.type === chartType)!;
  const state = (type: ChartType, eye: Eye) => current[type]?.[eye] ?? emptyChart;
  const prior = (type: ChartType, eye: Eye) => charts.data?.previousByChart?.[type]?.[eye];
  const previousMarks = (type: ChartType, eye: Eye) => prior(type,eye)?.chart.annotations ?? [];

  return (
    <div role="region" aria-label="Clinical charts" className="grid min-w-0 gap-4">
      <div className="flex flex-wrap gap-1 border-b pb-2">
        {chartTabs.map((tab) => (
          <button
            key={tab.type}
            type="button"
            aria-pressed={chartType === tab.type}
            onClick={() => setChartType(tab.type)}
            className={cn(
              "min-h-11 rounded-md px-3 text-sm font-semibold transition-colors sm:min-h-9",
              chartType === tab.type ? "bg-zinc-900 text-white" : "text-zinc-600 hover:bg-zinc-100",
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <p className="-mt-2 text-xs text-zinc-500">{active.description}</p>
      <p className="text-xs text-zinc-500">2D schematic coordinates — not physical measurements. Drafts are retained while switching chart tabs; save before leaving the consultation.</p>
      <ChartHistoryPanel key={chartType} encounterId={encounterId} chartType={chartType} eyes={chartType === "motility" ? ["OU"] : ["OD","OS"]} charts={current[chartType]} canEdit={canEdit} onSaved={charts.reload} />

      {chartType === "anterior" && (
        <div className="grid gap-4 lg:grid-cols-2">
          {(["OD", "OS"] as const).map((eye) => (
            <FreeChartEditor
              key={eye}
              encounterId={encounterId}
              chartType="anterior"
              eye={eye}
              title={eye === "OD" ? "Right eye (OD)" : "Left eye (OS)"}
              structures={structures}
              backdrop={<AnteriorBackdrop />}
              state={state("anterior", eye)}
              canEdit={canEdit}
              onSaved={charts.reload}
            />
          ))}
        </div>
      )}

      {chartType === "fundus" && (
        <div className="grid gap-4 lg:grid-cols-2">
          {(["OD", "OS"] as const).map((eye) => (
            <FreeChartEditor
              key={eye}
              encounterId={encounterId}
              chartType="fundus"
              eye={eye}
              title={eye === "OD" ? "Right fundus (OD)" : "Left fundus (OS)"}
              structures={fundusStructures}
              backdrop={<FundusBackdrop eye={eye} />}
              state={state("fundus", eye)}
              canEdit={canEdit}
              onSaved={charts.reload}
            />
          ))}
        </div>
      )}

      {chartType === "field" && (
        <div className="grid gap-4 lg:grid-cols-2">
          {(["OD", "OS"] as const).map((eye) => (
            <GridChartEditor
              key={eye}
              encounterId={encounterId}
              chartType="field"
              eye={eye}
              title={eye === "OD" ? "Right eye (OD)" : "Left eye (OS)"}
              cells={fieldCells(eye)}
              grades={fieldGrades}
              layout="radial"
              state={state("field", eye)}
              previousMarks={previousMarks("field", eye)}
              previousLabel={prior("field",eye)?.encounterNumber ?? null}
              canEdit={canEdit}
              onSaved={charts.reload}
            />
          ))}
        </div>
      )}

      {chartType === "amsler" && (
        <div className="grid gap-4 lg:grid-cols-2">
          {(["OD", "OS"] as const).map((eye) => (
            <AmslerChartCard
              key={eye}
              title={eye === "OD" ? "Right eye (OD)" : "Left eye (OS)"}
              state={state("amsler", eye)}
              previousMarks={previousMarks("amsler", eye)}
              previousLabel={prior("amsler",eye)?.encounterNumber ?? null}
            />
          ))}
        </div>
      )}

      {chartType === "motility" && (
        <GridChartEditor
          encounterId={encounterId}
          chartType="motility"
          eye="OU"
          title="Both eyes (OU)"
          cells={motilityCells}
          grades={motilityGrades}
          layout="grid"
          state={state("motility", "OU")}
          previousMarks={previousMarks("motility", "OU")}
          previousLabel={prior("motility","OU")?.encounterNumber ?? null}
          canEdit={canEdit}
          showCoverTest
          onSaved={charts.reload}
        />
      )}
    </div>
  );
}

function ChartFooter({ title, notes, setNotes, canEdit, dirty, saving, save }: { title: string; notes: string; setNotes(value: string): void; canEdit: boolean; dirty: boolean; saving: boolean; save(): void }) {
  return (
    <>
      <Field label="Notes">
        <Textarea aria-label={`${title} notes`} disabled={!canEdit} value={notes} onChange={(event) => setNotes(event.target.value)} />
      </Field>
      {canEdit && (
        <div className="flex justify-end">
          <Button size="sm" disabled={saving || !dirty} onClick={save}>{saving ? "Saving…" : "Save chart"}</Button>
        </div>
      )}
    </>
  );
}

function FreeChartEditor({ encounterId, chartType, eye, title, structures: available, backdrop, state, canEdit, onSaved }: {
  encounterId: string;
  chartType: ChartType;
  eye: Eye;
  title: string;
  structures: string[];
  backdrop: React.ReactNode;
  state: ChartState;
  canEdit: boolean;
  onSaved(): void;
}) {
  const editor = useChartSave(encounterId, chartType, eye, state, onSaved);
  const { annotations, setAnnotations, notes, setNotes, saving, dirty, save } = editor;
  const [pending, setPending] = React.useState<{ x: number; y: number } | null>(null);
  const [label, setLabel] = React.useState("");
  const [structure, setStructure] = React.useState(available[0]);
  const [color, setColor] = React.useState(colors[0].value);

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
    setAnnotations([...annotations, { x: pending.x, y: pending.y, shape: "dot", color, label: label.trim(), structure, ...(chartType === "fundus" ? { clockHour: fundusClockHour(pending.x, pending.y) } : {}) }]);
    setPending(null);
    setLabel("");
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{canEdit ? "Click the diagram to mark a finding." : "Read-only findings from this consultation."}</CardDescription>
      </CardHeader>
      <CardContent className="grid min-w-0 gap-3">
        <svg
          viewBox="0 0 100 100"
          className={cn("min-w-0 w-full rounded-md border bg-zinc-50", canEdit && "cursor-crosshair")}
          onClick={handleClick}
          role="img"
          aria-label={`${title} schematic diagram`}
        >
          {backdrop}
          {annotations.map((mark, index) => (
            <circle
              key={index}
              cx={mark.x}
              cy={mark.y}
              r={2.2}
              fill={mark.color}
              stroke="white"
              strokeWidth="0.6"
              className={canEdit ? "cursor-pointer" : undefined}
              onClick={(event) => { if (canEdit) { event.stopPropagation(); setAnnotations(annotations.filter((_, i) => i !== index)); } }}
            />
          ))}
          {pending && <circle cx={pending.x} cy={pending.y} r={2.2} fill="none" stroke="#111" strokeDasharray="1,1" strokeWidth="0.6" />}
        </svg>
        {annotations.length > 0 && (
          <ul className="grid gap-1 text-xs">
            {annotations.map((mark, index) => (
              <li key={index} className="flex items-center gap-2">
                <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: mark.color }} />
                <span className="flex-1">{mark.label} <span className="text-zinc-400">· {mark.structure.replaceAll("_", " ")}{chartType === "fundus" && ` · ${mark.clockHour ?? fundusClockHour(mark.x, mark.y)} o'clock`}</span></span>
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
            {chartType === "fundus" && <Badge>Clock hour: {fundusClockHour(pending.x, pending.y)}</Badge>}
            <div className="grid gap-2 sm:grid-cols-2">
              <Field label="Finding"><Input autoFocus value={label} onChange={(event) => setLabel(event.target.value)} placeholder="e.g. nasal pterygium" /></Field>
              <Field label="Structure">
                <Select value={structure} onChange={(event) => setStructure(event.target.value)}>
                  {available.map((item) => <option key={item} value={item}>{item.replaceAll("_", " ")}</option>)}
                </Select>
              </Field>
            </div>
            <div className="flex items-center gap-2">
              {colors.map((swatch) => (
                <button
                  key={swatch.value}
                  type="button"
                  title={swatch.label}
                  aria-label={swatch.label}
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
        <ChartSaveNotice editor={editor} canEdit={canEdit} />
        <ChartFooter title={title} notes={notes} setNotes={setNotes} canEdit={canEdit} dirty={dirty && !editor.remote} saving={saving} save={save} />
      </CardContent>
    </Card>
  );
}

function GridChartEditor({ encounterId, chartType, eye, title, cells, grades, layout, state, previousMarks, previousLabel, canEdit, showCoverTest = false, onSaved }: {
  encounterId: string;
  chartType: ChartType;
  eye: Eye;
  title: string;
  cells: ChartCell[];
  grades: Grade[];
  layout: "radial" | "grid";
  state: ChartState;
  previousMarks: Mark[];
  previousLabel: string | null;
  canEdit: boolean;
  showCoverTest?: boolean;
  onSaved(): void;
}) {
  const editor = useChartSave(encounterId, chartType, eye, state, onSaved);
  const { annotations, setAnnotations, notes, setNotes, saving, dirty, save } = editor;
  const [focused, setFocused] = React.useState<string | null>(null);
  const [showPrevious, setShowPrevious] = React.useState(true);
  const marked = new Map(annotations.map((mark) => [mark.cell ?? "", mark]));
  const before = new Map(previousMarks.map((mark) => [mark.cell ?? "", mark]));
  const gradeOf = (value?: string) => grades.find((grade) => grade.value === value);

  const setCoverValue = (cell: string, grade: string) => {
    const existing = marked.get(cell);
    if (!grade) { setAnnotations(annotations.filter((mark) => mark.cell !== cell)); return; }
    const updated: Mark = { ...existing, x: 50, y: 50, shape: "field", color: "#52525b", label: existing?.label ?? "", structure: "cover_test", cell, grade };
    setAnnotations(existing ? annotations.map((mark) => mark.cell === cell ? updated : mark) : [...annotations, updated]);
  };

  /** Tapping a zone walks the grade list and then clears it, so one control records and undoes. */
  const cycle = (cell: ChartCell) => {
    if (!canEdit) return;
    const existing = marked.get(cell.id);
    const position = existing ? grades.findIndex((grade) => grade.value === existing.grade) : -1;
    const next = grades[position + 1];
    if (!next) {
      setAnnotations(annotations.filter((mark) => mark.cell !== cell.id));
      return;
    }
    const updated: Mark = { ...existing, x: cell.cx, y: cell.cy, shape: "cell", color: next.color, label: existing?.label ?? "", structure: chartType, cell: cell.id, grade: next.value };
    setAnnotations(existing ? annotations.map((mark) => (mark.cell === cell.id ? updated : mark)) : [...annotations, updated]);
  };

  const setCellNote = (cellId: string, value: string) => {
    setAnnotations(annotations.map((mark) => (mark.cell === cellId ? { ...mark, label: value } : mark)));
  };

  const cellTitle = (cell: ChartCell) => {
    const grade = gradeOf(marked.get(cell.id)?.grade);
    const priorGrade = gradeOf(before.get(cell.id)?.grade);
    const parts = [cell.label, grade ? grade.label : "not tested"];
    if (priorGrade) parts.push(`previously ${priorGrade.label}`);
    return parts.join(" — ");
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>
          {canEdit ? "Tap a zone to cycle its grade; tapping past the last grade clears it." : "Read-only grading from this consultation."}
          {previousMarks.length > 0 && previousLabel && ` Where the grade moved, ${previousLabel} is shown alongside it.`}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {previousMarks.length > 0 && previousLabel && <label className="flex items-center gap-2 text-xs font-semibold text-zinc-600"><input type="checkbox" checked={showPrevious} onChange={(event) => setShowPrevious(event.target.checked)} />Show {previousLabel} in grey</label>}
        {layout === "radial" ? (
          <div className="grid grid-cols-[auto_1fr_auto] items-center gap-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">
            <span />
            <span className="text-center">superior</span>
            <span />
            <span className="[writing-mode:vertical-rl] rotate-180 text-center">{eye === "OD" ? "nasal" : "temporal"}</span>
            <svg viewBox="0 0 100 100" className="w-full rounded-md border bg-zinc-50" role="group" aria-label={`${title} confrontation field`}>
            <circle cx="50" cy="50" r="44" fill="white" stroke="#d4d4d8" strokeWidth="0.6" />
            {cells.map((cell) => {
              const grade = gradeOf(marked.get(cell.id)?.grade);
              const priorGrade = gradeOf(before.get(cell.id)?.grade);
              const priorShape = showPrevious && priorGrade && (cell.path
                ? <path d={cell.path} fill="#94a3b8" fillOpacity="0.28" stroke="#64748b" strokeWidth="0.7" strokeDasharray="1.5,1" />
                : <circle cx={cell.cx} cy={cell.cy} r="13" fill="#94a3b8" fillOpacity="0.28" stroke="#64748b" strokeWidth="0.7" strokeDasharray="1.5,1" />);
              const shape = cell.path
                ? <path d={cell.path} fill={grade?.color ?? "#ffffff"} fillOpacity={grade ? 0.35 : showPrevious && priorGrade ? 0 : 1} stroke="#52525b" strokeWidth="0.5" />
                : <circle cx={cell.cx} cy={cell.cy} r="13" fill={grade?.color ?? "#ffffff"} fillOpacity={grade ? 0.35 : showPrevious && priorGrade ? 0 : 1} stroke="#52525b" strokeWidth="0.5" />;
              return (
                <g
                  key={cell.id}
                  role="button"
                  tabIndex={canEdit ? 0 : -1}
                  aria-label={cellTitle(cell)}
                  className={cn("outline-none", canEdit && "cursor-pointer")}
                  onClick={() => cycle(cell)}
                  onFocus={() => setFocused(cell.id)}
                  onBlur={() => setFocused(null)}
                  onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); cycle(cell); } }}
                >
                  {priorShape}
                  {shape}
                  {focused === cell.id && (cell.path
                    ? <path d={cell.path} fill="none" stroke="#111827" strokeWidth="1.4" />
                    : <circle cx={cell.cx} cy={cell.cy} r="13" fill="none" stroke="#111827" strokeWidth="1.4" />)}
                  <title>{cellTitle(cell)}</title>
                  {grade && <text x={cell.cx} y={cell.cy + 1.6} textAnchor="middle" fontSize="5" fontWeight="700" fill={grade.color}>{grade.label}</text>}
                  {showPrevious && priorGrade && priorGrade.value !== marked.get(cell.id)?.grade && (
                    <circle cx={cell.cx} cy={cell.cy - 6} r="1.8" fill="none" stroke={priorGrade.color} strokeWidth="0.8" strokeDasharray="1,0.8" />
                  )}
                </g>
              );
            })}
            </svg>
            <span className="[writing-mode:vertical-rl] text-center">{eye === "OD" ? "temporal" : "nasal"}</span>
            <span />
            <span className="text-center">inferior</span>
            <span />
          </div>
        ) : (
          <div className="grid grid-cols-3 gap-2" role="group" aria-label={`${title} positions of gaze`}>
            {cells.map((cell) => {
              const grade = gradeOf(marked.get(cell.id)?.grade);
              const priorGrade = gradeOf(before.get(cell.id)?.grade);
              return (
                <button
                  key={cell.id}
                  type="button"
                  disabled={!canEdit}
                  aria-label={cellTitle(cell)}
                  onClick={() => cycle(cell)}
                  className={cn(
                    "grid min-h-20 place-items-center gap-0.5 rounded-md border p-2 text-center transition-colors",
                    grade ? "border-transparent" : "border-zinc-200 bg-white",
                    canEdit && "hover:border-zinc-400",
                  )}
                  style={grade ? { backgroundColor: `${grade.color}22`, borderColor: grade.color } : undefined}
                >
                  <span className="text-lg font-bold" style={{ color: grade?.color ?? "#a1a1aa" }}>{grade ? grade.label : "·"}</span>
                  <span className="text-[10px] font-semibold leading-tight text-zinc-600">{cell.label}</span>
                  {cell.hint && <span className="font-mono text-[9px] text-zinc-400">{cell.hint}</span>}
                  {showPrevious && priorGrade && priorGrade.value !== marked.get(cell.id)?.grade && (
                    <span className="text-[9px] text-zinc-400">was {priorGrade.label}</span>
                  )}
                </button>
              );
            })}
          </div>
        )}
        <div className="flex flex-wrap items-center gap-3 text-[11px] text-zinc-500">
          {grades.map((grade) => (
            <span key={grade.value} className="flex items-center gap-1">
              <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: grade.color }} />{grade.label}
            </span>
          ))}
        </div>
        {showCoverTest && (
          <div className="grid gap-3 rounded-md border p-4"><div><div className="font-semibold">Cover test</div><p className="text-xs text-zinc-500">Structured binocular alignment at distance and near.</p></div><div className="grid gap-3 sm:grid-cols-3">
            <Field label="Method"><Select disabled={!canEdit} value={marked.get("cover-method")?.grade ?? ""} onChange={(event) => setCoverValue("cover-method", event.target.value)}><option value="">Not recorded</option><option value="cover-uncover">Cover–uncover</option><option value="alternate-cover">Alternate cover</option></Select></Field>
            {(["distance", "near"] as const).map((distance) => <Field key={distance} label={distance === "distance" ? "Distance alignment" : "Near alignment"}><Select disabled={!canEdit} value={marked.get(`cover-${distance}`)?.grade ?? ""} onChange={(event) => setCoverValue(`cover-${distance}`, event.target.value)}><option value="">Not recorded</option><option value="ortho">Orthophoria</option><option value="exophoria">Exophoria</option><option value="esophoria">Esophoria</option><option value="exotropia">Exotropia</option><option value="esotropia">Esotropia</option><option value="vertical-deviation">Vertical deviation</option><option value="not-tested">Not tested</option></Select></Field>)}
          </div></div>
        )}
        {annotations.length > 0 && (
          <ul className="grid gap-2">
            {annotations.map((mark) => {
              const cell = cells.find((candidate) => candidate.id === mark.cell);
              if (!cell) return null;
              return (
                <li key={mark.cell} className="grid gap-1">
                  <Field label={`${cell.label} — ${gradeOf(mark.grade)?.label ?? ""}`}>
                    <Input disabled={!canEdit} value={mark.label} placeholder="Optional detail" onChange={(event) => setCellNote(cell.id, event.target.value)} />
                  </Field>
                </li>
              );
            })}
          </ul>
        )}
        <ChartSaveNotice editor={editor} canEdit={canEdit} />
        <ChartFooter title={title} notes={notes} setNotes={setNotes} canEdit={canEdit} dirty={dirty && !editor.remote} saving={saving} save={save} />
      </CardContent>
    </Card>
  );
}


/**
 * The Amsler grid is drawn by the patient during the vision test and committed to the
 * record from there, so it is read-only here. The previous visit sits behind it in a
 * paler tone: a scotoma that grew is the finding, not the marks on their own.
 */
function AmslerChartCard({ title, state, previousMarks, previousLabel }: {
  title: string;
  state: ChartState;
  previousMarks: Mark[];
  previousLabel: string | null;
}) {
  const combined = [
    ...previousMarks.map((mark) => ({ ...mark, color: "#94a3b8" })),
    ...state.annotations,
  ];
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>
          {state.version === 0
            ? "Not recorded for this visit."
            : state.annotations.length === 0
              ? "Recorded as normal — nothing marked."
              : `${state.annotations.length} areas marked.`}
          {previousMarks.length > 0 && previousLabel && ` Grey marks are ${previousLabel}.`}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid justify-items-center gap-3">
        {state.version === 0 && previousMarks.length === 0 ? (
          <EmptyState title="No Amsler grid" description="Run an Amsler test from Vision testing and save it to this consultation." />
        ) : (
          <>
            <AmslerSurface marks={combined} sizePx={320} readOnly inverted={false} />
            {state.notes && <p className="text-sm text-zinc-600">{state.notes}</p>}
          </>
        )}
      </CardContent>
    </Card>
  );
}
