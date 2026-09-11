import * as React from "react";
import { Eraser, PenLine, Trash2, Upload } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Badge, ErrorState, Skeleton } from "./ui/data";
import { useLoad } from "../hooks";

/**
 * D3: the signing doctor's own signature, established either by uploading an
 * image or by drawing one on screen. Everything here posts to /me/signature,
 * which resolves the owner from the session: there is no way to set or apply
 * anybody else's signature from this component or from the API behind it.
 */

interface StoredSignature { method: "uploaded" | "drawn"; mediaType: string; sizeBytes: number; version: number; updatedAt: string }
interface SignatureState { present: boolean; signature?: StoredSignature }

async function signatureData(blob: Blob) {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let binary = "";
  // Avoid passing a multi-megabyte array to fromCharCode in one call; WebKit
  // rejects that with a RangeError before the signature ever reaches the server.
  for (let start = 0; start < bytes.length; start += 8192) binary += String.fromCharCode(...bytes.subarray(start, start + 8192));
  return window.btoa(binary);
}

/** A stroke is a list of points; strokes are kept apart so the pen can be lifted. */
type Stroke = { x: number; y: number }[];

export function drawStrokes(context: CanvasRenderingContext2D, strokes: Stroke[], width: number, height: number) {
  context.clearRect(0, 0, width, height);
  context.lineWidth = 2.5;
  context.lineCap = "round";
  context.lineJoin = "round";
  context.strokeStyle = "#111827";
  for (const stroke of strokes) {
    if (stroke.length === 0) continue;
    context.beginPath();
    // A single tap still leaves a visible dot rather than nothing at all.
    if (stroke.length === 1) {
      context.arc(stroke[0].x, stroke[0].y, 1.25, 0, Math.PI * 2);
      context.fillStyle = "#111827";
      context.fill();
      continue;
    }
    context.moveTo(stroke[0].x, stroke[0].y);
    for (const point of stroke.slice(1)) context.lineTo(point.x, point.y);
    context.stroke();
  }
}

export function strokesAreEmpty(strokes: Stroke[]) {
  return strokes.every((stroke) => stroke.length === 0);
}

export function SignatureSettings() {
  const state = useLoad(() => api.get<SignatureState>("/me/signature"), []);
  const [preview, setPreview] = React.useState(0);
  const [busy, setBusy] = React.useState(false);

  const save = async (blob: Blob, method: "uploaded" | "drawn") => {
    setBusy(true);
    try {
      if (blob.size > 2 * 1024 * 1024) throw new APIError(413, { code: "SIGNATURE_TOO_LARGE", message: "A signature image must be 2 MB or smaller." });
      // Wails/WebKitGTK has crashed while handing multipart request bodies to
      // its custom asset scheme. JSON is already the stable transport for the
      // rest of the desktop API, and keeps the image private on the same local
      // authenticated endpoint.
      await api.put("/me/signature", { method, data: await signatureData(blob) });
      toast.success("Signature saved");
      setPreview((value) => value + 1);
      state.reload();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Could not save the signature");
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await api.delete("/me/signature");
      toast.success("Signature removed");
      state.reload();
    } catch (reason) {
      toast.error(reason instanceof APIError ? reason.body.message : "Could not remove the signature");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>My signature</CardTitle>
        <CardDescription>
          Applied to the prescriptions you issue, with your name and the time recorded on each one.
          This is your signature alone — nobody else can set it or apply it.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {state.loading ? <Skeleton className="h-32" /> : state.error ? <ErrorState message={state.error.message} retry={state.reload} /> : (
          <>
            {state.data?.present ? (
              <div className="grid gap-2">
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <Badge tone="success">On file</Badge>
                  <span className="text-zinc-500">{state.data.signature?.method === "drawn" ? "Drawn on screen" : "Uploaded image"}</span>
                  <Button className="ml-auto" size="sm" variant="outline" disabled={busy} onClick={remove}><Trash2 className="h-3.5 w-3.5" />Remove</Button>
                </div>
                <SignaturePreview revision={preview} />
              </div>
            ) : (
              <p className="rounded-md border border-dashed p-3 text-sm text-zinc-500">
                No signature yet. Prescriptions you issue will print with a blank signature line until you add one.
              </p>
            )}
            <SignatureUpload busy={busy} onFile={(file) => save(file, "uploaded")} />
            <SignaturePad busy={busy} onDrawn={(blob) => save(blob, "drawn")} />
          </>
        )}
      </CardContent>
    </Card>
  );
}

/** The stored image, fetched through the authenticated client. */
export function SignaturePreview({ revision, className }: { revision?: number; className?: string }) {
  const [url, setUrl] = React.useState("");
  React.useEffect(() => {
    let objectURL = "";
    let active = true;
    api.blob("/me/signature/image")
      .then(({ blob }) => { if (!active) return; objectURL = URL.createObjectURL(blob); setUrl(objectURL); })
      .catch(() => { if (active) setUrl(""); });
    return () => { active = false; setUrl(""); if (objectURL) URL.revokeObjectURL(objectURL); };
  }, [revision]);
  if (!url) return null;
  return <img alt="Your stored signature" src={url} className={className ?? "h-20 w-auto self-start rounded-md border bg-white object-contain p-2"} />;
}

function SignatureUpload({ busy, onFile }: { busy: boolean; onFile(file: File): void }) {
  return (
    <label className="flex min-h-16 cursor-pointer items-center justify-center gap-2 rounded-md border border-dashed border-zinc-300 p-4 text-sm font-semibold hover:bg-zinc-50">
      <Upload className="h-4 w-4" />
      Upload a signature image — PNG with a transparent background prints best
      <input
        className="sr-only"
        type="file"
        accept="image/png,image/jpeg"
        disabled={busy}
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = "";
          if (file) onFile(file);
        }}
      />
    </label>
  );
}

