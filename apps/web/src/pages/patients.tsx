import * as React from "react";
import { Camera, ChevronLeft, ChevronRight, Download, FileUp, ImageOff, Plus, Printer, Search, ShieldCheck, SlidersHorizontal, Trash2, UserRound, X } from "lucide-react";
import { toast } from "sonner";
import { api, APIError } from "../api";
import { useAuth } from "../auth";
import { saveBlob } from "../download";
import { useDebouncedValue, useLoad, usePagedList } from "../hooks";
import { dateTime } from "../lib";
import { useRealtime } from "../realtime";
import type { Patient, Payer, PatientPolicy, VerificationStatus } from "../types";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "../components/ui/dialog";
import { Badge, EmptyState, ErrorState, Pager, Skeleton, Table, Td, Th } from "../components/ui/data";
import { Field, Input, Select, Textarea } from "../components/ui/input";
import { PrintHeader, triggerPrint } from "../components/print";
import { DocumentViewer } from "../components/document-viewer";
import { DocumentRow, type DocumentItem } from "./records";
import { PatientFilterBar, activeFilterCount, emptyPatientFilters, patientAge, patientSearchQuery, type PatientSearchPage } from "../components/patient-search";
import { civilStatusOptions, civilStatusText, religionOptions, religionText } from "../components/demographics";

const emptyPatient = { firstName: "", middleName: "", lastName: "", preferredName: "", sex: "", dateOfBirth: "", phone: "", alternatePhone: "", email: "", address: "", city: "", occupation: "", employer: "", preferredLanguage: "", communicationPreference: "", referralSource: "", referringProvider: "", civilStatus: "", religion: "", religionOther: "", notes: "", tags: [] as string[] };
const pageSize = 25;

