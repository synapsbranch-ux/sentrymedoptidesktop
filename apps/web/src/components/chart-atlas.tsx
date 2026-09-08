import * as React from "react";
import { anatomyRegions, anatomyVersion, initialAtlasView, type AnatomyRegion } from "./chart-anatomy";
import { ChartSection } from "./chart-section";
import type { Eye } from "./chart-backdrops";
import { Button } from "./ui/button";

const ChartEye3D = React.lazy(() => import("./chart-eye-3d"));

class AtlasBoundary extends React.Component<{ children: React.ReactNode; onUnavailable(): void }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  componentDidCatch() { this.props.onUnavailable(); }
  render() { return this.state.failed ? null : this.props.children; }
}

export function ChartAtlas({ eye, mode, region, onExplore, onUnavailable }: {
  eye: Eye; mode: "2.5D" | "3D"; region: AnatomyRegion | null;
  onExplore(region: AnatomyRegion): void; onUnavailable(): void;
}) {
  const [view, setView] = React.useState(initialAtlasView);
  const [cutaway, setCutaway] = React.useState(true);
  const [isolate, setIsolate] = React.useState(false);
  const regionName = anatomyRegions.find(item => item.id === region)?.label;
  return <div className="grid min-w-0 gap-3" aria-label={`${eye} anatomy explorer`} data-anatomy-model={anatomyVersion}>
    <p className="text-xs text-slate-600">Generic anatomy for orientation and patient explanation. The highlighted structure is not the location, size or depth of a lesion. Use 2D for exact chart positions.</p>
    {mode === "2.5D" ? <>
      <ChartSection eye={eye} region={region} onExplore={onExplore} />
      {(region === "macula" || region === "optic_disc") && <p className="text-xs">This section does not locate the {regionName?.toLowerCase()}. Use the fundus chart or the 3D structure view.</p>}
    </> : <>
      <AtlasBoundary onUnavailable={onUnavailable}><React.Suspense fallback={<p role="status">Loading 3D view…</p>}>
        <ChartEye3D eye={eye} region={region} view={view} cutaway={cutaway} isolate={isolate} onExplore={onExplore} onViewChange={setView} onUnavailable={onUnavailable} />
      </React.Suspense></AtlasBoundary>
      <p className="text-xs text-slate-500">Drag with a mouse to rotate. On a phone, use the controls below. The cornea is anterior; Reset view restores the initial orientation.</p>
      <div className="grid min-w-0 gap-2 rounded border p-3 text-xs">
        <label className="grid gap-1">Rotation <input aria-label={`${eye} model rotation`} type="range" min="-180" max="180" value={view.yaw} onChange={event => setView({ ...view, yaw: Number(event.target.value) })} /></label>
        <label className="grid gap-1">Tilt <input aria-label={`${eye} model tilt`} type="range" min="-70" max="70" value={view.pitch} onChange={event => setView({ ...view, pitch: Number(event.target.value) })} /></label>
        <label className="grid gap-1">Zoom <input aria-label={`${eye} model zoom`} type="range" min="0.7" max="1.7" step="0.1" value={view.zoom} onChange={event => setView({ ...view, zoom: Number(event.target.value) })} /></label>
        <div className="flex flex-wrap gap-3">
          <label className="flex min-h-11 items-center gap-2"><input type="checkbox" checked={cutaway} onChange={event => setCutaway(event.target.checked)} />Cutaway</label>
          <label className="flex min-h-11 items-center gap-2"><input type="checkbox" checked={isolate} disabled={!region} onChange={event => setIsolate(event.target.checked)} />Isolate selected structure</label>
        </div>
        <Button type="button" size="sm" variant="outline" onClick={() => { setView(initialAtlasView); setCutaway(true); setIsolate(false); }}>Reset view</Button>
      </div>
    </>}
    <div className="flex flex-wrap gap-1" role="group" aria-label={`${eye} anatomical structures`}>
      {anatomyRegions.map(item => <button key={item.id} type="button" aria-pressed={region === item.id} onClick={() => onExplore(item.id)} className={`min-h-11 rounded border px-3 text-xs ${region === item.id ? "border-blue-700 bg-blue-50 font-bold text-blue-900" : "bg-white"}`}>
        <span className="mr-1 inline-block h-2 w-2 rounded-full" style={{ backgroundColor: item.color }} />{item.label}
      </button>)}
    </div>
    <p role="status" className="text-xs font-semibold">{regionName ? `Structure: ${regionName}` : "Select an anatomical structure or a finding with a recorded structure."}</p>
  </div>;
}
