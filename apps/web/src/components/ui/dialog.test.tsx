// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Dialog, DialogContent, DialogFooter, DialogTitle } from "./dialog";

afterEach(cleanup);

describe("Dialog viewport containment", () => {
  it("keeps large forms inside the visible viewport with accessible controls", () => {
    render(<Dialog open><DialogContent className="max-w-6xl"><DialogTitle>Large form</DialogTitle><div style={{ height: 2000 }}>Fields</div><DialogFooter><button>Save</button></DialogFooter></DialogContent></Dialog>);
    const dialog = screen.getByRole("dialog");
    expect(dialog.className).toContain("max-h-[calc(100dvh-.5rem)]");
    expect(dialog.className).toContain("w-full");
    expect(dialog.className).toContain("overflow-y-auto");
    expect(dialog.className).toContain("bottom-0");
    expect(dialog.className).toContain("touch-pan-y");
    expect(screen.getByRole("button", { name: "Close" }).className).toContain("sticky");
    expect(screen.getByRole("button", { name: "Save" }).parentElement?.className).toContain("bottom-0");
  });
});