export function PatientsPage() {
  const { revision } = useRealtime();
  const [query, setQuery] = React.useState("");
  const [filters, setFilters] = React.useState(emptyPatientFilters);
  const [showFilters, setShowFilters] = React.useState(false);
  const [page, setPage] = React.useState(1);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [selected, setSelected] = React.useState<Patient | null>(null);
  // 300ms, so a search runs once the front desk stops typing rather than on
  // every keystroke, and never as a full load of the patients table.
  const debounced = useDebouncedValue(query, 300);
  React.useEffect(() => { setPage(1); }, [debounced, filters]);
  const patients = useLoad(() => api.get<PatientSearchPage>(patientSearchQuery(debounced, filters, page, pageSize)), [debounced, filters, page, revision]);
  React.useEffect(() => { const id = new URLSearchParams(location.search).get("id"); if (id) api.get<Patient>(`/patients/${id}`).then(setSelected).catch(() => undefined); }, []);
  const total = patients.data?.total ?? 0;
  const lastPage = Math.max(1, Math.ceil(total / pageSize));
  const filterCount = activeFilterCount(filters);
  return <div className="page"><div className="flex flex-wrap items-end justify-between gap-4"><div><p className="section-title">Patient records</p><h1 className="page-title">Patients</h1><p className="page-description">Demographics, history, clinical timeline, financial and optical records.</p></div><Dialog open={createOpen} onOpenChange={setCreateOpen}><DialogTrigger asChild><Button><Plus className="h-4 w-4" />New patient</Button></DialogTrigger><PatientForm onSaved={(patient) => { setCreateOpen(false); setSelected(patient); patients.reload(); }} /></Dialog></div>
    <div className="mt-6 flex flex-wrap items-center gap-3">
      <div className="relative min-w-0 flex-1 sm:max-w-xl"><Search aria-hidden className="pointer-events-none absolute left-3 top-3.5 h-4 w-4 text-zinc-400" /><Input className="pl-10" type="search" aria-label="Search patients" placeholder="Last name, first name, file number, phone, date of birth…" value={query} onChange={(event) => setQuery(event.target.value)} /></div>
      <Button variant={showFilters ? "default" : "outline"} onClick={() => setShowFilters((value) => !value)} aria-expanded={showFilters}><SlidersHorizontal className="h-4 w-4" />Filters{filterCount > 0 && <Badge className="ml-1">{filterCount}</Badge>}</Button>
    </div>
    {showFilters && <div className="mt-3"><PatientFilterBar filters={filters} onChange={setFilters} civilStatusOptions={civilStatusOptions} /></div>}
    <Card className="mt-4 overflow-hidden">{patients.loading ? <div className="p-5"><Skeleton className="h-72" /></div> : patients.error ? <div className="p-5"><ErrorState message={patients.error.message} retry={patients.reload} /></div> : !patients.data?.items.length ? <EmptyState title="No patients found" description={query || filterCount ? "No record matches this search. Try fewer words or clear the filters." : "Create the first patient to begin the clinic workflow."} action={!query && !filterCount && <Button onClick={() => setCreateOpen(true)}>Create patient</Button>} /> : <><div className="hidden md:block"><Table><thead><tr><Th>File no.</Th><Th>Name</Th><Th>Date of birth</Th><Th>Phone</Th><Th>Last visit</Th><Th>Tags</Th></tr></thead><tbody>{patients.data.items.map((patient) => <tr key={patient.id} className="cursor-pointer hover:bg-zinc-50" onClick={() => setSelected(patient)}><Td className="font-mono text-xs font-bold">{patient.medicalRecordNumber}</Td><Td><div className="font-semibold">{patient.firstName} {patient.lastName}</div><div className="text-xs text-zinc-500">{patient.email || "No email"}</div></Td><Td className="whitespace-nowrap">{patient.dateOfBirth || "—"}{patientAge(patient.dateOfBirth) !== null && <span className="ml-1 text-xs text-zinc-500">({patientAge(patient.dateOfBirth)})</span>}</Td><Td className="whitespace-nowrap">{patient.phone || "—"}</Td><Td className="whitespace-nowrap text-xs text-zinc-500">{patient.lastVisitAt ? dateTime(patient.lastVisitAt) : "Never seen"}</Td><Td><div className="flex flex-wrap gap-1">{patient.tags.map((tag) => <Badge key={tag}>{tag}</Badge>)}</div></Td></tr>)}</tbody></Table></div><div className="divide-y md:hidden">{patients.data.items.map((patient) => <button key={patient.id} onClick={() => setSelected(patient)} className="flex w-full items-center gap-3 p-4 text-left"><div className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-zinc-100"><UserRound className="h-4 w-4" /></div><div className="min-w-0 flex-1"><div className="font-semibold">{patient.firstName} {patient.lastName}</div><div className="font-mono text-xs text-zinc-500">{patient.medicalRecordNumber} · {patient.dateOfBirth || "—"} · {patient.phone || "No phone"}</div><div className="text-xs text-zinc-500">{patient.lastVisitAt ? `Last visit ${dateTime(patient.lastVisitAt)}` : "Never seen"}</div></div></button>)}</div></>}</Card>
    {total > 0 && <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-sm text-zinc-500"><span>{`${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, total)} of ${total}`}</span><div className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}><ChevronLeft className="h-3.5 w-3.5" />Previous</Button><span className="font-mono text-xs">{page} / {lastPage}</span><Button size="sm" variant="outline" disabled={!patients.data?.hasMore} onClick={() => setPage((value) => value + 1)}>Next<ChevronRight className="h-3.5 w-3.5" /></Button></div></div>}
    {selected && <PatientPanel patient={selected} onClose={() => setSelected(null)} onChanged={(patient) => { setSelected(patient); patients.reload(); }} />}
  </div>;
}

export function PatientForm({ patient, onSaved }: { patient?: Patient; onSaved(patient: Patient): void }) {
  const [form, setForm] = React.useState(() => Object.fromEntries(
    Object.entries(emptyPatient).map(([key, fallback]) => [key, patient?.[key as keyof typeof emptyPatient] ?? fallback]),
  ) as typeof emptyPatient); const [saving, setSaving] = React.useState(false); const [conflict, setConflict] = React.useState(false);
  const update = (key: keyof typeof emptyPatient) => (event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => setForm({ ...form, [key]: event.target.value });
  const submit = async (event: React.FormEvent) => { event.preventDefault(); if (saving) return; setSaving(true); setConflict(false); try { const result = patient ? await api.put<Patient>(`/patients/${patient.id}`, { ...form, version: patient.version }) : await api.post<Patient>("/patients", form); toast.success(patient ? "Patient updated" : "Patient created"); onSaved(result); } catch (reason) { if (reason instanceof APIError && reason.isConflict) setConflict(true); else toast.error(reason instanceof Error ? reason.message : "Could not save patient"); } finally { setSaving(false); } };
  return <DialogContent><DialogHeader><DialogTitle>{patient ? "Edit patient record" : "Register a new patient"}</DialogTitle><DialogDescription>Only identity is required now. Optional clinical and contact data can be completed later.</DialogDescription></DialogHeader><form onSubmit={submit}><div className="grid gap-4 sm:grid-cols-2"><Field label="First name"><Input required value={form.firstName} onChange={update("firstName")} /></Field><Field label="Last name"><Input required value={form.lastName} onChange={update("lastName")} /></Field><Field label="Middle name"><Input value={form.middleName} onChange={update("middleName")} /></Field><Field label="Preferred name"><Input value={form.preferredName} onChange={update("preferredName")} /></Field><Field label="Date of birth"><Input type="date" value={form.dateOfBirth} onChange={update("dateOfBirth")} /></Field><Field label="Sex"><Select value={form.sex} onChange={update("sex")}><option value="">Not specified</option><option value="female">Female</option><option value="male">Male</option><option value="other">Other</option></Select></Field><Field label="Phone"><Input value={form.phone} onChange={update("phone")} /></Field><Field label="Alternate phone"><Input value={form.alternatePhone} onChange={update("alternatePhone")} /></Field><Field label="Email"><Input type="email" value={form.email} onChange={update("email")} /></Field><Field label="City / area"><Input value={form.city} onChange={update("city")} /></Field><Field label="Address"><Input value={form.address} onChange={update("address")} /></Field><Field label="Civil status" hint="Optional"><Select value={form.civilStatus} onChange={update("civilStatus")}><option value="">Not recorded</option>{civilStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select></Field><Field label="Religion" hint="Optional"><Select value={form.religion} onChange={update("religion")}><option value="">Not recorded</option>{religionOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select></Field>{form.religion === "other" && <Field label="Religion — please specify"><Input maxLength={120} value={form.religionOther} onChange={update("religionOther")} /></Field>}<Field label="Occupation"><Input value={form.occupation} onChange={update("occupation")} /></Field><div className="sm:col-span-2"><Field label="Notes"><Textarea value={form.notes} onChange={update("notes")} /></Field></div></div>{conflict && <div role="alert" className="mt-4 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900"><strong>This record was modified by another user.</strong><p>Close and reload the latest patient record before saving.</p></div>}<DialogFooter><Button type="submit" disabled={saving}>{saving ? "Saving…" : patient ? "Save changes" : "Create patient"}</Button></DialogFooter></form></DialogContent>;
}

function PatientPanel({ patient, onClose, onChanged }: { patient: Patient; onClose(): void; onChanged(patient: Patient): void }) {
  const [editing, setEditing] = React.useState(false); const timeline = useLoad(() => api.get<{ items: { type: string; id: string; at: string; title: string }[] }>(`/patients/${patient.id}/timeline`), [patient.id]); const [uploading, setUploading] = React.useState(false); const [documentsRevision, setDocumentsRevision] = React.useState(0);
  const upload = async (event: React.ChangeEvent<HTMLInputElement>) => { const file = event.target.files?.[0]; if (!file) return; setUploading(true); const body = new FormData(); body.set("patientId", patient.id); body.set("category", "patient_document"); body.set("file", file); try { await api.post("/documents", body); toast.success("Document uploaded"); setDocumentsRevision((value) => value + 1); timeline.reload(); } catch (reason) { toast.error(reason instanceof Error ? reason.message : "Upload failed"); } finally { setUploading(false); event.target.value = ""; } };
  return <div className="fixed inset-0 z-40"><button aria-label="Close patient" className="absolute inset-0 bg-black/30" onClick={onClose} /><aside aria-label="Patient record" className="absolute inset-y-0 right-0 w-full max-w-2xl overflow-y-auto bg-white shadow-2xl"><div className="sticky top-0 z-10 flex items-center gap-3 border-b bg-white p-4 pt-[max(1rem,env(safe-area-inset-top))]"><div className="min-w-0 flex-1"><div className="font-mono text-xs font-bold text-zinc-500">{patient.medicalRecordNumber}</div><h2 className="truncate text-xl font-bold">{patient.firstName} {patient.lastName}</h2></div><Button variant="outline" onClick={triggerPrint}><Printer className="h-4 w-4" />Print</Button><Button variant="outline" onClick={() => setEditing(true)}>Edit</Button><Button size="icon" variant="ghost" onClick={onClose}><X className="h-5 w-5" /></Button></div><div className="grid gap-4 p-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:p-6"><Card><CardHeader><CardTitle>Patient overview</CardTitle></CardHeader><CardContent className="grid gap-4 text-sm sm:grid-cols-2"><Info label="Date of birth" value={patient.dateOfBirth} /><Info label="Phone" value={patient.phone} /><Info label="Email" value={patient.email} /><Info label="City" value={patient.city} /><Info label="Address" value={patient.address} /><Info label="Language" value={patient.preferredLanguage} /><Info label="Civil status" value={civilStatusText(patient.civilStatus)} /><Info label="Religion" value={religionText(patient.religion, patient.religionOther)} /></CardContent></Card><MedicalHistory patientId={patient.id} /><PatientInsurance patientId={patient.id} /><PatientDocuments patientId={patient.id} reloadKey={documentsRevision} uploading={uploading} onUpload={upload} /><Card><CardHeader><CardTitle>Longitudinal timeline</CardTitle></CardHeader><CardContent>{timeline.loading ? <Skeleton className="h-48" /> : timeline.data?.items.length ? <div className="relative ml-2 border-l border-zinc-200 pl-5">{timeline.data.items.map((item) => <div key={`${item.type}-${item.id}`} className="relative pb-5"><span className="absolute -left-[25px] top-1 h-2 w-2 rounded-full bg-black" /><div className="text-sm font-semibold">{item.title}</div><div className="mt-1 text-xs text-zinc-500">{dateTime(item.at)}</div></div>)}</div> : <EmptyState title="No patient activity yet" description="Appointments, consultations, prescriptions, payments and lab orders appear here." />}</CardContent></Card></div></aside><article className="print-only print-area document-print"><PrintHeader documentTitle="Patient summary" number={patient.medicalRecordNumber} date={new Date().toLocaleDateString()} /><h2 className="mt-6 text-2xl font-bold">{patient.firstName} {patient.middleName} {patient.lastName}</h2><div className="mt-6 grid grid-cols-2 gap-x-8 gap-y-4 text-sm">{[["Date of birth", patient.dateOfBirth], ["Sex", patient.sex], ["Civil status", civilStatusText(patient.civilStatus)], ["Religion", religionText(patient.religion, patient.religionOther)], ["Phone", patient.phone], ["Alternate phone", patient.alternatePhone], ["Email", patient.email], ["Language", patient.preferredLanguage], ["Address", [patient.address, patient.city].filter(Boolean).join(", ")], ["Occupation", patient.occupation], ["Employer", patient.employer], ["Referral source", patient.referralSource], ["Referring provider", patient.referringProvider]].map(([label, value]) => <div key={label}><div className="text-xs font-bold uppercase text-zinc-500">{label}</div><div className="mt-1">{value || "—"}</div></div>)}</div>{patient.notes && <div className="mt-6 border-t pt-4 text-sm"><strong>Notes</strong><p className="mt-2 whitespace-pre-wrap">{patient.notes}</p></div>}<section className="mt-8"><h3 className="border-b pb-2 font-bold">Recent timeline</h3><div className="mt-3 space-y-2">{timeline.data?.items.slice(0, 20).map((item) => <div className="flex justify-between gap-4 text-sm" key={`${item.type}-${item.id}`}><span>{item.title}</span><span className="text-zinc-500">{dateTime(item.at)}</span></div>)}</div></section></article><Dialog open={editing} onOpenChange={setEditing}>{editing && <PatientForm patient={patient} onSaved={(updated) => { setEditing(false); onChanged(updated); }} />}</Dialog></div>;
}
/** Files attached to this patient, opened in the in-app viewer. Before this the
 *  patient record listed no documents at all and offered no way to open one. */
function PatientDocuments({ patientId, reloadKey, uploading, onUpload }: { patientId: string; reloadKey: number; uploading: boolean; onUpload(event: React.ChangeEvent<HTMLInputElement>): void }) {
  const documents = usePagedList<DocumentItem>((page, limit) => `/documents?patientId=${encodeURIComponent(patientId)}&page=${page}&limit=${limit}`, [patientId, reloadKey], 25);
  const [viewing, setViewing] = React.useState<DocumentItem | null>(null);
  return <Card><CardHeader><CardTitle>Documents</CardTitle></CardHeader><CardContent>
    <label className="flex min-h-16 cursor-pointer items-center justify-center gap-2 rounded-md border border-dashed border-zinc-300 p-5 text-sm font-semibold hover:bg-zinc-50"><FileUp className="h-4 w-4" />{uploading ? "Uploading…" : "Upload PDF, image, scan or document"}<input className="sr-only" type="file" accept=".pdf,.jpg,.jpeg,.png,.tif,.tiff,.doc,.docx" onChange={onUpload} disabled={uploading} /></label>
    {documents.loading ? <Skeleton className="mt-4 h-24" /> : documents.error ? <div className="mt-4"><ErrorState message={documents.error.message} retry={documents.reload} /></div> : documents.items.length ? <><div className="mt-2 divide-y">{documents.items.map((item) => <DocumentRow key={item.id} item={item} onOpen={setViewing} />)}</div><Pager page={documents.page} pageSize={documents.pageSize} total={documents.total} hasMore={documents.hasMore} onPrevious={documents.previous} onNext={documents.next} /></> : <p className="mt-4 text-sm text-zinc-500">No documents attached to this patient yet.</p>}
    <DocumentViewer document={viewing} onClose={() => setViewing(null)} />
  </CardContent></Card>;
}

function Info({ label, value }: { label: string; value?: string }) { return <div><div className="text-xs font-semibold text-zinc-500">{label}</div><div className="mt-1">{value || "—"}</div></div>; }

interface HistoryRecord {
  chronicDiseases: string[]; diabetes: boolean | null; hypertension: boolean | null; cardiovascularNotes: string; neurologicalNotes: string; surgeries: string; pregnancyNotes: string; tobaccoUse: string; familyMedicalHistory: string; familyOcularHistory: string; previousEyeSurgery: string; ocularTrauma: string; glaucomaHistory: string; cataractHistory: string; retinalDisease: string; previousGlasses: string; previousContactLenses: string; version: number; updatedAt: string;
}

function MedicalHistory({ patientId }: { patientId: string }) {
  const history = useLoad(() => api.get<HistoryRecord>(`/patients/${patientId}/history`), [patientId]);
  const [editing, setEditing] = React.useState(false);
  if (history.loading) return <Card><CardContent className="p-5"><Skeleton className="h-28" /></CardContent></Card>;
  if (history.error || !history.data) return <Card><CardContent className="p-5"><ErrorState message={history.error?.message ?? "Medical history unavailable"} retry={history.reload} /></CardContent></Card>;
  const item = history.data;
  const highlights = [item.diabetes === true && "Diabetes", item.hypertension === true && "Hypertension", item.glaucomaHistory && "Glaucoma history", item.cataractHistory && "Cataract history", ...item.chronicDiseases].filter(Boolean) as string[];
  return <><Card><CardHeader className="flex-row items-center justify-between"><div><CardTitle>Medical & ocular history</CardTitle><p className="mt-1 text-xs text-zinc-500">Structured intake, independently versioned</p></div><Button size="sm" variant="outline" onClick={() => setEditing(true)}>Review</Button></CardHeader><CardContent>{highlights.length ? <div className="flex flex-wrap gap-2">{highlights.map((value) => <Badge key={value} tone="warning">{value}</Badge>)}</div> : <p className="text-sm text-zinc-500">No significant history recorded.</p>}</CardContent></Card><Dialog open={editing} onOpenChange={setEditing}><HistoryForm history={item} patientId={patientId} onSaved={() => { setEditing(false); history.reload(); }} /></Dialog></>;
}

function HistoryForm({ history, patientId, onSaved }: { history: HistoryRecord; patientId: string; onSaved(): void }) {
  const [form, setForm] = React.useState(history);
  const [chronic, setChronic] = React.useState(history.chronicDiseases.join(", "));
  const [saving, setSaving] = React.useState(false);
  const [conflict, setConflict] = React.useState(false);
  const update = (key: keyof HistoryRecord) => (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setForm({ ...form, [key]: event.target.value });
  const save = async (event: React.FormEvent) => { event.preventDefault(); setSaving(true); setConflict(false); try { await api.put(`/patients/${patientId}/history`, { ...form, chronicDiseases: chronic.split(",").map((value) => value.trim()).filter(Boolean) }); toast.success("Medical history updated"); onSaved(); } catch (reason) { if (reason instanceof APIError && reason.isConflict) setConflict(true); else toast.error(reason instanceof Error ? reason.message : "Could not update medical history"); } finally { setSaving(false); } };
  return <DialogContent><DialogHeader><DialogTitle>Medical & ocular history</DialogTitle><DialogDescription>Fields are optional. Unknown remains distinct from No.</DialogDescription></DialogHeader><form className="grid gap-4" onSubmit={save}><div className="grid gap-4 sm:grid-cols-2"><TriState label="Diabetes" value={form.diabetes} onChange={(diabetes) => setForm({ ...form, diabetes })} /><TriState label="Hypertension" value={form.hypertension} onChange={(hypertension) => setForm({ ...form, hypertension })} /></div><Field label="Chronic diseases" hint="Separate multiple conditions with commas"><Input value={chronic} onChange={(event) => setChronic(event.target.value)} /></Field><div className="grid gap-4 sm:grid-cols-2"><Field label="Cardiovascular history"><Textarea value={form.cardiovascularNotes} onChange={update("cardiovascularNotes")} /></Field><Field label="Neurological history"><Textarea value={form.neurologicalNotes} onChange={update("neurologicalNotes")} /></Field><Field label="Previous surgeries"><Textarea value={form.surgeries} onChange={update("surgeries")} /></Field><Field label="Tobacco use"><Textarea value={form.tobaccoUse} onChange={update("tobaccoUse")} /></Field><Field label="Family medical history"><Textarea value={form.familyMedicalHistory} onChange={update("familyMedicalHistory")} /></Field><Field label="Family ocular history"><Textarea value={form.familyOcularHistory} onChange={update("familyOcularHistory")} /></Field><Field label="Previous eye surgery"><Textarea value={form.previousEyeSurgery} onChange={update("previousEyeSurgery")} /></Field><Field label="Ocular trauma"><Textarea value={form.ocularTrauma} onChange={update("ocularTrauma")} /></Field><Field label="Glaucoma history"><Textarea value={form.glaucomaHistory} onChange={update("glaucomaHistory")} /></Field><Field label="Cataract history"><Textarea value={form.cataractHistory} onChange={update("cataractHistory")} /></Field><Field label="Retinal disease"><Textarea value={form.retinalDisease} onChange={update("retinalDisease")} /></Field><Field label="Pregnancy (when relevant)"><Textarea value={form.pregnancyNotes} onChange={update("pregnancyNotes")} /></Field><Field label="Previous glasses"><Textarea value={form.previousGlasses} onChange={update("previousGlasses")} /></Field><Field label="Previous contact lenses"><Textarea value={form.previousContactLenses} onChange={update("previousContactLenses")} /></Field></div>{conflict && <ConflictNotice />}<DialogFooter><Button type="submit" disabled={saving}>{saving ? "Saving…" : "Save history"}</Button></DialogFooter></form></DialogContent>;
}

function TriState({ label, value, onChange }: { label: string; value: boolean | null; onChange(value: boolean | null): void }) {
  return <Field label={label}><Select value={value === null ? "unknown" : value ? "yes" : "no"} onChange={(event) => onChange(event.target.value === "unknown" ? null : event.target.value === "yes")}><option value="unknown">Unknown / not assessed</option><option value="no">No</option><option value="yes">Yes</option></Select></Field>;
}

function ConflictNotice() { return <div role="alert" className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900"><strong>This history changed on another device.</strong><p>Close, reload and review the latest history before saving.</p></div>; }

const verificationTone = (status: VerificationStatus): "neutral" | "success" | "warning" | "danger" =>
  status === "verified" ? "success" : status === "rejected" || status === "expired" ? "danger" : status === "pending_verification" ? "warning" : "neutral";
const verificationLabel = (status: VerificationStatus) => status.replaceAll("_", " ");

/**
 * The patient's insurance: providers, coverage split, verification and the
 * card photos, all from the patient record so staff never re-enter what a
 * policy already says. A claim's split is proposed from a policy's coverage
 * percentage instead of being worked out by hand on every invoice.
 */
function PatientInsurance({ patientId }: { patientId: string }) {
  const { user } = useAuth();
  const doctor = user?.role === "doctor";
  const policies = useLoad(() => api.get<{ items: PatientPolicy[] }>(`/insurance/policies?patientId=${encodeURIComponent(patientId)}`), [patientId]);
  const [adding, setAdding] = React.useState(false);
  const [selected, setSelected] = React.useState<PatientPolicy | null>(null);
  const reload = () => policies.reload();
  return <Card>
    <CardHeader className="flex-row items-center justify-between"><CardTitle>Insurance</CardTitle>{doctor && <Button size="sm" variant="outline" onClick={() => setAdding(true)}><Plus className="h-3.5 w-3.5" />Add policy</Button>}</CardHeader>
    <CardContent className="grid gap-3">
      {policies.loading ? <Skeleton className="h-20" /> : policies.data?.items.length ? policies.data.items.map((policy) => (
        <button key={policy.id} className={`flex items-center justify-between gap-3 rounded-md border p-3 text-left text-sm hover:border-black ${!policy.isActive ? "opacity-50" : ""}`} onClick={() => setSelected(policy)}>
          <div>
            <div className="flex flex-wrap items-center gap-2"><strong>{policy.payerName}</strong>{policy.isPrimary && <Badge>Primary</Badge>}{!policy.isActive && <Badge tone="neutral">Inactive</Badge>}<Badge tone={verificationTone(policy.verificationStatus)}>{verificationLabel(policy.verificationStatus)}</Badge></div>
            <div className="mt-1 text-xs text-zinc-500">{[policy.policyNumber, policy.memberNumber].filter(Boolean).join(" · ") || "No policy number"}{policy.expirationDate && ` · Expires ${policy.expirationDate}`}</div>
          </div>
          <span className="font-mono font-bold">{policy.coveragePercent}%</span>
        </button>
      )) : <p className="text-sm text-zinc-500">No insurance on file. Without a policy, a claim's split has to be entered by hand.</p>}
    </CardContent>
    <Dialog open={adding} onOpenChange={setAdding}><PolicyForm patientId={patientId} onSaved={() => { setAdding(false); reload(); }} /></Dialog>
    <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>{selected && <PolicyDetail policy={selected} patientId={patientId} doctor={doctor} onChanged={(updated) => { reload(); setSelected(updated); }} onClose={() => setSelected(null)} />}</Dialog>
  </Card>;
}

function PayerSelect({ value, onChange }: { value: string; onChange(payerId: string): void }) {
  const payers = useLoad(() => api.get<{ items: Payer[] }>("/insurance/payers"));
  return <Select required value={value} onChange={(event) => onChange(event.target.value)}>
    <option value="">Select an insurer…</option>
    {payers.data?.items.filter((payer) => payer.active).map((payer) => <option key={payer.id} value={payer.id}>{payer.name}</option>)}
  </Select>;
}

/** A short, focused first form — the rest of a policy's detail (subscriber,
 *  dates, notes, card, verification) is completed afterward from its own
 *  detail dialog rather than in one long form up front. */
function PolicyForm({ patientId, onSaved }: { patientId: string; onSaved(): void }) {
  const payers = useLoad(() => api.get<{ items: Payer[] }>("/insurance/payers"));
  const [form, setForm] = React.useState({ payerId: "", policyNumber: "", memberNumber: "", coveragePercent: 80, isPrimary: true });
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const payer = payers.data?.items.find((candidate) => candidate.id === form.payerId);
    setSaving(true);
    try { await api.post(`/patients/${patientId}/insurance`, { ...form, payerName: payer?.name ?? "", coveragePercent: payer && !form.coveragePercent ? payer.defaultCoveragePercent : form.coveragePercent }); toast.success("Policy recorded"); onSaved(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not record the policy"); }
    finally { setSaving(false); }
  };
  return <DialogContent><DialogHeader><DialogTitle>Add insurance policy</DialogTitle><DialogDescription>Card photos, verification and other details can be added once the policy is created.</DialogDescription></DialogHeader>
    <form className="grid gap-4" onSubmit={submit}>
      <Field label="Insurer"><PayerSelect value={form.payerId} onChange={(payerId) => setForm({ ...form, payerId, coveragePercent: payers.data?.items.find((p) => p.id === payerId)?.defaultCoveragePercent ?? form.coveragePercent })} /></Field>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Coverage (%)"><Input type="number" min={0} max={100} value={form.coveragePercent} onChange={(event) => setForm({ ...form, coveragePercent: Number(event.target.value) })} /></Field>
        <Field label="Policy number"><Input value={form.policyNumber} onChange={(event) => setForm({ ...form, policyNumber: event.target.value })} /></Field>
        <Field label="Member number"><Input value={form.memberNumber} onChange={(event) => setForm({ ...form, memberNumber: event.target.value })} /></Field>
      </div>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.isPrimary} onChange={(event) => setForm({ ...form, isPrimary: event.target.checked })} />Primary insurance</label>
      <DialogFooter><Button type="submit" disabled={saving || !form.payerId}>{saving ? "Saving…" : "Record policy"}</Button></DialogFooter>
    </form>
  </DialogContent>;
}

type PolicyTab = "coverage" | "card" | "verification";

function PolicyDetail({ policy, patientId, doctor, onChanged, onClose }: { policy: PatientPolicy; patientId: string; doctor: boolean; onChanged(policy: PatientPolicy): void; onClose(): void }) {
  const [tab, setTab] = React.useState<PolicyTab>("coverage");
  const detail = useLoad(() => api.get<{ items: PatientPolicy[] }>(`/insurance/policies?patientId=${encodeURIComponent(patientId)}`), [patientId, policy.id]);
  const current = detail.data?.items.find((item) => item.id === policy.id) ?? policy;
  const refresh = () => detail.reload();
  React.useEffect(() => { if (detail.data) { const found = detail.data.items.find((item) => item.id === policy.id); if (found) onChanged(found); } }, [detail.data]); // eslint-disable-line react-hooks/exhaustive-deps
  return <DialogContent className="max-w-2xl">
    <DialogHeader><DialogTitle className="flex flex-wrap items-center gap-2">{current.payerName}{current.isPrimary && <Badge>Primary</Badge>}{!current.isActive && <Badge tone="neutral">Inactive</Badge>}</DialogTitle><DialogDescription>{[current.policyNumber, current.memberNumber].filter(Boolean).join(" · ") || "No policy number on file"}</DialogDescription></DialogHeader>
    <div className="flex gap-1 border-b pb-2">
      <Tab active={tab === "coverage"} onClick={() => setTab("coverage")}>Coverage</Tab>
      <Tab active={tab === "card"} onClick={() => setTab("card")}>Insurance card</Tab>
      <Tab active={tab === "verification"} onClick={() => setTab("verification")}>Verification</Tab>
    </div>
    {tab === "coverage" && <CoverageTab policy={current} patientId={patientId} doctor={doctor} onChanged={refresh} onClose={onClose} />}
    {tab === "card" && <CardTab policy={current} doctor={doctor} />}
    {tab === "verification" && <VerificationTab policy={current} patientId={patientId} doctor={doctor} onChanged={refresh} />}
  </DialogContent>;
}

function Tab({ active, onClick, children }: { active: boolean; onClick(): void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} className={`rounded-md px-3 py-1.5 text-sm font-semibold ${active ? "bg-black text-white" : "text-zinc-500 hover:bg-zinc-100"}`}>{children}</button>;
}

function CoverageTab({ policy, patientId, doctor, onChanged, onClose }: { policy: PatientPolicy; patientId: string; doctor: boolean; onChanged(): void; onClose(): void }) {
  const [form, setForm] = React.useState({
    payerId: policy.payerId, payerName: policy.payerName, policyNumber: policy.policyNumber, memberNumber: policy.memberNumber,
    groupNumber: policy.groupNumber, subscriberName: policy.subscriberName, relationshipToSubscriber: policy.relationshipToSubscriber,
    authorization: policy.authorization, coveragePercent: policy.coveragePercent, effectiveDate: policy.effectiveDate, expirationDate: policy.expirationDate,
    coverageNotes: policy.coverageNotes, notes: policy.notes,
  });
  const [saving, setSaving] = React.useState(false);
  const save = async (event: React.FormEvent) => {
    event.preventDefault(); setSaving(true);
    try { await api.put(`/patients/${patientId}/insurance/${policy.id}`, { ...form, version: policy.version }); toast.success("Policy updated"); onChanged(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not update the policy"); }
    finally { setSaving(false); }
  };
  const setPrimary = async () => { try { await api.post(`/patients/${patientId}/insurance/${policy.id}/set-primary`, { version: policy.version }); toast.success("Marked as primary insurance"); onChanged(); } catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not set as primary"); } };
  const toggleActive = async () => {
    const action = policy.isActive ? "deactivate" : "reactivate";
    try { await api.post(`/patients/${patientId}/insurance/${policy.id}/${action}`, { version: policy.version }); toast.success(policy.isActive ? "Policy deactivated" : "Policy reactivated"); onChanged(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not update the policy"); }
  };
  const remove = async () => {
    if (!window.confirm(`Remove this ${policy.payerName} policy? This cannot be undone.`)) return;
    try { await api.delete(`/patients/${patientId}/insurance/${policy.id}?version=${policy.version}`); toast.success("Policy removed"); onClose(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Remove the insurance card and any documents first, or deactivate this policy instead."); }
  };
  return <form className="grid gap-4 pt-4" onSubmit={save}>
    <div className="grid gap-4 sm:grid-cols-2">
      <Field label="Coverage (%)"><Input type="number" min={0} max={100} value={form.coveragePercent} onChange={(e) => setForm({ ...form, coveragePercent: Number(e.target.value) })} disabled={!doctor} /></Field>
      <Field label="Authorization / reference"><Input value={form.authorization} onChange={(e) => setForm({ ...form, authorization: e.target.value })} disabled={!doctor} /></Field>
      <Field label="Policy number"><Input value={form.policyNumber} onChange={(e) => setForm({ ...form, policyNumber: e.target.value })} disabled={!doctor} /></Field>
      <Field label="Member number"><Input value={form.memberNumber} onChange={(e) => setForm({ ...form, memberNumber: e.target.value })} disabled={!doctor} /></Field>
      <Field label="Group number"><Input value={form.groupNumber} onChange={(e) => setForm({ ...form, groupNumber: e.target.value })} disabled={!doctor} /></Field>
      <Field label="Subscriber name"><Input value={form.subscriberName} onChange={(e) => setForm({ ...form, subscriberName: e.target.value })} disabled={!doctor} /></Field>
      <Field label="Relationship to subscriber"><Select value={form.relationshipToSubscriber} onChange={(e) => setForm({ ...form, relationshipToSubscriber: e.target.value })} disabled={!doctor}><option value="">Not specified</option><option value="self">Self</option><option value="spouse">Spouse</option><option value="child">Child</option><option value="other">Other</option></Select></Field>
      <Field label="Effective date"><Input type="date" value={form.effectiveDate} onChange={(e) => setForm({ ...form, effectiveDate: e.target.value })} disabled={!doctor} /></Field>
      <Field label="Expiration date"><Input type="date" value={form.expirationDate} onChange={(e) => setForm({ ...form, expirationDate: e.target.value })} disabled={!doctor} /></Field>
    </div>
    <Field label="Coverage notes" hint="What this policy covers, exclusions, contract specifics."><Textarea value={form.coverageNotes} onChange={(e) => setForm({ ...form, coverageNotes: e.target.value })} disabled={!doctor} /></Field>
    <Field label="Notes"><Textarea value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} disabled={!doctor} /></Field>
    {doctor && <div className="flex flex-wrap gap-2 border-t pt-4">
      {!policy.isPrimary && policy.isActive && <Button type="button" variant="outline" size="sm" onClick={setPrimary}><ShieldCheck className="h-3.5 w-3.5" />Make primary</Button>}
      <Button type="button" variant="outline" size="sm" onClick={toggleActive}>{policy.isActive ? "Deactivate" : "Reactivate"}</Button>
      <Button type="button" variant="ghost" size="sm" className="ml-auto text-red-700 hover:text-red-700" onClick={remove}><Trash2 className="h-3.5 w-3.5" />Remove</Button>
    </div>}
    <DialogFooter><Button type="submit" disabled={saving || !doctor}>{saving ? "Saving…" : "Save changes"}</Button></DialogFooter>
  </form>;
}

function VerificationTab({ policy, patientId, doctor, onChanged }: { policy: PatientPolicy; patientId: string; doctor: boolean; onChanged(): void }) {
  const [status, setStatus] = React.useState<VerificationStatus>(policy.verificationStatus);
  const [reference, setReference] = React.useState(policy.verificationReference);
  const [contact, setContact] = React.useState(policy.verificationContact);
  const [notes, setNotes] = React.useState(policy.verificationNotes);
  const [saving, setSaving] = React.useState(false);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setSaving(true);
    try { await api.post(`/patients/${patientId}/insurance/${policy.id}/verify`, { status, reference, contact, notes, version: policy.version }); toast.success("Verification recorded"); onChanged(); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not record verification"); }
    finally { setSaving(false); }
  };
  return <form className="grid gap-4 pt-4" onSubmit={submit}>
    <div className="rounded-md border p-3 text-sm text-zinc-600">Verification is recorded manually by staff — this clinic has no electronic eligibility check. {policy.verifiedBy && <>Last verified by {policy.verifiedBy}{policy.verifiedAt && ` on ${dateTime(policy.verifiedAt)}`}.</>}</div>
    <Field label="Status"><Select value={status} onChange={(e) => setStatus(e.target.value as VerificationStatus)} disabled={!doctor}><option value="not_verified">Not verified</option><option value="pending_verification">Pending verification</option><option value="verified">Verified</option><option value="rejected">Rejected</option><option value="expired">Expired</option></Select></Field>
    <div className="grid gap-4 sm:grid-cols-2">
      <Field label="Authorization / reference number"><Input value={reference} onChange={(e) => setReference(e.target.value)} disabled={!doctor} /></Field>
      <Field label="Contact person"><Input value={contact} onChange={(e) => setContact(e.target.value)} disabled={!doctor} /></Field>
    </div>
    <Field label="Notes"><Textarea value={notes} onChange={(e) => setNotes(e.target.value)} disabled={!doctor} /></Field>
    <DialogFooter><Button type="submit" disabled={saving || !doctor}>{saving ? "Saving…" : "Record verification"}</Button></DialogFooter>
  </form>;
}

function CardTab({ policy, doctor }: { policy: PatientPolicy; doctor: boolean }) {
  return <div className="grid gap-4 pt-4 sm:grid-cols-2">
    <CardSide policy={policy} side="front" doctor={doctor} />
    <CardSide policy={policy} side="back" doctor={doctor} />
  </div>;
}

function CardSide({ policy, side, doctor }: { policy: PatientPolicy; side: "front" | "back"; doctor: boolean }) {
  const [revision, setRevision] = React.useState(0);
  const cards = useLoad(() => api.get<{ items: { id: string; side: string }[] }>(`/patient-insurance/${policy.id}/cards`), [policy.id, revision]);
  const present = cards.data?.items.some((card) => card.side === side) ?? false;
  const [preview, setPreview] = React.useState("");
  const [uploading, setUploading] = React.useState(false);
  const [fullscreen, setFullscreen] = React.useState(false);
  React.useEffect(() => {
    if (!present) { setPreview(""); return; }
    let active = true; let url = "";
    api.blob(`/patient-insurance/${policy.id}/cards/${side}/content`).then(({ blob }) => { if (!active) return; url = URL.createObjectURL(blob); setPreview(url); }).catch(() => undefined);
    return () => { active = false; if (url) URL.revokeObjectURL(url); };
  }, [present, policy.id, side, revision]);
  const upload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]; if (!file) return; setUploading(true);
    try { const body = new FormData(); body.set("file", file); await api.post(`/patient-insurance/${policy.id}/cards/${side}`, body); toast.success(`${side === "front" ? "Front" : "Back"} of card saved`); setRevision((v) => v + 1); }
    catch (reason) { toast.error(reason instanceof Error ? reason.message : "Could not save the image"); }
    finally { setUploading(false); event.target.value = ""; }
  };
  const download = async () => {
    try { const { blob } = await api.blob(`/patient-insurance/${policy.id}/cards/${side}/content?download=true`); await saveBlob(blob, `insurance-card-${side}.jpg`); }
    catch (reason) { toast.error(reason instanceof Error ? reason.message : "Could not download the image"); }
  };
  const remove = async () => {
    if (!window.confirm(`Delete the ${side} of this insurance card?`)) return;
    try { await api.delete(`/patient-insurance/${policy.id}/cards/${side}`); toast.success("Card image removed"); setRevision((v) => v + 1); }
    catch (reason) { toast.error(reason instanceof APIError ? reason.body.message : "Could not delete the image"); }
  };
  return <div className="grid gap-2">
    <div className="text-xs font-bold uppercase text-zinc-500">{side}</div>
    <div className="grid aspect-[16/10] place-items-center overflow-hidden rounded-lg border bg-zinc-50">
      {present && preview ? <img src={preview} alt={`Insurance card ${side}`} className="h-full w-full cursor-zoom-in object-contain" onClick={() => setFullscreen(true)} /> : <div className="grid place-items-center gap-1 text-zinc-400"><ImageOff className="h-6 w-6" /><span className="text-xs">Not uploaded</span></div>}
    </div>
    <div className="flex flex-wrap gap-2">
      <label className="flex min-h-9 flex-1 cursor-pointer items-center justify-center gap-1 rounded-md border px-2 text-xs font-semibold hover:bg-zinc-50"><Camera className="h-3.5 w-3.5" />{uploading ? "Saving…" : present ? "Replace" : "Take photo / upload"}<input className="sr-only" type="file" accept="image/*" capture="environment" onChange={upload} disabled={uploading} /></label>
      {present && <Button size="sm" variant="outline" onClick={download}><Download className="h-3.5 w-3.5" /></Button>}
      {present && doctor && <Button size="sm" variant="outline" onClick={remove}><Trash2 className="h-3.5 w-3.5" /></Button>}
    </div>
    {fullscreen && preview && <div className="fixed inset-0 z-[60] grid place-items-center bg-black/80 p-4" onClick={() => setFullscreen(false)}><img src={preview} alt={`Insurance card ${side}, full screen`} className="max-h-full max-w-full object-contain" /></div>}
  </div>;
}
