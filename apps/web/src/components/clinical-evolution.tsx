import * as React from "react";
import { AlertTriangle, ArrowRight, Info, TriangleAlert } from "lucide-react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { cn } from "../lib";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Badge, EmptyState, ErrorState, Skeleton } from "./ui/data";

interface EyeMeasurement { od: number | null; os: number | null; odLabel?: string; osLabel?: string }
interface RefractionPoint {
  source: string;
  odSphere: number | null; osSphere: number | null;
  odCylinder: number | null; osCylinder: number | null;
  odAxis: number | null; osAxis: number | null;
  odSphericalEquivalent: number | null; osSphericalEquivalent: number | null;
}
interface TrendPoint {
  encounterId: string;
  encounterNumber: string;
  date: string;
  refraction?: RefractionPoint;
  refractions?: Record<string, RefractionPoint>;
  visualAcuity?: EyeMeasurement;
  iop?: EyeMeasurement;
  pachymetry?: EyeMeasurement;
}
interface ClinicalAlert { severity: "info" | "warning" | "danger"; eye?: string; title: string; detail: string }
interface CorrectedIOP { measured?: number; pachymetry?: number; estimated?: number; interpretation: string }
interface TrendSummary extends Record<string, unknown> { iopContext?: { od?: CorrectedIOP; os?: CorrectedIOP; correctionEnabled: boolean } }
interface TrendsResponse { points: TrendPoint[]; alerts: ClinicalAlert[]; summary: TrendSummary; thresholds: { elevatedIOPMmHg: number } }
interface VisitChange { category: string; changeType: "added" | "removed" | "modified"; eye?: string; before: string; after: string; delta: string; severity: "info" | "warning" | "danger" }
interface DeltaResponse {
  hasPrevious: boolean;
  previous?: { encounterNumber: string; date: string };
  current?: { encounterNumber: string; date: string };
  changes: VisitChange[];
}

export function refractionForSource(point: Pick<TrendPoint, "refractions">, source: "subjective" | "prescription" | "autorefraction") {
  return point.refractions?.[source];
}

const severityTone: Record<ClinicalAlert["severity"], "neutral" | "warning" | "danger"> = { info: "neutral", warning: "warning", danger: "danger" };

function shortDate(value: string) {
  return new Date(value).toLocaleDateString(undefined, { year: "2-digit", month: "short", day: "numeric" });
}

/**
 * A compact two-series line chart drawn as inline SVG, matching the app's
 * dependency-free charting approach. `invert` flips the vertical axis for logMAR, where a
 * higher number means worse vision, so "down" always reads as "worse" on every chart here.
 */
function TrendChart({ title, description, points, unit, invert = false, threshold }: {
  title: string;
  description: string;
  points: { date: string; od: number | null; os: number | null }[];
  unit: string;
  invert?: boolean;
  threshold?: number;
}) {
  const usable = points.filter((point) => point.od !== null || point.os !== null);
  if (usable.length === 0) {
    return (
      <Card>
        <CardHeader><CardTitle>{title}</CardTitle><CardDescription>{description}</CardDescription></CardHeader>
        <CardContent><EmptyState title="Not recorded yet" description="Values entered during pre-test and examination appear here." /></CardContent>
      </Card>
    );
  }
  const values = usable.flatMap((point) => [point.od, point.os]).filter((value): value is number => value !== null);
  const candidates = threshold === undefined ? values : [...values, threshold];
  const minimum = Math.min(...candidates);
  const maximum = Math.max(...candidates);
  const span = maximum - minimum || 1;
  const width = 100;
  const height = 42;
  const x = (index: number) => (usable.length === 1 ? width / 2 : (index / (usable.length - 1)) * width);
  const y = (value: number) => {
    const ratio = (value - minimum) / span;
    return invert ? 4 + ratio * (height - 8) : height - 4 - ratio * (height - 8);
  };
  const series = (eye: "od" | "os") =>
    usable
      .map((point, index) => ({ point: point[eye], index }))
      .filter((entry): entry is { point: number; index: number } => entry.point !== null);

  return (
    <Card>
      <CardHeader><CardTitle>{title}</CardTitle><CardDescription>{description}</CardDescription></CardHeader>
      <CardContent>
        <svg viewBox={`0 0 ${width} ${height}`} className="w-full" role="img" aria-label={title} preserveAspectRatio="none" style={{ height: 160 }}>
          {threshold !== undefined && (
            <line x1={0} x2={width} y1={y(threshold)} y2={y(threshold)} stroke="#dc2626" strokeWidth={0.4} strokeDasharray="2,1.5" />
          )}
          {(["od", "os"] as const).map((eye) => {
            const entries = series(eye);
            if (entries.length === 0) return null;
            const color = eye === "od" ? "#111827" : "#2563eb";
            const path = entries.map((entry, position) => `${position === 0 ? "M" : "L"}${x(entry.index)},${y(entry.point)}`).join(" ");
            return (
              <g key={eye}>
                {entries.length > 1 && <path d={path} fill="none" stroke={color} strokeWidth={0.8} />}
                {entries.map((entry) => (
                  <circle key={entry.index} cx={x(entry.index)} cy={y(entry.point)} r={1.2} fill={color}>
                    <title>{`${eye.toUpperCase()} · ${shortDate(usable[entry.index].date)} · ${entry.point} ${unit}`}</title>
                  </circle>
                ))}
              </g>
            );
          })}
        </svg>
        <div className="mt-3 flex items-center justify-between text-[11px] text-zinc-500">
          <span className="flex items-center gap-3">
            <span className="flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-zinc-900" />OD</span>
            <span className="flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-blue-600" />OS</span>
            {threshold !== undefined && <span className="text-red-700">— seuil {threshold} {unit}</span>}
          </span>
          <span className="font-mono">{shortDate(usable[0].date)} → {shortDate(usable[usable.length - 1].date)}</span>
        </div>
      </CardContent>
    </Card>
  );
}

