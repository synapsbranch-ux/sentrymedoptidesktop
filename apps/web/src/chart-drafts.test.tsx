// @vitest-environment jsdom
import * as React from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api, APIError } from "./api";
import { chartDirty, receiveChart, ChartDraftProvider, useChartSave, type ChartState, type Draft } from "./components/chart-drafts";

afterEach(()=>{cleanup();vi.restoreAllMocks();});
const base:ChartState={annotations:[],notes:"server",version:1};
describe("chart draft safety",()=>{
  it("retains dirty drafts when a new server version arrives",()=>{
    const draft:Draft={base,value:{...base,notes:"mine"},saving:false,error:""};
    const remote={...base,version:2,notes:"theirs"};
    const next=receiveChart(draft,remote);
    expect(next.value.notes).toBe("mine");expect(next.base.version).toBe(1);expect(next.remote).toEqual(remote);
    expect(receiveChart(next,base)).toBe(next);
  });
  it("refreshes clean drafts and treats exam status as a clinical edit",()=>{
    const draft:Draft={base,value:base,saving:false,error:""};
    expect(receiveChart(draft,{...base,version:2}).base.version).toBe(2);
    expect(chartDirty({...draft,value:{...base,examStatus:"not_examined"}})).toBe(true);
  });
  it("does not lose one chart draft when switching chart type",()=>{
    const wrapper=({children}:{children:React.ReactNode})=><ChartDraftProvider>{children}</ChartDraftProvider>;
    const hook=renderHook(({type})=>useChartSave("visit",type,"OD",base,()=>{}),{initialProps:{type:"fundus"},wrapper});
    act(()=>hook.result.current.setNotes("unsaved fundus"));
    hook.rerender({type:"anterior"});expect(hook.result.current.notes).toBe("server");
    hook.rerender({type:"fundus"});expect(hook.result.current.notes).toBe("unsaved fundus");
  });
  it("keeps edits made while an earlier save is in flight",async()=>{
    let finish!:(value:unknown)=>void;
    vi.spyOn(api,"put").mockImplementation(()=>new Promise(resolve=>{finish=resolve;}));
    const wrapper=({children}:{children:React.ReactNode})=><ChartDraftProvider>{children}</ChartDraftProvider>;
    const hook=renderHook(()=>useChartSave("visit","fundus","OD",base,()=>{}),{wrapper});
    act(()=>hook.result.current.setNotes("first edit"));
    act(()=>hook.result.current.setAnnotations([{x:20,y:30,shape:"dot",color:"#dc2626",label:"finding",structure:"retina"}]));
    const markID=hook.result.current.annotations[0].id;
    let pending!:Promise<void>;act(()=>{pending=hook.result.current.save();});
    act(()=>hook.result.current.setNotes("second edit"));
    await act(async()=>{finish({version:2,annotations:hook.result.current.annotations});await pending;});
    expect(hook.result.current.notes).toBe("second edit");expect(hook.result.current.dirty).toBe(true);
    expect(hook.result.current.annotations[0].id).toBe(markID);
    expect(markID).toMatch(/^[0-9a-f-]{36}$/);
  });
  it("retains a failed draft and requires review before rebasing a conflict",async()=>{
    const put=vi.spyOn(api,"put").mockRejectedValue(new APIError(0,{code:"CLINIC_SERVER_UNREACHABLE",message:"Offline"}));
    const wrapper=({children}:{children:React.ReactNode})=><ChartDraftProvider>{children}</ChartDraftProvider>;
    const hook=renderHook(()=>useChartSave("visit","fundus","OD",base,()=>{}),{wrapper});
    act(()=>hook.result.current.setNotes("my finding"));
    await act(async()=>{await hook.result.current.save();});
    expect(hook.result.current.notes).toBe("my finding");expect(hook.result.current.error).toBe("Offline");
    put.mockRejectedValue(new APIError(409,{code:"CONCURRENT_MODIFICATION",message:"Conflict"}));
    vi.spyOn(api,"get").mockResolvedValue({charts:{fundus:{OD:{...base,version:2,notes:"other doctor"}}}});
    await act(async()=>{await hook.result.current.save();});
    expect(hook.result.current.remote?.notes).toBe("other doctor");
    expect(hook.result.current.notes).toBe("my finding");
    const attempts=put.mock.calls.length;
    await act(async()=>{await hook.result.current.save();});expect(put).toHaveBeenCalledTimes(attempts);
    const confirmation=vi.spyOn(window,"confirm").mockReturnValue(false);
    act(()=>hook.result.current.resolve(true));expect(hook.result.current.remote).toBeDefined();
    confirmation.mockReturnValue(true);
    act(()=>hook.result.current.resolve(true));expect(hook.result.current.remote).toBeUndefined();
    put.mockResolvedValue({version:3,annotations:[]});
    await act(async()=>{await hook.result.current.save();});
    expect(put).toHaveBeenLastCalledWith(expect.any(String),expect.objectContaining({version:2,notes:"my finding"}));
  });
});
