import { expect, test, type Page } from "@playwright/test";

// A real WebGL2 renderer via Chromium's software GPU, also available on CI hosts.
test.use({ serviceWorkers: "block", launchOptions: { args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"] } });

async function prepare(page: Page) {
  await page.goto("/");
  await page.getByLabel("Email or username").fill("doctor.dev");
  await page.getByRole("textbox",{name:"Password",exact:true}).fill("Doctor-Development-Only-2026");
  await page.getByRole("button",{name:"Sign in"}).click();
  await expect(page.getByRole("heading",{name:/Good day/})).toBeVisible();
  const post = async (path:string,data:unknown) => {
    const result=await page.request.post(`/api/v1${path}`,{data});
    expect(result.ok(),await result.text()).toBe(true); return result.json();
  };
  const name=`Atlas-${Date.now()}`;
  const patient=await post("/patients",{firstName:"Test",lastName:name,tags:[]});
  const visit=await post("/encounters",{patientId:patient.id,visitReason:"Anatomy review"});
  const path=`/api/v1/encounters/${visit.id}/eye-diagrams`;
  for (const eye of ["OD","OS"]) {
    const saved=await page.request.put(`${path}/fundus/${eye}`,{data:{version:0,annotations:[{x:eye==="OD"?20:80,y:25,shape:"dot",label:`${eye} retinal finding`,structure:"retina",color:"#dc2626"}],notes:"Original chart"}});
    expect(saved.ok()).toBe(true);
  }
  const before=await (await page.request.get(path)).json();
  await page.goto("/clinical");
  await page.getByRole("button",{name:new RegExp(name)}).click();
  await page.getByRole("button",{name:"Charts",exact:true}).click();
  await page.getByRole("button",{name:"Fundus",exact:true}).click();
  const od=page.getByRole("region",{name:"Right fundus (OD) chart editor"});
  return {od,path,before,visit,name};
}

test("2D, section and real 3D share selection without rewriting clinical positions",async({page})=>{
  const requests:string[]=[]; page.on("request",request=>requests.push(request.url()));
  const {od,path,before,visit,name}=await prepare(page);
  await od.getByRole("button",{name:/^OD retinal finding/}).click();
  await od.getByRole("button",{name:"2.5D",exact:true}).click();
  await expect(od.getByRole("img",{name:"OD 2.5D anatomical section"})).toBeVisible();
  await expect(od.getByRole("button",{name:"Retina",exact:true})).toHaveAttribute("aria-pressed","true");
  expect(requests.some(url=>url.includes("chart-eye-3d-"))).toBe(false);
  await od.getByRole("button",{name:"3D",exact:true}).click();
  const canvas=od.getByRole("img",{name:"OD 3D anatomical model"});
  await expect(canvas).toBeVisible();
  await expect(od.getByRole("button",{name:"Retina",exact:true})).toHaveAttribute("aria-pressed","true");
  await od.getByRole("slider",{name:"OD model rotation"}).fill("55");
  await od.getByRole("slider",{name:"OD model zoom"}).fill("1.2");
  await od.getByRole("button",{name:"Lens",exact:true}).click();
  await od.getByLabel("Isolate selected structure").check();
  await expect(od.getByText("Structure: Lens",{exact:true})).toBeVisible();
  await od.getByRole("button",{name:"Reset view"}).click();
  await expect(od.getByRole("slider",{name:"OD model rotation"})).toHaveValue("-35");
  await od.getByRole("button",{name:/^OD retinal finding/}).click();
  await canvas.screenshot({path:"test-results/eye-3d-mobile.png"});
  // Touch picking selects anatomy without adding or deleting a clinical finding.
  await od.getByRole("slider",{name:"OD model rotation"}).fill("0");
  await od.getByRole("slider",{name:"OD model tilt"}).fill("0");
  await od.getByLabel("Cutaway",{exact:true}).uncheck();
  const bounds=await canvas.boundingBox();
  await canvas.tap({position:{x:bounds!.width/2,y:bounds!.height/2}});
  await expect(od.getByRole("button",{name:"Cornea",exact:true})).toHaveAttribute("aria-pressed","true");
  await od.getByRole("button",{name:/^OD retinal finding/}).click();
  await od.getByRole("button",{name:"2D",exact:true}).click();
  await expect(od.getByRole("button",{name:/^OD retinal finding/})).toHaveAttribute("aria-pressed","true");
  await expect(od.getByRole("button",{name:"Save chart",exact:true})).toBeDisabled();
  const after=await (await page.request.get(path)).json();
  expect(after.charts).toEqual(before.charts);

  // Editing the shared finding preserves its IDs and exact legacy chart position.
  await od.getByRole("textbox",{name:"OD selected finding description"}).fill("Reviewed retinal finding");
  await od.getByRole("button",{name:"3D",exact:true}).click();
  await od.getByRole("button",{name:"Save chart",exact:true}).click();
  await expect(od.getByRole("button",{name:"Save chart",exact:true})).toBeDisabled();
  const saved=await (await page.request.get(path)).json();
  expect(saved.charts.fundus.OD.annotations[0]).toEqual({...before.charts.fundus.OD.annotations[0],label:"Reviewed retinal finding"});
  expect(saved.charts.fundus.OS).toEqual(before.charts.fundus.OS);
  expect(await od.evaluate(element=>element.scrollWidth<=element.clientWidth+1)).toBe(true);

  const diagnosis=await page.request.post(`/api/v1/encounters/${visit.id}/diagnoses`,{data:{diagnosis:"Test observation",primary:true}});
  expect(diagnosis.ok()).toBe(true);
  const finalized=await page.request.post(`/api/v1/encounters/${visit.id}/finalize`,{data:{version:1}});
  expect(finalized.ok()).toBe(true);
  await page.reload();
  await page.getByRole("button",{name:new RegExp(name)}).click();
  await page.getByRole("button",{name:"Charts",exact:true}).click();
  await page.getByRole("button",{name:"Fundus",exact:true}).click();
  await od.getByRole("button",{name:/^Reviewed retinal finding/}).click();
  await od.getByRole("button",{name:"3D",exact:true}).click();
  await expect(od.getByRole("img",{name:"OD 3D anatomical model"})).toBeVisible();
  await expect(od.getByRole("textbox",{name:"OD selected finding description"})).toBeDisabled();
  await expect(od.getByRole("button",{name:"Save chart",exact:true})).toHaveCount(0);
  expect((await (await page.request.get(path)).json()).charts).toEqual(saved.charts);
});

test("failed WebGL and context loss return to 2D with the draft intact",async({page})=>{
  const {od}=await prepare(page);
  await od.getByRole("textbox",{name:"Right fundus (OD) notes"}).fill("Unsaved note");
  await od.getByRole("button",{name:"3D",exact:true}).click();
  const canvas=od.getByRole("img",{name:"OD 3D anatomical model"});
  await expect(canvas).toBeVisible();
  await canvas.evaluate(element=>element.dispatchEvent(new Event("webglcontextlost",{cancelable:true})));
  await expect(od.getByText(/3D is unavailable on this device/)).toBeVisible();
  await expect(od.getByRole("button",{name:"2D",exact:true})).toHaveAttribute("aria-pressed","true");
  await expect(od.getByRole("textbox",{name:"Right fundus (OD) notes"})).toHaveValue("Unsaved note");
  await page.evaluate(()=>{
    const original=HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext=function(this:HTMLCanvasElement,type:string,...args:unknown[]){
      if(type.startsWith("webgl")) return null;
      return Reflect.apply(original,this,[type,...args]);
    } as typeof original;
  });
  await od.getByRole("button",{name:"3D",exact:true}).click();
  await expect(od.getByText(/3D is unavailable on this device/)).toBeVisible();
  await expect(od.getByRole("textbox",{name:"Right fundus (OD) notes"})).toHaveValue("Unsaved note");
  await expect(od.getByRole("button",{name:"Save chart",exact:true})).toBeEnabled();
});

test("an unavailable 3D module leaves the 2D editor usable",async({page})=>{
  await page.route("**/assets/chart-eye-3d-*.js",route=>route.abort());
  const {od}=await prepare(page);
  await od.getByRole("textbox",{name:"Right fundus (OD) notes"}).fill("Keep this draft");
  await od.getByRole("button",{name:"3D",exact:true}).click();
  await expect(od.getByText(/3D is unavailable on this device/)).toBeVisible();
  await expect(od.getByRole("textbox",{name:"Right fundus (OD) notes"})).toHaveValue("Keep this draft");
  await expect(od.getByRole("button",{name:"Save chart",exact:true})).toBeEnabled();
  await od.getByRole("button",{name:"2.5D",exact:true}).click();
  await expect(od.getByRole("img",{name:"OD 2.5D anatomical section"})).toBeVisible();
});
