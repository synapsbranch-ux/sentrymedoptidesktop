import * as React from "react";
import { api, APIError } from "../api";

export interface Mark {
  id?: string; trackingId?: string; coordinateSystem?: string;
  x: number; y: number; shape: string; color: string; label: string; structure: string;
  cell?: string; grade?: string; clockHour?: number;
}
export type ExamStatus = "unspecified" | "not_examined" | "no_findings" | "findings";
export interface ChartState { annotations: Mark[]; notes: string; version: number; examStatus?: ExamStatus; schemaVersion?: number }
export interface Draft { base: ChartState; value: ChartState; remote?: ChartState; saving: boolean; error: string }
// getRandomValues also works on LAN HTTP clients where randomUUID is unavailable.
export function chartMarkID() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2,"0")).join("");
  return `${hex.slice(0,8)}-${hex.slice(8,12)}-${hex.slice(12,16)}-${hex.slice(16,20)}-${hex.slice(20)}`;
}
export function chartDirty(draft: Draft) {
  return JSON.stringify([draft.value.annotations,draft.value.notes,draft.value.examStatus ?? "unspecified"]) !== JSON.stringify([draft.base.annotations,draft.base.notes,draft.base.examStatus ?? "unspecified"]);
}
export function receiveChart(draft: Draft, remote: ChartState): Draft {
  if (remote.version <= draft.base.version) return draft;
  if (chartDirty(draft) || draft.saving) return { ...draft, remote };
  return { base: remote, value: remote, saving: false, error: "" };
}
class DraftStore {
  drafts = new Map<string,Draft>();
  listeners = new Set<() => void>();
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  set(key: string, draft: Draft) { this.drafts.set(key,draft); this.listeners.forEach(listener => listener()); }
}
const Context = React.createContext<DraftStore | null>(null);
export function ChartDraftProvider({ children, onDirtyChange }: { children: React.ReactNode; onDirtyChange?(dirty: boolean): void }) {
  const [store] = React.useState(() => new DraftStore());
  React.useEffect(() => {
    const notify = () => onDirtyChange?.([...store.drafts.values()].some(chartDirty));
    notify();
    const unsubscribe = store.subscribe(notify);
    return () => { unsubscribe(); onDirtyChange?.(false); };
  },[store,onDirtyChange]);
  React.useEffect(() => {
    const leaving = (event: BeforeUnloadEvent) => {
      if ([...store.drafts.values()].some(chartDirty)) { event.preventDefault(); event.returnValue = ""; }
    };
    window.addEventListener("beforeunload",leaving);
    return () => window.removeEventListener("beforeunload",leaving);
  },[store]);
  return <Context.Provider value={store}>{children}</Context.Provider>;
}

export function useChartSave(encounterId: string, chartType: string, eye: string, state: ChartState, onSaved: () => void) {
  const store = React.useContext(Context);
  if (!store) throw new Error("Chart editor needs its consultation draft provider");
  const key = `${chartType}/${eye}`;
  if (!store.drafts.has(key)) store.drafts.set(key,{ base: state, value: state, saving: false, error: "" });
  const draft = React.useSyncExternalStore(store.subscribe,() => store.drafts.get(key)!);
  React.useEffect(() => {
    const current = store.drafts.get(key)!;
    const next = receiveChart(current,state);
    if (next !== current) store.set(key,next);
  },[store,key,state]);
  const update = (patch: Partial<ChartState>) => {
    const current = store.drafts.get(key)!;
    store.set(key,{ ...current, value: { ...current.value,...patch } });
  };
  const save = async () => {
    const sent = store.drafts.get(key)!;
    if (sent.saving || sent.remote) return;
    store.set(key,{ ...sent,saving:true,error:"" });
    try {
      const result = await api.put<{ version:number; annotations:Mark[] }>(`/encounters/${encounterId}/eye-diagrams/${chartType}/${eye}`,{ ...sent.value,version:sent.base.version,schemaVersion:2 });
      const saved = { ...sent.value,version:result.version,annotations:result.annotations };
      const current = store.drafts.get(key)!;
      const changedWhileSaving = JSON.stringify(current.value) !== JSON.stringify(sent.value);
      store.set(key,{ base:saved,value:changedWhileSaving ? current.value : saved,saving:false,error:"" });
      onSaved();
    } catch (reason) {
      let remote: ChartState | undefined;
      if (reason instanceof APIError && reason.isConflict) {
        try {
          const latest = await api.get<{ charts:Record<string,Record<string,ChartState>> }>(`/encounters/${encounterId}/eye-diagrams`);
          remote=latest.charts[chartType]?.[eye];
        } catch { /* Keep the draft and its original version when refresh also fails. */ }
      }
      store.set(key,{ ...store.drafts.get(key)!,saving:false,remote,error:reason instanceof Error ? reason.message : "Could not save chart" });
    }
  };
  const resolve = (keep: boolean) => {
    const current=store.drafts.get(key)!;
    if (!current.remote) return;
    if (!window.confirm(keep ? "Use your reviewed draft instead of the server version on the next save?" : "Discard your draft and load the server version?")) return;
    store.set(key,{ base:current.remote,value:keep ? current.value : current.remote,saving:false,error:"" });
  };
  return { annotations:draft.value.annotations,setAnnotations:(annotations:Mark[])=>update({annotations:annotations.map(mark=>{
    const id=mark.id ?? chartMarkID();
    return {...mark,id,trackingId:mark.trackingId ?? id,coordinateSystem:mark.coordinateSystem ?? "legacy-svg-percent-v1"};
  })}),notes:draft.value.notes,setNotes:(notes:string)=>update({notes}),
    examStatus:draft.value.examStatus ?? "unspecified",setExamStatus:(examStatus:ExamStatus)=>update({examStatus}),
    saving:draft.saving,dirty:chartDirty(draft),save,error:draft.error,remote:draft.remote,resolve };
}

export function ChartSaveNotice({ editor, canEdit }: { editor: ReturnType<typeof useChartSave>; canEdit: boolean }) {
  return <div className="grid min-w-0 gap-2 text-xs">
    <label className="grid min-w-0 gap-1 font-semibold">Examination status<select aria-label="Examination status" className="min-h-11 min-w-0 w-full rounded border bg-white p-2" disabled={!canEdit || editor.saving} value={editor.examStatus} onChange={event=>editor.setExamStatus(event.target.value as ExamStatus)}>
      <option value="unspecified">Not specified (an empty chart does not mean normal)</option><option value="not_examined">Not examined</option><option value="no_findings">Examined — no findings recorded</option><option value="findings">Findings recorded</option>
    </select></label>
    {editor.error && <p role="alert" className="text-red-700">{editor.error} Your draft is retained.</p>}
    {editor.remote && <div role="alert" className="rounded border border-amber-400 p-3"><p className="font-bold">Another version exists. Compare before saving.</p>
      <p>Server version {editor.remote.version} · {editor.remote.examStatus ?? "unspecified"}</p>
      <p className="whitespace-pre-wrap">{editor.remote.notes || "No notes"}</p>
      <ul className="my-2 space-y-1">{editor.remote.annotations.map((mark,index)=><li key={mark.id ?? index}>{mark.structure}: {mark.label || mark.grade || mark.shape} ({mark.x}, {mark.y})</li>)}</ul>
      <button type="button" disabled={!canEdit} className="min-h-11 underline" onClick={()=>editor.resolve(false)}>Discard my draft and load server</button>{" · "}
      <button type="button" disabled={!canEdit} className="min-h-11 underline" onClick={()=>editor.resolve(true)}>I reviewed it — keep my draft for the next save</button>
    </div>}
  </div>;
}
