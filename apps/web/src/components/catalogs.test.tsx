// @vitest-environment jsdom
import * as React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { CatalogInput, CatalogManager, CatalogPicker, prescriptionItemFields, type CatalogEntry } from "./catalogs";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function entry(overrides: Partial<CatalogEntry> = {}): CatalogEntry {
  return { id: "c1", catalog: "appointment_reason", label: "Eye examination", details: {}, sortOrder: 10, active: true, version: 1, updatedAt: "", ...overrides };
}

function stubCatalog(items: CatalogEntry[]) {
  return vi.spyOn(api, "get").mockImplementation((path: string) =>
    Promise.resolve(path.startsWith("/catalogs") ? { items } : { items: [] }) as never);
}

const wrap = (node: React.ReactElement) => render(<I18nProvider>{node}</I18nProvider>);

describe("CatalogInput", () => {
  it("suggests the clinic's entries without preventing anything else being typed", async () => {
    stubCatalog([entry(), entry({ id: "c2", label: "Follow-up" })]);
    const onChange = vi.fn();
    wrap(<CatalogInput catalog="appointment_reason" listId="reasons" value="" onChange={onChange} />);
    await waitFor(() => expect(document.querySelectorAll("#reasons option")).toHaveLength(2));
    // An input with a `list` attribute exposes the combobox role.
    const field = screen.getByRole("combobox");
    expect(field.getAttribute("list")).toBe("reasons");
    fireEvent.change(field, { target: { value: "Something the catalog does not have" } });
    expect(onChange).toHaveBeenCalledWith("Something the catalog does not have");
  });
});

describe("CatalogPicker", () => {
  it("says where to fill an empty catalog instead of showing an inert dropdown", async () => {
    stubCatalog([]);
    wrap(<CatalogPicker catalog="prescription_item" label="Catalog" onSelect={() => undefined} />);
    expect(await screen.findByText(/System → Catalogs/)).toBeTruthy();
    expect(screen.queryByRole("combobox")).toBeNull();
  });

  it("hands the whole entry over so its defaults can prefill a prescription", async () => {
    stubCatalog([entry({ id: "p1", catalog: "prescription_item", label: "Timolol 0.5%", details: { strength: "0.5%", frequency: "Twice daily" } })]);
    const onSelect = vi.fn();
    wrap(<CatalogPicker catalog="prescription_item" label="Catalog" onSelect={onSelect} />);
    const select = await screen.findByRole("combobox");
    fireEvent.change(select, { target: { value: "p1" } });
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ label: "Timolol 0.5%", details: { strength: "0.5%", frequency: "Twice daily" } }));
  });

  it("offers every field a prescription needs", () => {
    expect([...prescriptionItemFields]).toEqual(["strength", "dosage", "frequency", "route", "duration", "instructions"]);
  });
});

describe("CatalogManager", () => {
  it("shows retired entries as retired and offers to restore them", async () => {
    stubCatalog([entry(), entry({ id: "c3", label: "Old reason", active: false })]);
    wrap(<CatalogManager catalog="appointment_reason" title="Appointment reasons" description="Editable here." />);
    expect(await screen.findByText("Old reason")).toBeTruthy();
    expect(screen.getByText("Retired")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Restore" })).toBeTruthy();
    // An active entry offers edit and retire, not restore.
    expect(screen.getByLabelText("Retire Eye examination")).toBeTruthy();
  });

  it("retires rather than deletes, so existing records keep their meaning", async () => {
    stubCatalog([entry()]);
    const remove = vi.spyOn(api, "delete").mockResolvedValue(undefined as never);
    wrap(<CatalogManager catalog="appointment_reason" title="Appointment reasons" description="Editable here." />);
    fireEvent.click(await screen.findByLabelText("Retire Eye examination"));
    await waitFor(() => expect(remove).toHaveBeenCalledWith("/catalogs/appointment_reason/c1"));
  });

  it("adds an entry from the interface, with no deployment", async () => {
    stubCatalog([]);
    const post = vi.spyOn(api, "post").mockResolvedValue({} as never);
    wrap(<CatalogManager catalog="prescription_item" title="Prescription catalog" description="Editable here." detailFields={prescriptionItemFields} />);
    fireEvent.click(screen.getAllByRole("button", { name: "Add entry" })[0]);
    fireEvent.change(screen.getByLabelText(/^Label/), { target: { value: "Timolol 0.5%" } });
    fireEvent.change(screen.getByLabelText(/^Frequency/), { target: { value: "Twice daily" } });
    fireEvent.click(screen.getByRole("button", { name: "Save entry" }));
    await waitFor(() => expect(post).toHaveBeenCalledWith("/catalogs/prescription_item", expect.objectContaining({
      label: "Timolol 0.5%", details: expect.objectContaining({ frequency: "Twice daily" }),
    })));
  });
});
