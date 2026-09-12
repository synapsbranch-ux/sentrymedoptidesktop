import * as React from "react";
import { ImagePlus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useLoad } from "../hooks";
import { Button } from "./ui/button";

interface StoredImage { id: string; displayName: string; mediaType: string; sizeBytes: number; caption: string; sortOrder: number; createdAt: string; uploadedBy: string }

/**
 * Pictures attached to a stock item or a lab order.
 *
 * Each image is fetched through the authenticated API and rendered from an
 * object URL: a plain <img src> cannot carry the desktop shell's in-memory
 * session header, so a direct URL would show a broken image there.
 */
export function ImageGallery({ entityType, entityId, canEdit, emptyHint }: { entityType: "inventory_item" | "lab_order"; entityId: string; canEdit: boolean; emptyHint: string }) {
  const images = useLoad(() => api.get<{ items: StoredImage[] }>(`/images?entityType=${entityType}&entityId=${encodeURIComponent(entityId)}`), [entityType, entityId]);
  const [uploading, setUploading] = React.useState(false);

  const upload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    const body = new FormData();
    body.append("entityType", entityType);
    body.append("entityId", entityId);
    body.append("file", file);
    setUploading(true);
    try { await api.post("/images", body); toast.success("Image added"); images.reload(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not add the image"); }
    finally { setUploading(false); event.target.value = ""; }
  };

  const remove = async (image: StoredImage) => {
    if (!window.confirm(`Remove ${image.displayName}?`)) return;
    try { await api.delete(`/images/${image.id}`); images.reload(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not remove the image"); }
  };

  return <div className="grid gap-3">
    {images.data?.items.length ? (
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        {images.data.items.map((image) => (
          <figure key={image.id} className="overflow-hidden rounded-lg border">
            <StoredImageView id={image.id} alt={image.caption || image.displayName} />
            <figcaption className="flex items-center justify-between gap-2 border-t px-2 py-1.5 text-[11px] text-[var(--muted-foreground)]">
              <span className="min-w-0 truncate">{image.caption || image.displayName}</span>
              {canEdit && <button type="button" aria-label={`Remove ${image.displayName}`} className="shrink-0 text-zinc-400 hover:text-red-700" onClick={() => remove(image)}><Trash2 className="h-3.5 w-3.5" /></button>}
            </figcaption>
          </figure>
        ))}
      </div>
    ) : <p className="text-sm text-[var(--muted-foreground)]">{emptyHint}</p>}
    {canEdit && (
      <label className="inline-flex h-11 w-fit cursor-pointer items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--input)] bg-[var(--card)] px-4 text-sm font-semibold hover:bg-[var(--muted)]">
        <ImagePlus className="h-4 w-4" />{uploading ? "Uploading…" : "Add image"}
        <input className="sr-only" type="file" accept="image/jpeg,image/png,image/webp" onChange={upload} disabled={uploading} />
      </label>
    )}
  </div>;
}

/**
 * One stored image, fetched through the authenticated client. Exported because
 * the lab order document prints the frame photograph, and a third copy of this
 * effect is a third place for the object URL to leak.
 */
export function StoredImageView({ id, alt, className }: { id: string; alt: string; className?: string }) {
  const [source, setSource] = React.useState("");
  React.useEffect(() => {
    let url = "";
    let active = true;
    api.blob(`/images/${id}/content`)
      .then(({ blob }) => { if (!active) return; url = URL.createObjectURL(blob); setSource(url); })
      .catch(() => undefined);
    return () => { active = false; if (url) URL.revokeObjectURL(url); };
  }, [id]);
  return <div className={className ?? "grid aspect-square place-items-center bg-zinc-50"}>{source ? <img className="h-full w-full object-contain" src={source} alt={alt} /> : <span className="text-xs text-zinc-400">Loading…</span>}</div>;
}

/** A single thumbnail for a list row, with no gallery around it. */
export function FirstImageThumbnail({ entityType, entityId }: { entityType: "inventory_item" | "lab_order"; entityId: string }) {
  const images = useLoad(() => api.get<{ items: StoredImage[] }>(`/images?entityType=${entityType}&entityId=${encodeURIComponent(entityId)}`), [entityType, entityId]);
  const first = images.data?.items[0];
  if (!first) return null;
  return <div className="h-10 w-10 overflow-hidden rounded border"><StoredImageView id={first.id} alt={first.caption || first.displayName} /></div>;
}

export type { StoredImage };
