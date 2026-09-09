import * as React from "react";
import { Eye, FileText, Plus, Printer } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useAuth } from "../auth";
import { useLoad, usePagedList } from "../hooks";
import { dateTime } from "../lib";
import { PrintHeader, SignatureArea, triggerPrint } from "../components/print";
import { DocumentViewer } from "../components/document-viewer";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "../components/ui/dialog";
import { Field, FieldGroup, Input, Select, Textarea } from "../components/ui/input";
import { PatientPicker } from "../components/patient-search";
import { CatalogPicker } from "../components/catalogs";
import { Badge, EmptyState, ErrorState, Pager, Skeleton, Table, Td, Th } from "../components/ui/data";

interface Prescription { id: string; prescriptionNumber: string; patientId: string; patientName: string; standalone: boolean; type: "spectacle" | "contact_lens" | "medication"; od: Record<string, string>; os: Record<string, string>; details: Record<string, string>; notes: string; issuedAt: string; expiresAt: string; doctor: string; signed: boolean; signedBy: string; signedAt: string }

export function PrescriptionsPage() {
  const { user } = useAuth();
  const prescriptions = usePagedList<Prescription>((page, limit) => `/prescriptions?page=${page}&limit=${limit}`);
  const [selected, setSelected] = React.useState<Prescription | null>(null);
  const [issuing, setIssuing] = React.useState(false);
  return <div className="page"><div className="flex flex-wrap items-end justify-between gap-4"><div><p className="section-title">Clinical documents</p><h1 className="page-title">Prescriptions</h1><p className="page-description">Doctor-issued spectacle, contact lens and medication prescriptions.</p></div>{user?.role === "doctor" && <Dialog open={issuing} onOpenChange={setIssuing}><DialogTrigger asChild><Button><Plus className="h-4 w-4" />New prescription</Button></DialogTrigger>{issuing && <StandalonePrescriptionForm onIssued={(created) => { setIssuing(false); prescriptions.reload(); setSelected(created); }} />}</Dialog>}</div><Card className="mt-6"><CardHeader><CardTitle>Prescription register</CardTitle><CardDescription>Every printable document uses the configured clinic identity and logo.</CardDescription></CardHeader><CardContent>{prescriptions.loading ? <Skeleton className="h-72" /> : prescriptions.error ? <ErrorState message={prescriptions.error.message} retry={prescriptions.reload} /> : prescriptions.items.length ? <><div className="hidden md:block"><Table><thead><tr><Th>Prescription</Th><Th>Patient</Th><Th>Type</Th><Th>Issued</Th><Th>Doctor</Th><Th>Origin</Th><Th>Action</Th></tr></thead><tbody>{prescriptions.items.map((item) => <tr key={item.id}><Td className="font-mono text-xs font-bold">{item.prescriptionNumber}</Td><Td>{item.patientName}</Td><Td><Badge>{item.type.replaceAll("_", " ")}</Badge></Td><Td>{dateTime(item.issuedAt)}</Td><Td>{item.doctor}</Td><Td>{item.standalone ? <Badge tone="warning">Outside a consultation</Badge> : <Badge>From a consultation</Badge>}</Td><Td><Button size="sm" variant="outline" onClick={() => setSelected(item)}><Printer className="h-3 w-3" />Open</Button></Td></tr>)}</tbody></Table></div><div className="grid gap-3 md:hidden">{prescriptions.items.map((item) => <button className="rounded-lg border p-4 text-left" key={item.id} onClick={() => setSelected(item)}><div className="flex justify-between"><span className="font-mono text-xs font-bold">{item.prescriptionNumber}</span><Badge>{item.type.replaceAll("_", " ")}</Badge></div><div className="mt-2 font-semibold">{item.patientName}</div><div className="mt-1 text-xs text-zinc-500">{item.doctor} · {dateTime(item.issuedAt)}</div>{item.standalone && <div className="mt-2"><Badge tone="warning">Outside a consultation</Badge></div>}</button>)}</div><Pager page={prescriptions.page} pageSize={prescriptions.pageSize} total={prescriptions.total} hasMore={prescriptions.hasMore} onPrevious={prescriptions.previous} onNext={prescriptions.next} /></> : <EmptyState title="No prescriptions" description="Doctor-issued prescriptions appear here." />}</CardContent></Card><Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>{selected && <PrescriptionPrint prescription={selected} />}</Dialog></div>;
}