export function ClinicalEvolution({ encounterId, patientId }: { encounterId: string; patientId: string }) {
  const trends = useLoad(() => api.get<TrendsResponse>(`/patients/${patientId}/clinical-trends`), [patientId]);
  const delta = useLoad(() => api.get<DeltaResponse>(`/encounters/${encounterId}/delta`), [encounterId]);

  if (trends.loading || delta.loading) return <Skeleton className="h-96" />;
  if (trends.error) return <ErrorState message={trends.error.message} retry={trends.reload} />;
  const points = trends.data?.points ?? [];
  const alerts = trends.data?.alerts ?? [];
  const summary = trends.data?.summary ?? {};
  const [refractionSource, setRefractionSource] = React.useState<"subjective" | "prescription" | "autorefraction">("subjective");
  const selectedRefraction = (point: TrendPoint) => refractionForSource(point, refractionSource);

  return (
    <div className="grid gap-4">
      {alerts.length > 0 && (
        <Card className="border-amber-300">
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><TriangleAlert className="h-4 w-4 text-amber-600" />Signals to review</CardTitle>
            <CardDescription>Derived from values already recorded — confirm clinically before acting.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2">
            {alerts.map((alert, index) => (
              <div key={index} className={cn("flex items-start gap-3 rounded-md border p-3 text-sm", alert.severity === "danger" ? "border-red-300 bg-red-50" : alert.severity === "warning" ? "border-amber-300 bg-amber-50" : "border-zinc-200")}>
                {alert.severity === "info" ? <Info className="mt-0.5 h-4 w-4 shrink-0 text-zinc-400" /> : <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" />}
                <div>
                  <div className="font-semibold">{alert.title}{alert.eye && <Badge className="ml-2" tone={severityTone[alert.severity]}>{alert.eye}</Badge>}</div>
                  <div className="text-zinc-600">{alert.detail}</div>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Since the last visit</CardTitle>
          <CardDescription>
            {delta.data?.hasPrevious && delta.data.previous
              ? `Compared with ${delta.data.previous.encounterNumber} on ${shortDate(delta.data.previous.date)}`
              : "This is the first recorded consultation for this patient."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {delta.data?.hasPrevious ? (
            delta.data.changes.length > 0 ? (
              <div className="grid gap-2">
                {delta.data.changes.map((change, index) => (
                  <div key={index} className={cn("flex flex-wrap items-center gap-3 rounded-md border p-3 text-sm", change.severity === "danger" ? "border-red-300 bg-red-50" : change.severity === "warning" ? "border-amber-300 bg-amber-50" : "border-zinc-200")}>
                    <span className="font-semibold">{change.category}</span>
                    <Badge tone={change.changeType === "removed" ? "warning" : "neutral"}>{change.changeType}</Badge>
                    {change.eye && <Badge>{change.eye}</Badge>}
                    <span className="flex items-center gap-2 font-mono text-xs text-zinc-500">
                      {change.before} <ArrowRight className="h-3 w-3" /> <strong className="text-[var(--foreground)]">{change.after}</strong>
                    </span>
                    <span className="ml-auto font-mono text-xs font-bold">{change.delta}</span>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyState title="No measurable change" description="Acuity, pressure and refraction are unchanged since the previous visit." />
            )
          ) : (
            <EmptyState title="No previous visit" description="Trends will build up from the next consultation onwards." />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-start justify-between gap-4">
          <div><CardTitle>Refractive progression</CardTitle><CardDescription>Sphere, cylinder and spherical equivalent are never mixed across measurement sources.</CardDescription></div>
          <label className="grid gap-1 text-xs font-semibold">Source
            <select className="h-9 rounded-md border bg-[var(--background)] px-3 text-sm" value={refractionSource} onChange={(event) => setRefractionSource(event.target.value as typeof refractionSource)}>
              <option value="subjective">Subjective</option><option value="prescription">Final prescription</option><option value="autorefraction">Autorefraction</option>
            </select>
          </label>
        </CardHeader>
        <CardContent className="grid gap-4 xl:grid-cols-3">
          <TrendChart title="Sphere" description={`${refractionSource} sphere`} unit="D" points={points.map((point) => ({ date: point.date, od: selectedRefraction(point)?.odSphere ?? null, os: selectedRefraction(point)?.osSphere ?? null }))} />
          <TrendChart title="Cylinder" description={`${refractionSource} cylinder`} unit="D" points={points.map((point) => ({ date: point.date, od: selectedRefraction(point)?.odCylinder ?? null, os: selectedRefraction(point)?.osCylinder ?? null }))} />
          <TrendChart title="Spherical equivalent" description={`${refractionSource}; sphere + cylinder ÷ 2`} unit="D" points={points.map((point) => ({ date: point.date, od: selectedRefraction(point)?.odSphericalEquivalent ?? null, os: selectedRefraction(point)?.osSphericalEquivalent ?? null }))} />
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <TrendChart
          title="Visual acuity (logMAR)"
          description="Lower is better; one line equals 0.10 logMAR."
          unit="logMAR"
          invert
          points={points.map((point) => ({ date: point.date, od: point.visualAcuity?.od ?? null, os: point.visualAcuity?.os ?? null }))}
        />
        <TrendChart
          title="Intraocular pressure"
          description={summary.iopPeakOD !== undefined ? `Peak on record OD: ${summary.iopPeakOD} mmHg.` : "Tonometry per eye."}
          unit="mmHg"
          threshold={trends.data?.thresholds.elevatedIOPMmHg ?? 21}
          points={points.map((point) => ({ date: point.date, od: point.iop?.od ?? null, os: point.iop?.os ?? null }))}
        />
        <TrendChart
          title="Central corneal thickness"
          description="A thin cornea makes tonometry underestimate the true pressure."
          unit="µm"
          points={points.map((point) => ({ date: point.date, od: point.pachymetry?.od ?? null, os: point.pachymetry?.os ?? null }))}
        />
      </div>
      {summary.iopContext && (
        <Card><CardHeader><CardTitle>IOP interpreted with pachymetry</CardTitle><CardDescription>Measured values remain authoritative. A numeric estimate appears only when a doctor configured and enabled a coefficient.</CardDescription></CardHeader><CardContent className="grid gap-3 md:grid-cols-2">{(["od", "os"] as const).map((eye) => {
          const value = summary.iopContext?.[eye];
          return <div key={eye} className="rounded-md border p-4"><div className="font-semibold uppercase">{eye}</div>{value ? <><div className="mt-2 font-mono text-sm">Measured: {value.measured} mmHg · CCT: {value.pachymetry ?? "—"} µm</div>{value.estimated !== undefined && <div className="mt-1 font-mono text-sm font-bold">Configured estimate: {value.estimated} mmHg</div>}<p className="mt-2 text-xs text-zinc-600">{value.interpretation}</p></> : <p className="mt-2 text-sm text-zinc-500">No IOP recorded.</p>}</div>;
        })}</CardContent></Card>
      )}
    </div>
  );
}
