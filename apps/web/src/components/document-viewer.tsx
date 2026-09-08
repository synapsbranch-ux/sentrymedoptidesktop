import * as React from "react";
import { Download, FileWarning, Loader2, Minus, Plus, Printer, RotateCw } from "lucide-react";
import { api } from "../api";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "./ui/dialog";
import { ErrorState } from "./ui/data";
import { useI18n } from "../i18n";

export interface ViewableDocument {
  id: string;
  displayName: string;
  mediaType: string;
  sizeBytes?: number;
}

type Preview = "image" | "pdf" | "none";

/** Image formats every target browser paints. TIFF is deliberately absent: only
 *  Safari renders it, so scans in that format are offered as a download. */
const previewableImages = new Set(["image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/avif"]);

export function previewKind(mediaType: string): Preview {
  const type = (mediaType || "").split(";")[0].trim().toLowerCase();
  if (previewableImages.has(type)) return "image";
  if (type === "application/pdf") return "pdf";
  return "none";
}

const zoomSteps = [0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4];

/**
 * Opens a stored document without leaving the application. The bytes are fetched
 * through the authenticated API client rather than linked directly, because a
 * plain <a href> or <img src> cannot carry the desktop shell's in-memory session
 * header and always produced a dead link there.
 */
export function DocumentViewer({ document: item, onClose }: { document: ViewableDocument | null; onClose(): void }) {
  const { t } = useI18n();
  const [objectURL, setObjectURL] = React.useState("");
  const [error, setError] = React.useState<Error | null>(null);
  const [loading, setLoading] = React.useState(false);
  const [zoomIndex, setZoomIndex] = React.useState(2);
  const [rotation, setRotation] = React.useState(0);
  const frameRef = React.useRef<HTMLIFrameElement | null>(null);
  const id = item?.id ?? "";

  React.useEffect(() => {
    if (!id) return;
    let url = "";
    let active = true;
    setLoading(true);
    setError(null);
    setZoomIndex(2);
    setRotation(0);
    api.blob(`/documents/${id}/content`)
      .then(({ blob }) => {
        if (!active) return;
        url = URL.createObjectURL(blob);
        setObjectURL(url);
      })
      .catch((reason: unknown) => { if (active) setError(reason instanceof Error ? reason : new Error(String(reason))); })
      .finally(() => { if (active) setLoading(false); });
    return () => {
      active = false;
      setObjectURL("");
      if (url) URL.revokeObjectURL(url);
    };
  }, [id]);

  if (!item) return null;
  const kind = previewKind(item.mediaType);
  const zoom = zoomSteps[zoomIndex];

  const download = () => {
    if (!objectURL) return;
    const link = window.document.createElement("a");
    link.href = objectURL;
    link.download = item.displayName || "document";
    window.document.body.append(link);
    link.click();
    link.remove();
  };

  const print = () => {
    if (kind === "pdf") {
      // The embedded viewer owns the rendered pages, so printing is delegated to it.
      const frame = frameRef.current?.contentWindow;
      if (frame) { frame.focus(); frame.print(); return; }
    }
    window.print();
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-5xl">
        <DialogHeader className="no-print">
          <DialogTitle className="truncate pr-8">{item.displayName}</DialogTitle>
          <DialogDescription>
            {item.mediaType}
            {typeof item.sizeBytes === "number" && ` · ${(item.sizeBytes / 1024).toFixed(1)} KB`}
          </DialogDescription>
        </DialogHeader>
        <div className="no-print flex flex-wrap items-center gap-2">
          {kind === "image" && (
            <>
              <Button aria-label="Zoom out" size="icon" variant="outline" disabled={zoomIndex === 0} onClick={() => setZoomIndex((value) => Math.max(0, value - 1))}><Minus className="h-4 w-4" /></Button>
              <span className="min-w-14 text-center font-mono text-xs" aria-live="polite">{Math.round(zoom * 100)}%</span>
              <Button aria-label="Zoom in" size="icon" variant="outline" disabled={zoomIndex === zoomSteps.length - 1} onClick={() => setZoomIndex((value) => Math.min(zoomSteps.length - 1, value + 1))}><Plus className="h-4 w-4" /></Button>
              <Button aria-label="Rotate" size="icon" variant="outline" onClick={() => setRotation((value) => (value + 90) % 360)}><RotateCw className="h-4 w-4" /></Button>
            </>
          )}
          <Button className="ml-auto" size="sm" variant="outline" disabled={!objectURL} onClick={download}><Download className="h-3.5 w-3.5" />Download</Button>
          {kind !== "none" && <Button size="sm" variant="outline" disabled={!objectURL} onClick={print}><Printer className="h-3.5 w-3.5" />Print</Button>}
        </div>
        <div className="mt-3 min-h-[60vh] overflow-auto rounded-lg border bg-[var(--muted)]">
          {loading && <div className="grid min-h-[60vh] place-items-center text-sm text-[var(--muted-foreground)]"><span className="flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />{t("Opening document…")}</span></div>}
          {!loading && error && <div className="p-4"><ErrorState message={error.message} /></div>}
          {!loading && !error && objectURL && kind === "image" && (
            <div className="grid min-h-[60vh] place-items-center p-4">
              <img
                alt={item.displayName}
                src={objectURL}
                className="print-area max-w-none origin-center transition-transform"
                style={{ transform: `scale(${zoom}) rotate(${rotation}deg)` }}
              />
            </div>
          )}
          {!loading && !error && objectURL && kind === "pdf" && (
            <iframe ref={frameRef} title={item.displayName} src={objectURL} className="h-[70vh] w-full border-0" />
          )}
          {!loading && !error && kind === "none" && (
            <div className="grid min-h-[60vh] place-items-center p-8 text-center">
              <div>
                <FileWarning aria-hidden className="mx-auto h-8 w-8 text-[var(--muted-foreground)]" />
                <h3 className="mt-3 font-semibold">{t("This file type cannot be previewed here")}</h3>
                <p className="mt-1 max-w-sm text-sm text-[var(--muted-foreground)]">
                  {t("Download it to open in the application that handles this format. The file itself is intact.")}
                </p>
                <Button className="mt-4" disabled={!objectURL} onClick={download}><Download className="h-4 w-4" />Download</Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
