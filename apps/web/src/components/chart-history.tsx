import * as React from "react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { chartMarkID, useChartSave, type ChartState, type Mark } from "./chart-drafts";
import { Button } from "./ui/button";

interface Revision extends ChartState { encounterId:string; encounterNumber:string; recordedAt:string; recordedBy:string; baseline:boolean }
const empty:ChartState={ annotations:[],notes:"",version:0 };
export function ChartHistoryPanel({encounterId,chartType,eyes,charts,canEdit,onSaved}: {
  encounterId:string;chartType:string;eyes:string[];charts:Partial<Record<string,ChartState>>;canEdit:boolean;onSaved():void;
}) {
  const [open,setOpen]=React.useState(false);
  const [eye,setEye]=React.useState(eyes[0]);
  return <section className="min-w-0 rounded border p-3 text-sm [overflow-wrap:anywhere]">
    <Button type="button" variant="outline" className="h-auto min-h-11 max-w-full whitespace-normal" onClick={()=>setOpen(!open)} aria-expanded={open}>Chart history and finding follow-up</Button>
    {open && <div className="mt-3 grid gap-3"><label>Eye <select className="min-h-11 rounded border px-3" value={eye} onChange={event=>setEye(event.target.value)}>{eyes.map(value=><option key={value}>{value}</option>)}</select></label>
      <History key={eye} encounterId={encounterId} chartType={chartType} eye={eye} state={charts?.[eye] ?? empty} canEdit={canEdit} onSaved={onSaved} />
    </div>}
  </section>;
}
function History({encounterId,chartType,eye,state,canEdit,onSaved}:{encounterId:string;chartType:string;eye:string;state:ChartState;canEdit:boolean;onSaved():void}) {
  const [offset,setOffset]=React.useState(0);
  const history=useLoad(()=>api.get<{items:Revision[];nextOffset:number|null}>(`/encounters/${encounterId}/eye-diagrams/${chartType}/${eye}/history?offset=${offset}`),[encounterId,chartType,eye,offset,state.version]);
  const editor=useChartSave(encounterId,chartType,eye,state,onSaved);
  const follow=(mark:Mark)=>{
    if (!mark.id || !window.confirm("Add this previous finding to your draft for re-examination? This does not establish that it is still present.")) return;
    const id=chartMarkID();
    editor.setAnnotations([...editor.annotations,{...mark,id,trackingId:mark.trackingId ?? mark.id}]);
    editor.setExamStatus("unspecified");
  };
  if(history.error) return <p role="alert">{history.error.message} <button type="button" onClick={history.reload}>Retry</button></p>;
  if(history.loading) return <p>Loading history…</p>;
  return <div className="grid gap-2">
    <p className="text-xs text-zinc-500">Read-only saved revisions for this eye and chart. Missing annotations do not mean resolved. Follow-up copies require re-examination and an explicit save.</p>
    {!history.data?.items.length && <p>No saved revision.</p>}
    {history.data?.items.map(item=><details className="rounded border p-3" key={`${item.encounterId}/${item.version}`}>
      <summary className="cursor-pointer">{item.encounterNumber} · revision {item.version} · {new Date(item.recordedAt).toLocaleString()} · {item.recordedBy}{item.baseline ? " · migration baseline (earlier revisions unavailable)" : ""}</summary>
      <p className="mt-2">Status: {item.examStatus ?? "unspecified"}</p><p className="whitespace-pre-wrap">{item.notes || "No notes"}</p>
      <ul className="mt-2 grid gap-2">{item.annotations.map((mark,index)=><li key={mark.id ?? index} className="rounded bg-zinc-50 p-2">
        <span>{mark.structure} · {mark.label || mark.grade || mark.shape} · ({mark.x}, {mark.y}){mark.clockHour ? ` · ${mark.clockHour} o'clock` : ""}</span>
        {canEdit && chartType!=="amsler" && item.encounterId!==encounterId && <Button className="ml-2" size="sm" variant="outline" disabled={editor.saving || editor.annotations.length>=200 || editor.annotations.some(current=>(current.trackingId ?? current.id)===(mark.trackingId ?? mark.id) || (!!mark.cell && current.cell===mark.cell))} onClick={()=>follow(mark)}>Follow this finding</Button>}
      </li>)}</ul>
    </details>)}
    <div className="flex gap-2"><Button variant="outline" disabled={offset===0} onClick={()=>setOffset(Math.max(0,offset-20))}>Previous page</Button><Button variant="outline" disabled={history.data?.nextOffset==null} onClick={()=>setOffset(history.data!.nextOffset!)}>Next page</Button></div>
  </div>;
}
