// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { api } from "../api";
import type { Patient } from "../types";
import { Dialog } from "../components/ui/dialog";
import { PatientForm } from "./patients";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("edits gender without submitting read-only API response fields or losing hidden editable values", async () => {
 const patient = {
  id: "p1", medicalRecordNumber: "PT-000001", firstName: "Alice", lastName: "Joseph",
  sex: "female", version: 4, createdAt: "2026-01-01", updatedAt: "2026-02-01", updatedBy: "staff",
  tags: ["follow-up"], employer: "School", preferredLanguage: "ht", lastVisitAt: "2026-02-01",
 } as unknown as Patient;
 const put=vi.spyOn(api,"put").mockResolvedValue({...patient,sex:"male",version:5});
 const saved=vi.fn();
 render(<I18nProvider><Dialog open><PatientForm patient={patient} onSaved={saved}/></Dialog></I18nProvider>);
 fireEvent.change(screen.getByLabelText("Sex"),{target:{value:"male"}});
 fireEvent.click(screen.getByRole("button",{name:"Save changes"}));
 await waitFor(()=>expect(saved).toHaveBeenCalledWith(expect.objectContaining({sex:"male",version:5})));
 expect(put).toHaveBeenCalledWith("/patients/p1",expect.objectContaining({sex:"male",version:4,tags:["follow-up"],employer:"School",preferredLanguage:"ht"}));
 const body=put.mock.calls[0][1] as Record<string,unknown>;
 for(const key of ["id","medicalRecordNumber","createdAt","updatedAt","updatedBy","lastVisitAt"]) expect(body).not.toHaveProperty(key);
});