/**
 * Draw-to-sign. Pointer events cover mouse, trackpad and finger with one code
 * path, which is what makes this work on the tablet it will actually be used on;
 * `touch-none` stops the browser scrolling the page instead of drawing.
 */
export function SignaturePad({ busy, onDrawn }: { busy: boolean; onDrawn(blob: Blob): void }) {
  const canvasRef = React.useRef<HTMLCanvasElement | null>(null);
  const [strokes, setStrokes] = React.useState<Stroke[]>([]);
  const drawing = React.useRef(false);

  React.useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext("2d");
    if (canvas && context) drawStrokes(context, strokes, canvas.width, canvas.height);
  }, [strokes]);

  const pointAt = (event: React.PointerEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    if (!canvas) return { x: 0, y: 0 };
    const bounds = canvas.getBoundingClientRect();
    // The canvas is drawn at its intrinsic size but laid out fluidly, so the
    // pointer position has to be scaled or the ink lands away from the finger.
    return {
      x: ((event.clientX - bounds.left) / bounds.width) * canvas.width,
      y: ((event.clientY - bounds.top) / bounds.height) * canvas.height,
    };
  };

  const start = (event: React.PointerEvent<HTMLCanvasElement>) => {
    if (busy) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    drawing.current = true;
    setStrokes((value) => [...value, [pointAt(event)]]);
  };

  const move = (event: React.PointerEvent<HTMLCanvasElement>) => {
    if (!drawing.current) return;
    const point = pointAt(event);
    setStrokes((value) => value.map((stroke, index) => (index === value.length - 1 ? [...stroke, point] : stroke)));
  };

  const end = () => { drawing.current = false; };

  const save = () => {
    const canvas = canvasRef.current;
    if (!canvas || strokesAreEmpty(strokes)) {
      toast.error("Draw your signature before saving it.");
      return;
    }
    canvas.toBlob((blob) => { if (blob) onDrawn(blob); }, "image/png");
  };

  return (
    <div className="grid gap-2">
      <div className="flex items-center gap-2 text-sm font-semibold"><PenLine className="h-4 w-4" />Or sign here</div>
      <canvas
        ref={canvasRef}
        aria-label="Signature pad"
        role="img"
        width={600}
        height={200}
        className="h-40 w-full touch-none rounded-md border bg-white"
        onPointerDown={start}
        onPointerMove={move}
        onPointerUp={end}
        onPointerLeave={end}
        onPointerCancel={end}
      />
      <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="outline" disabled={busy || strokes.length === 0} onClick={() => setStrokes([])}><Eraser className="h-3.5 w-3.5" />Clear</Button>
        <Button size="sm" disabled={busy || strokesAreEmpty(strokes)} onClick={save}><PenLine className="h-3.5 w-3.5" />Save drawn signature</Button>
      </div>
      <p className="text-xs text-zinc-500">Works with a mouse, a trackpad or a finger on a tablet.</p>
    </div>
  );
}