const valueFor = (values: Record<string, string>, eye: "od" | "os", field: string) => values[field] ?? values[`${eye}${field}`] ?? values[`${eye}${field}`.toLowerCase()] ?? "—";
function PrescriptionPrint({ prescription }: { prescription: Prescription }) {
  const fields = prescription.type === "contact_lens" ? ["power", "bc", "dia", "cylinder", "axis", "add"] : ["sphere", "cylinder", "axis", "add", "prism", "base"];
  // The signature applied when this prescription was issued, fetched through the
  // authenticated client because a plain <img src> carries no desktop session.
  const [signature, setSignature] = React.useState("");
  React.useEffect(() => {
    if (!prescription.signed) return;
    let objectURL = "";
    let active = true;
    api.blob(`/prescriptions/${prescription.id}/signature`)
      .then(({ blob }) => { if (!active) return; objectURL = URL.createObjectURL(blob); setSignature(objectURL); })
      .catch(() => undefined);
    return () => { active = false; setSignature(""); if (objectURL) URL.revokeObjectURL(objectURL); };
  }, [prescription.id, prescription.signed]);
  return <DialogContent className="max-w-3xl"><DialogHeader className="no-print"><div className="flex items-center justify-between pr-8"><DialogTitle>{prescription.patientName}</DialogTitle><Button variant="outline" onClick={triggerPrint}><Printer className="h-4 w-4" />Print / PDF</Button></div></DialogHeader><article className="print-area rounded-lg border p-6"><PrintHeader documentTitle={`${prescription.type.replaceAll("_", " ")} prescription`} number={prescription.prescriptionNumber} date={dateTime(prescription.issuedAt)} /><section className="mt-6"><div className="text-xs font-bold uppercase tracking-wide text-zinc-500">Patient</div><div className="mt-1 text-lg font-bold">{prescription.patientName}</div>{prescription.standalone && <p className="mt-2 text-xs text-zinc-600">Issued outside a consultation.</p>}</section>{prescription.type !== "medication" ? <><div className="mt-6 grid grid-cols-[1fr_1fr_1fr] overflow-hidden rounded-md border text-sm"><div className="bg-zinc-50 p-3" /><div className="bg-zinc-50 p-3 text-center font-mono font-bold">OD</div><div className="bg-zinc-50 p-3 text-center font-mono font-bold">OS</div>{fields.map((field) => <React.Fragment key={field}><div className="border-t bg-zinc-50 p-3 font-semibold capitalize">{field}</div><div className="border-l border-t p-3 text-center font-mono">{valueFor(prescription.od, "od", field)}</div><div className="border-l border-t p-3 text-center font-mono">{valueFor(prescription.os, "os", field)}</div></React.Fragment>)}</div>{Object.entries(prescription.details).filter(([, value]) => value).length > 0 && <div className="mt-5 grid gap-2 rounded-md border p-4 text-sm sm:grid-cols-2">{Object.entries(prescription.details).filter(([, value]) => value).map(([key, value]) => <div key={key}><span className="capitalize text-zinc-500">{key.replaceAll(/([A-Z])/g, " $1")}: </span><strong>{String(value)}</strong></div>)}</div>}</> : <div className="mt-6 grid gap-3">{Object.entries(prescription.details).filter(([, value]) => value).map(([key, value]) => <div className="grid grid-cols-[150px_1fr] border-b pb-2 text-sm" key={key}><span className="font-semibold capitalize text-zinc-500">{key}</span><strong>{String(value)}</strong></div>)}</div>}{prescription.notes && <div className="mt-6 rounded-md bg-zinc-50 p-4 text-sm"><strong>Notes</strong><p className="mt-1 whitespace-pre-wrap">{prescription.notes}</p></div>}{prescription.expiresAt && <p className="mt-4 text-sm"><strong>Expires:</strong> {prescription.expiresAt}</p>}<SignatureArea doctor={prescription.signedBy || prescription.doctor} signatureSource={signature} signedAt={prescription.signedAt ? dateTime(prescription.signedAt) : ""} /></article></DialogContent>;
}

