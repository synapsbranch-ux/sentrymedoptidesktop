import { desktopBridge } from "./native";

export type SaveOutcome = "saved" | "cancelled";

/**
 * The one function this application uses to hand a generated file (an
 * invoice/claim/report PDF, a prescription, a stored document, a CSV export,
 * the clinic's CA certificate…) to the person using it.
 *
 * Every "Download" button used to build its own Blob, create its own
 * <a download> element and click it — or, in two places, pointed a bare
 * <a href> straight at the API. Inside the Wails desktop window that browser
 * download machinery is not reliable: the embedded WebKitGTK webview on
 * Linux has no download delegate registered at all, so handing it a blob URL
 * to download can leave the single-threaded GTK/JS event loop unable to make
 * progress — which is exactly the "the application needs to be restarted"
 * failure this was written to fix. See docs/ARCHITECTURE.md.
 *
 * Inside the desktop shell this instead calls the Wails-bound SaveFile
 * method, which uses the operating system's own native Save As dialog and
 * never touches the webview's download handling. Everywhere else — the
 * mobile/LAN PWA and any ordinary browser — it keeps using the standard,
 * well-supported Blob + temporary-anchor download, which does not share the
 * desktop shell's failure mode.
 *
 * Every caller gets the same guarantee: this either returns, or throws an
 * Error with a message fit to show in a toast. It never leaves a dangling
 * object URL, a disabled button, or a stuck dialog behind, on success or on
 * failure.
 */
export async function saveBlob(blob: Blob, filename: string): Promise<SaveOutcome> {
  const bridge = desktopBridge();
  if (bridge?.SaveFile) {
    const base64 = await blobToBase64(blob);
    let path: string;
    try {
      path = await bridge.SaveFile(filename, base64);
    } catch (reason) {
      throw new Error(reason instanceof Error ? reason.message : "Could not open the save dialog.");
    }
    return path ? "saved" : "cancelled";
  }
  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    link.rel = "noopener";
    document.body.append(link);
    try {
      link.click();
    } finally {
      link.remove();
    }
  } finally {
    // The browser reads the object URL to start its own download before this
    // task runs; revoking immediately can cancel that download on some
    // browsers, so release it shortly after instead of holding it forever.
    window.setTimeout(() => URL.revokeObjectURL(url), 30_000);
  }
  return "saved";
}

function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("Could not read the generated file."));
    reader.onload = () => {
      const result = String(reader.result ?? "");
      // readAsDataURL yields "data:<type>;base64,<payload>" — only the
      // payload is meaningful to the Go []byte parameter on the other side.
      const comma = result.indexOf(",");
      resolve(comma === -1 ? "" : result.slice(comma + 1));
    };
    reader.readAsDataURL(blob);
  });
}

/** Fetches a stored/generated file through the authenticated API client and saves it, in one call — the shape almost every download button needs. */
export async function downloadFromApi(fetchBlob: () => Promise<{ blob: Blob; filename: string }>, fallbackFilename: string): Promise<SaveOutcome> {
  const { blob, filename } = await fetchBlob();
  return saveBlob(blob, filename || fallbackFilename);
}
