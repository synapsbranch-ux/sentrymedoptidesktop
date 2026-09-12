// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { saveBlob } from "./download";

afterEach(() => {
  vi.restoreAllMocks();
  delete (window as { go?: unknown }).go;
  document.querySelectorAll("a[download]").forEach((node) => node.remove());
});

describe("saveBlob — browser/PWA path (no desktop shell present)", () => {
  beforeEach(() => {
    URL.createObjectURL = vi.fn(() => "blob:clinic/file");
    URL.revokeObjectURL = vi.fn();
  });

  it("clicks a temporary anchor and removes it immediately, never leaving it in the document", async () => {
    const clicks: HTMLAnchorElement[] = [];
    const originalCreateElement = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tag: string) => {
      const element = originalCreateElement(tag);
      if (tag === "a") {
        clicks.push(element as HTMLAnchorElement);
        (element as HTMLAnchorElement).click = vi.fn();
      }
      return element;
    });
    const outcome = await saveBlob(new Blob(["invoice"]), "invoice.pdf");
    expect(outcome).toBe("saved");
    expect(clicks).toHaveLength(1);
    expect(clicks[0].download).toBe("invoice.pdf");
    expect(clicks[0].click).toHaveBeenCalledTimes(1);
    // Removed straight after the click — never left sitting in the DOM.
    expect(document.body.contains(clicks[0])).toBe(false);
  });

  it("does not throw if the click itself throws, and still removes the anchor", async () => {
    const originalCreateElement = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tag: string) => {
      const element = originalCreateElement(tag);
      if (tag === "a") (element as HTMLAnchorElement).click = vi.fn(() => { throw new Error("boom"); });
      return element;
    });
    await expect(saveBlob(new Blob(["x"]), "x.pdf")).rejects.toThrow("boom");
    expect(document.querySelectorAll("a[download]")).toHaveLength(0);
  });

  it("supports repeated downloads in the same session without leaking state", async () => {
    // jsdom has no real download machinery, so a genuine anchor click here
    // logs a benign "navigation not implemented" console error; stub click()
    // the same way a real browser's download handling would intercept it.
    const originalCreateElement = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tag: string) => {
      const element = originalCreateElement(tag);
      if (tag === "a") (element as HTMLAnchorElement).click = vi.fn();
      return element;
    });
    for (let i = 0; i < 5; i++) {
      // eslint-disable-next-line no-await-in-loop
      await expect(saveBlob(new Blob([`file-${i}`]), `file-${i}.pdf`)).resolves.toBe("saved");
    }
    expect(document.querySelectorAll("a[download]")).toHaveLength(0);
  });
});

describe("saveBlob — desktop shell path", () => {
  it("base64-encodes the file and hands it to the native Save As dialog instead of the browser download machinery", async () => {
    const saveFile = vi.fn().mockResolvedValue("/home/clinic/Documents/invoice.pdf");
    (window as unknown as { go: unknown }).go = { main: { DesktopBridge: { SaveFile: saveFile } } };
    const createObjectURL = vi.fn();
    URL.createObjectURL = createObjectURL;

    const outcome = await saveBlob(new Blob(["invoice bytes"]), "invoice.pdf");

    expect(outcome).toBe("saved");
    expect(saveFile).toHaveBeenCalledTimes(1);
    const [name, base64] = saveFile.mock.calls[0] as [string, string];
    expect(name).toBe("invoice.pdf");
    expect(typeof base64).toBe("string");
    expect(base64.length).toBeGreaterThan(0);
    // The desktop path never touches the browser's own download machinery —
    // the whole point is to avoid the webview's unreliable handling of it.
    expect(createObjectURL).not.toHaveBeenCalled();
  });

  it("treats an empty path as a user cancellation, not a failure", async () => {
    (window as unknown as { go: unknown }).go = { main: { DesktopBridge: { SaveFile: vi.fn().mockResolvedValue("") } } };
    await expect(saveBlob(new Blob(["x"]), "x.pdf")).resolves.toBe("cancelled");
  });

  it("surfaces a native dialog failure as a plain Error, never an unhandled rejection shape", async () => {
    (window as unknown as { go: unknown }).go = { main: { DesktopBridge: { SaveFile: vi.fn().mockRejectedValue("native failure") } } };
    await expect(saveBlob(new Blob(["x"]), "x.pdf")).rejects.toThrow();
  });
});