export interface DocumentItem { id: string; patientId: string; category: string; displayName: string; mediaType: string; sizeBytes: number; createdAt: string }

/** One row that opens the document in the in-app viewer. */
export function DocumentRow({ item, onOpen }: { item: DocumentItem; onOpen(item: DocumentItem): void }) {
  return <div className="flex items-center gap-3 py-3">
    <div className="grid h-10 w-10 shrink-0 place-items-center rounded-md bg-zinc-100"><FileText className="h-4 w-4" /></div>
    <div className="min-w-0 flex-1"><div className="truncate font-semibold">{item.displayName}</div><div className="text-xs text-zinc-500">{item.category} · {(item.sizeBytes / 1024).toFixed(1)} KB · {dateTime(item.createdAt)}</div></div>
    <Button variant="outline" size="sm" onClick={() => onOpen(item)}><Eye className="h-3 w-3" />Open</Button>
  </div>;
}

export function DocumentsPage() {
  const documents = usePagedList<DocumentItem>((page, limit) => `/documents?page=${page}&limit=${limit}`);
  const [viewing, setViewing] = React.useState<DocumentItem | null>(null);
  return <div className="page"><div><p className="section-title">Local file storage</p><h1 className="page-title">Documents</h1><p className="page-description">Files live on the clinic server filesystem; metadata stays in SQLite.</p></div><Card className="mt-6"><CardHeader><CardTitle>Patient documents</CardTitle><CardDescription>Upload new files from the patient profile.</CardDescription></CardHeader><CardContent>{documents.loading ? <Skeleton className="h-72" /> : documents.error ? <ErrorState message={documents.error.message} retry={documents.reload} /> : documents.items.length ? <><div className="divide-y">{documents.items.map((item) => <DocumentRow key={item.id} item={item} onOpen={setViewing} />)}</div><Pager page={documents.page} pageSize={documents.pageSize} total={documents.total} hasMore={documents.hasMore} onPrevious={documents.previous} onNext={documents.next} /></> : <EmptyState title="No documents" description="PDF, JPG, PNG, TIFF, DOC and DOCX files uploaded from patient charts appear here." />}</CardContent></Card><DocumentViewer document={viewing} onClose={() => setViewing(null)} /></div>;
}

/**
 * D4: a prescription issued outside a consultation. Same document, same
 * signature, same patient file — it simply has no encounter behind it, and both
 * the register and the printed page say so.
 */
