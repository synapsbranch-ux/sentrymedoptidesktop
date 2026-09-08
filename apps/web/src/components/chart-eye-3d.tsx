import * as React from "react";
import * as THREE from "three";
import { anatomyRegions, clampAtlasView, eyeLandmarks, type AnatomyRegion, type AtlasView } from "./chart-anatomy";
import type { Eye } from "./chart-backdrops";

interface Props {
  eye: Eye; region: AnatomyRegion | null; view: AtlasView; cutaway: boolean; isolate: boolean;
  onExplore(region: AnatomyRegion): void; onViewChange(view: AtlasView): void; onUnavailable(): void;
}
interface SceneHandle { update(props: Props): void; dispose(): void }

/** Render-only anatomy: no patient coordinates are projected onto this model. */
function createEyeScene(canvas: HTMLCanvasElement, props: React.RefObject<Props>): SceneHandle {
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: false });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
  renderer.setClearColor("#f8fafc");
  renderer.localClippingEnabled = true;
  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(38, 1, 0.1, 30);
  const group = new THREE.Group();
  scene.add(group, new THREE.HemisphereLight(0xffffff, 0x64748b, 2.5));
  const light = new THREE.DirectionalLight(0xffffff, 2.5);
  light.position.set(3, 4, 5); scene.add(light);
  const geometries: THREE.BufferGeometry[] = [];
  const materials: THREE.MeshStandardMaterial[] = [];
  const meshes: THREE.Mesh[] = [];
  // In model space +Y is superior, +Z is anterior. OD/OS mirror the nasal axis only.
  const plane = new THREE.Plane(new THREE.Vector3(-1, 0, 0), 0);
  const shellPlanes = [plane];
  const mesh = (id: AnatomyRegion, geometry: THREE.BufferGeometry, scale: [number, number, number], position: readonly [number, number, number], opacity = 1, clip = false) => {
    const material = new THREE.MeshStandardMaterial({ color: anatomyRegions.find(item => item.id === id)!.color, roughness: 0.48, transparent: opacity < 1, opacity, side: THREE.DoubleSide, depthWrite: opacity === 1 });
    const object = new THREE.Mesh(geometry, material);
    object.scale.set(...scale); object.position.set(...position);
    object.userData = { region: id, clip };
    geometries.push(geometry); materials.push(material); meshes.push(object); group.add(object);
    return object;
  };
  const sphere = () => new THREE.SphereGeometry(1, 40, 24);
  mesh("sclera", sphere(), [1, 1, 1], [0, 0, 0], 1, true);
  mesh("retina", sphere(), [0.96, 0.96, 0.96], [0, 0, 0], 1, true);
  mesh("vitreous", sphere(), [0.89, 0.89, 0.82], [0, 0, -0.03], 0.1, true);
  mesh("lens", sphere(), [0.38, 0.38, 0.16], [0, 0, 0.53]);
  mesh("iris", new THREE.RingGeometry(0.18, 0.53, 48), [1, 1, 1], [0, 0, 0.79]);
  // A dome cap joins the anterior globe; it does not cover the iris as an opaque globe would.
  const cap = new THREE.SphereGeometry(0.67, 40, 20, 0, Math.PI * 2, 0, Math.PI / 3);
  cap.rotateX(Math.PI / 2);
  mesh("cornea", cap, [1, 1, 1], [0, 0, 0.465], 0.3);
  // Remove the globe's anterior cap so that the cornea, iris and pupil are visible.
  const anteriorPlane = new THREE.Plane(new THREE.Vector3(0, 0, -1), 0.8);
  const landmarks = eyeLandmarks(props.current.eye);
  mesh("optic_disc", sphere(), [0.105, 0.12, 0.03], landmarks.disc);
  mesh("macula", sphere(), [0.13, 0.13, 0.024], landmarks.macula);
  const nerve = new THREE.CylinderGeometry(0.115, 0.14, 0.64, 24);
  nerve.rotateX(Math.PI / 2);
  mesh("optic_nerve", nerve, [1, 1, 1], [landmarks.disc[0], 0, -1.24]);

  let disposed = false;
  let frame = 0;
  const render = () => {
    if (disposed || frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      if (!disposed && !document.hidden) {
        try { renderer.render(scene, camera); }
        catch { props.current.onUnavailable(); }
      }
    });
  };
  const update = (value: Props) => {
    group.rotation.set(THREE.MathUtils.degToRad(value.view.pitch), THREE.MathUtils.degToRad(value.view.yaw), 0);
    camera.position.set(0, 0, 4.7 / value.view.zoom);
    group.updateMatrixWorld(true);
    // A world-space cut plane follows the rotated anatomy, never the camera.
    plane.set(new THREE.Vector3(-1, 0, 0), 0).applyMatrix4(group.matrixWorld);
    anteriorPlane.set(new THREE.Vector3(0, 0, -1), 0.8).applyMatrix4(group.matrixWorld);
    meshes.forEach(object => {
      object.visible = !value.isolate || !value.region || object.userData.region === value.region;
      const material = object.material as THREE.MeshStandardMaterial;
      material.emissive.set(object.userData.region === value.region ? "#2563eb" : "#000000");
      material.emissiveIntensity = 0.6;
      const wasClipped = material.clippingPlanes?.length ?? 0;
      material.clippingPlanes = object.userData.clip ? [anteriorPlane, ...(value.cutaway ? shellPlanes : [])] : [];
      if (wasClipped !== material.clippingPlanes.length) material.needsUpdate = true;
      // Vitreous transparency would otherwise intercept every pick through the opening.
      object.userData.pickable = object.userData.region !== "vitreous" || value.region === "vitreous";
    });
    render();
  };
  const resize = () => {
    const width = canvas.clientWidth, height = canvas.clientHeight;
    if (!width || !height) return;
    renderer.setSize(width, height, false);
    camera.aspect = width / height; camera.updateProjectionMatrix(); render();
  };
  const observer = new ResizeObserver(resize); observer.observe(canvas);
  const raycaster = new THREE.Raycaster();
  let drag: { x: number; y: number; view: AtlasView; moved: boolean; pointerId: number } | null = null;
  const down = (event: PointerEvent) => {
    if (event.button !== 0) return;
    drag = { x: event.clientX, y: event.clientY, view: props.current.view, moved: false, pointerId: event.pointerId };
    if (event.pointerType === "mouse") canvas.setPointerCapture(event.pointerId);
  };
  const move = (event: PointerEvent) => {
    if (!drag || event.pointerId !== drag.pointerId) return;
    const dx = event.clientX - drag.x, dy = event.clientY - drag.y;
    if (Math.hypot(dx, dy) > 6) drag.moved = true;
    if (drag.moved && event.pointerType === "mouse") props.current.onViewChange(clampAtlasView({ ...drag.view, yaw: drag.view.yaw + dx * 0.4, pitch: drag.view.pitch + dy * 0.4 }));
  };
  const up = (event: PointerEvent) => {
    if (!drag || event.pointerId !== drag.pointerId) return;
    const click = !drag.moved; drag = null;
    if (canvas.hasPointerCapture(event.pointerId)) canvas.releasePointerCapture(event.pointerId);
    if (!click) return;
    const rect = canvas.getBoundingClientRect();
    raycaster.setFromCamera(new THREE.Vector2((event.clientX - rect.left) / rect.width * 2 - 1, -(event.clientY - rect.top) / rect.height * 2 + 1), camera);
    const hit = raycaster.intersectObjects(meshes).find(item => {
      const material = (item.object as THREE.Mesh).material as THREE.MeshStandardMaterial;
      return item.object.visible && item.object.userData.pickable && !material.clippingPlanes?.some(clip => clip.distanceToPoint(item.point) < 0);
    });
    if (hit) props.current.onExplore(hit.object.userData.region as AnatomyRegion);
  };
  const cancel = () => { drag = null; };
  const lost = (event: Event) => { event.preventDefault(); props.current.onUnavailable(); };
  canvas.addEventListener("pointerdown", down); canvas.addEventListener("pointermove", move);
  canvas.addEventListener("pointerup", up); canvas.addEventListener("pointercancel", cancel);
  canvas.addEventListener("webglcontextlost", lost); document.addEventListener("visibilitychange", render);
  update(props.current); resize();
  return { update, dispose() {
    disposed = true; cancelAnimationFrame(frame); observer.disconnect();
    canvas.removeEventListener("pointerdown", down); canvas.removeEventListener("pointermove", move);
    canvas.removeEventListener("pointerup", up); canvas.removeEventListener("pointercancel", cancel);
    canvas.removeEventListener("webglcontextlost", lost); document.removeEventListener("visibilitychange", render);
    geometries.forEach(item => item.dispose()); materials.forEach(item => item.dispose()); renderer.dispose(); renderer.forceContextLoss();
  } };
}

export default function ChartEye3D(props: Props) {
  const canvas = React.useRef<HTMLCanvasElement>(null);
  const scene = React.useRef<SceneHandle | null>(null);
  const latest = React.useRef(props); latest.current = props;
  React.useEffect(() => {
    try { scene.current = createEyeScene(canvas.current!, latest); }
    catch { latest.current.onUnavailable(); }
    return () => { scene.current?.dispose(); scene.current = null; };
  }, [props.eye]);
  React.useEffect(() => { scene.current?.update(props); }, [props]);
  return <canvas ref={canvas} role="img" aria-label={`${props.eye} 3D anatomical model`} className="h-72 min-w-0 w-full rounded border [touch-action:pan-y]" />;
}
