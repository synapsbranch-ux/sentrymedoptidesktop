// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { DocumentViewer, previewKind, type ViewableDocument } from "./document-viewer";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

beforeEach(() => {
  // jsdom implements neither of these; the viewer only needs stable identifiers.
  URL.createObjectURL = vi.fn(() => "blob:clinic/preview");
  URL.revokeObjectURL = vi.fn();
});

const pdf: ViewableDocument = { id: "d1", displayName: "referral.pdf", mediaType: "application/pdf", sizeBytes: 2048 };
const photo: ViewableDocument = { id: "d2", displayName: "anterior.jpg", mediaType: "image/jpeg", sizeBytes: 4096 };
const scan: ViewableDocument = { id: "d3", displayName: "scan.tiff", mediaType: "image/tiff", sizeBytes: 8192 };

function open(item: ViewableDocument) {
  return render(<I18nProvider><DocumentViewer document={item} onClose={() => undefined} /></I18nProvider>);
}

describe("preview classification", () => {
  it("previews the formats browsers actually paint and downloads the rest", () => {
    expect(previewKind("application/pdf")).toBe("pdf");
    expect(previewKind("image/jpeg")).toBe("image");
    expect(previewKind("image/png; charset=binary")).toBe("image");
    expect(previewKind("image/tiff")).toBe("none");
    expect(previewKind("application/msword")).toBe("none");
    expect(previewKind("")).toBe("none");
  });
});

describe("DocumentViewer", () => {
  it("fetches through the authenticated client rather than linking to the file", async () => {
    const blob = vi.spyOn(api, "blob").mockResolvedValue({ blob: new Blob(["x"]), filename: "referral.pdf" });
    open(pdf);
    await waitFor(() => expect(blob).toHaveBeenCalledWith("/documents/d1/content"));
    expect(screen.getByTitle("referral.pdf").getAttribute("src")).toBe("blob:clinic/preview");
    // A direct href would send no credentials from the desktop shell.
    expect(document.querySelector('a[href*="/api/v1/documents"]')).toBeNull();
  });

  it("offers zoom and rotate on an image", async () => {
    vi.spyOn(api, "blob").mockResolvedValue({ blob: new Blob(["x"]), filename: "anterior.jpg" });
    open(photo);
    const image = await screen.findByAltText("anterior.jpg");
    expect(image.getAttribute("style")).toContain("scale(1)");
    screen.getByLabelText("Zoom in").click();
    await waitFor(() => expect(screen.getByAltText("anterior.jpg").getAttribute("style")).toContain("scale(1.25)"));
    screen.getByLabelText("Rotate").click();
    await waitFor(() => expect(screen.getByAltText("anterior.jpg").getAttribute("style")).toContain("rotate(90deg)"));
  });

  it("offers a download instead of a dead link for a format it cannot paint", async () => {
    vi.spyOn(api, "blob").mockResolvedValue({ blob: new Blob(["x"]), filename: "scan.tiff" });
    open(scan);
    expect(await screen.findByText("This file type cannot be previewed here")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Download" }).length).toBeGreaterThan(0);
  });

  it("reports a failed read instead of showing an empty frame", async () => {
    vi.spyOn(api, "blob").mockRejectedValue(new Error("Document was not found."));
    open(pdf);
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByText("Document was not found.")).toBeTruthy();
  });
});