function StandalonePrescriptionForm({ onIssued }: { onIssued(created: Prescription): void }) {
  const [patientId, setPatientId] = React.useState("");
  // Two ways in: pick somebody already on file, or type the details of somebody
  // who is not. The second path registers them as this prescription is issued,
  // so no one has to leave for the patient screen and come back.
  const [mode, setMode] = React.useState<"existing" | "new">("existing");
  const [newPatient, setNewPatient] = React.useState({ firstName: "", lastName: "", sex: "", dateOfBirth: "", phone: "", email: "", address: "", city: "" });
  const [duplicate, setDuplicate] = React.useState<{ patientId: string; medicalRecordNumber: string } | null>(null);
  const [type, setType] = React.useState<Prescription["type"]>("spectacle");
  const [od, setOd] = React.useState<Record<string, string>>({});
  const [os, setOs] = React.useState<Record<string, string>>({});
  const [medication, setMedication] = React.useState<Record<string, string>>({ medication: "", strength: "", dosage: "", frequency: "", route: "", duration: "", instructions: "" });
  const [notes, setNotes] = React.useState("");
  const [expiresAt, setExpiresAt] = React.useState("");
  const [saving, setSaving] = React.useState(false);

  const opticalFields = type === "contact_lens" ? ["power", "bc", "dia", "cylinder", "axis", "add"] : ["sphere", "cylinder", "axis", "add", "prism", "base"];

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    setDuplicate(null);
    try {
      const who = mode === "new" ? { newPatient: { ...newPatient, tags: [] } } : { patientId };
      const body = type === "medication"
        ? { ...who, encounterId: "", type, od: {}, os: {}, details: medication, notes, expiresAt }
        : { ...who, encounterId: "", type, od, os, details: {}, notes, expiresAt };
      const created = await api.post<{ id: string; prescriptionNumber: string; patientId: string; registeredPatient?: { id: string; medicalRecordNumber: string } }>("/prescriptions", body);
      toast.success(created.registeredPatient ? `Prescription ${created.prescriptionNumber} issued and patient ${created.registeredPatient.medicalRecordNumber} registered` : `Prescription ${created.prescriptionNumber} issued`);
      // Reload the full record so the print view has the signature and origin.
      const list = await api.get<{ items: Prescription[] }>(`/prescriptions?patientId=${encodeURIComponent(created.patientId)}`);
      const full = list.items.find((item) => item.id === created.id);
      if (full) onIssued(full);
    } catch (reason) {
      // A near-match on somebody already on file is offered, not just refused:
      // one click switches to their record instead of creating a second one.
      if (reason instanceof APIError && reason.body.code === "POSSIBLE_DUPLICATE_PATIENT") {
        const details = reason.body.details as { patientId?: string; medicalRecordNumber?: string } | undefined;
        if (details?.patientId) { setDuplicate({ patientId: details.patientId, medicalRecordNumber: details.medicalRecordNumber ?? "" }); return; }
      }
      toast.error(reason instanceof APIError ? reason.body.message : "Could not issue the prescription");
    } finally {
      setSaving(false);
    }
  };

  return <DialogContent className="max-w-3xl">
    <DialogHeader>
      <DialogTitle>New prescription</DialogTitle>
      <DialogDescription>For anyone not in a consultation — an existing patient, or somebody who is not on file yet and is registered as the prescription is issued. Either way it is filed in their record and marked as issued outside a consultation.</DialogDescription>
    </DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <div className="flex gap-1 rounded-md bg-[var(--muted)] p-1" role="tablist">
        {([["existing", "Existing patient"], ["new", "New person"]] as const).map(([value, label]) => (
          <button key={value} type="button" role="tab" aria-selected={mode === value} onClick={() => { setMode(value); setDuplicate(null); }} className={`min-h-10 flex-1 rounded-[var(--radius)] px-3 text-sm font-semibold ${mode === value ? "bg-[var(--card)] shadow-sm" : "text-[var(--muted-foreground)]"}`}>{label}</button>
        ))}
      </div>
      {mode === "existing"
        ? <FieldGroup label="Patient"><PatientPicker required value={patientId} onChange={setPatientId} /></FieldGroup>
        : <fieldset className="grid gap-3 rounded-md border p-3 sm:grid-cols-2">
            <legend className="px-1 text-xs font-bold uppercase">Register this person</legend>
            <Field label="First name"><Input required value={newPatient.firstName} onChange={(event) => setNewPatient({ ...newPatient, firstName: event.target.value })} /></Field>
            <Field label="Last name"><Input required value={newPatient.lastName} onChange={(event) => setNewPatient({ ...newPatient, lastName: event.target.value })} /></Field>
            <Field label="Date of birth"><Input type="date" value={newPatient.dateOfBirth} onChange={(event) => setNewPatient({ ...newPatient, dateOfBirth: event.target.value })} /></Field>
            <Field label="Sex"><Select value={newPatient.sex} onChange={(event) => setNewPatient({ ...newPatient, sex: event.target.value })}><option value="">Not recorded</option><option value="female">Female</option><option value="male">Male</option><option value="other">Other</option></Select></Field>
            <Field label="Phone"><Input value={newPatient.phone} onChange={(event) => setNewPatient({ ...newPatient, phone: event.target.value })} /></Field>
            <Field label="Email"><Input type="email" value={newPatient.email} onChange={(event) => setNewPatient({ ...newPatient, email: event.target.value })} /></Field>
            <Field label="Address"><Input value={newPatient.address} onChange={(event) => setNewPatient({ ...newPatient, address: event.target.value })} /></Field>
            <Field label="City"><Input value={newPatient.city} onChange={(event) => setNewPatient({ ...newPatient, city: event.target.value })} /></Field>
          </fieldset>}
      {duplicate && <div role="alert" className="grid gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900"><span><strong>Someone with these details is already on file</strong> as {duplicate.medicalRecordNumber}.</span><div><Button type="button" size="sm" variant="outline" onClick={() => { setPatientId(duplicate.patientId); setMode("existing"); setDuplicate(null); }}>Use the existing record</Button></div></div>}
      <Field label="Prescription type">
        <Select value={type} onChange={(event) => setType(event.target.value as Prescription["type"])}>
          <option value="spectacle">Spectacle</option><option value="contact_lens">Contact lens</option><option value="medication">Medication</option>
        </Select>
      </Field>
      {type === "medication" ? (
        <>
          <CatalogPicker
            catalog="prescription_item"
            label="Clinic prescription catalog"
            hint="Selecting an entry fills the fields below with its defaults; every field stays editable."
            onSelect={(entry) => setMedication({ ...medication, medication: entry.label, ...Object.fromEntries(Object.entries(entry.details).filter(([key]) => key in medication)) })}
          />
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {(["medication", "strength", "dosage", "frequency", "route", "duration"] as const).map((field) => (
              <Field key={field} label={field.charAt(0).toUpperCase() + field.slice(1)}>
                <Input required={field === "medication" || field === "dosage"} value={medication[field]} onChange={(event) => setMedication({ ...medication, [field]: event.target.value })} />
              </Field>
            ))}
          </div>
          <Field label="Instructions"><Textarea value={medication.instructions} onChange={(event) => setMedication({ ...medication, instructions: event.target.value })} /></Field>
        </>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {(["od", "os"] as const).map((eye) => (
            <fieldset key={eye} className="grid gap-2 rounded-md border p-3">
              <legend className="px-1 font-mono text-xs font-bold uppercase">{eye}</legend>
              {opticalFields.map((field) => (
                <Field key={field} label={field.charAt(0).toUpperCase() + field.slice(1)}>
                  <Input
                    value={(eye === "od" ? od : os)[field] ?? ""}
                    onChange={(event) => (eye === "od" ? setOd({ ...od, [field]: event.target.value }) : setOs({ ...os, [field]: event.target.value }))}
                  />
                </Field>
              ))}
            </fieldset>
          ))}
        </div>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Expires on" hint="Optional"><Input type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></Field>
        <Field label="Notes"><Input value={notes} onChange={(event) => setNotes(event.target.value)} /></Field>
      </div>
      <p className="rounded-md bg-zinc-50 p-3 text-xs text-zinc-600">
        Your stored signature is applied when this is issued, with your name and the time. Add or change it in System → My signature.
      </p>
      <DialogFooter>
        <Button type="submit" disabled={saving || (mode === "existing" ? !patientId : !newPatient.firstName.trim() || !newPatient.lastName.trim())}>{saving ? "Issuing…" : mode === "new" ? "Register and issue" : "Issue and open for printing"}</Button>
      </DialogFooter>
    </form>
  </DialogContent>;
}
