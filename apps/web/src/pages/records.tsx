import * as React from "react";
import { Eye, FileText, Printer } from "lucide-react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { dateTime } from "../lib";
import { PrintHeader, SignatureArea, triggerPrint } from "../components/print";
import { DocumentViewer } from "../components/document-viewer";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "../components/ui/dialog";
import { Badge, EmptyState, ErrorState, Skeleton, Table, Td, Th } from "../components/ui/data";

interface Prescription { id: string; prescriptionNumber: string; patientName: string; type: "spectacle" | "contact_lens" | "medication"; od: Record<string, string>; os: Record<string, string>; details: Record<string, string>; notes: string; issuedAt: string; expiresAt: string; doctor: string }

export function PrescriptionsPage() {
  const prescriptions = useLoad(() => api.get<{ items: Prescription[] }>("/prescriptions")); const [selected, setSelected] = React.useState<Prescription | null>(null);
  return <div className="page"><div><p className="section-title">Clinical documents</p><h1 className="page-title">Prescriptions</h1><p className="page-description">Doctor-issued spectacle, contact lens and medication prescriptions.</p></div><Card className="mt-6"><CardHeader><CardTitle>Prescription register</CardTitle><CardDescription>Every printable document uses the configured clinic identity and logo.</CardDescription></CardHeader><CardContent>{prescriptions.loading ? <Skeleton className="h-72" /> : prescriptions.error ? <ErrorState message={prescriptions.error.message} retry={prescriptions.reload} /> : prescriptions.data?.items.length ? <><div className="hidden md:block"><Table><thead><tr><Th>Prescription</Th><Th>Patient</Th><Th>Type</Th><Th>Issued</Th><Th>Doctor</Th><Th>Action</Th></tr></thead><tbody>{prescriptions.data.items.map((item) => <tr key={item.id}><Td className="font-mono text-xs font-bold">{item.prescriptionNumber}</Td><Td>{item.patientName}</Td><Td><Badge>{item.type.replaceAll("_", " ")}</Badge></Td><Td>{dateTime(item.issuedAt)}</Td><Td>{item.doctor}</Td><Td><Button size="sm" variant="outline" onClick={() => setSelected(item)}><Printer className="h-3 w-3" />Open</Button></Td></tr>)}</tbody></Table></div><div className="grid gap-3 md:hidden">{prescriptions.data.items.map((item) => <button className="rounded-lg border p-4 text-left" key={item.id} onClick={() => setSelected(item)}><div className="flex justify-between"><span className="font-mono text-xs font-bold">{item.prescriptionNumber}</span><Badge>{item.type.replaceAll("_", " ")}</Badge></div><div className="mt-2 font-semibold">{item.patientName}</div><div className="mt-1 text-xs text-zinc-500">{item.doctor} · {dateTime(item.issuedAt)}</div></button>)}</div></> : <EmptyState title="No prescriptions" description="Doctor-issued prescriptions appear here." />}</CardContent></Card><Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>{selected && <PrescriptionPrint prescription={selected} />}</Dialog></div>;
}

const valueFor = (values: Record<string, string>, eye: "od" | "os", field: string) => values[field] ?? values[`${eye}${field}`] ?? values[`${eye}${field}`.toLowerCase()] ?? "—";
function PrescriptionPrint({ prescription }: { prescription: Prescription }) {
  const fields = prescription.type === "contact_lens" ? ["power", "bc", "dia", "cylinder", "axis", "add"] : ["sphere", "cylinder", "axis", "add", "prism", "base"];
  return <DialogContent className="max-w-3xl"><DialogHeader className="no-print"><div className="flex items-center justify-between pr-8"><DialogTitle>{prescription.patientName}</DialogTitle><Button variant="outline" onClick={triggerPrint}><Printer className="h-4 w-4" />Print / PDF</Button></div></DialogHeader><article className="print-area rounded-lg border p-6"><PrintHeader documentTitle={`${prescription.type.replaceAll("_", " ")} prescription`} number={prescription.prescriptionNumber} date={dateTime(prescription.issuedAt)} /><section className="mt-6"><div className="text-xs font-bold uppercase tracking-wide text-zinc-500">Patient</div><div className="mt-1 text-lg font-bold">{prescription.patientName}</div></section>{prescription.type !== "medication" ? <><div className="mt-6 grid grid-cols-[1fr_1fr_1fr] overflow-hidden rounded-md border text-sm"><div className="bg-zinc-50 p-3" /><div className="bg-zinc-50 p-3 text-center font-mono font-bold">OD</div><div className="bg-zinc-50 p-3 text-center font-mono font-bold">OS</div>{fields.map((field) => <React.Fragment key={field}><div className="border-t bg-zinc-50 p-3 font-semibold capitalize">{field}</div><div className="border-l border-t p-3 text-center font-mono">{valueFor(prescription.od, "od", field)}</div><div className="border-l border-t p-3 text-center font-mono">{valueFor(prescription.os, "os", field)}</div></React.Fragment>)}</div>{Object.entries(prescription.details).filter(([, value]) => value).length > 0 && <div className="mt-5 grid gap-2 rounded-md border p-4 text-sm sm:grid-cols-2">{Object.entries(prescription.details).filter(([, value]) => value).map(([key, value]) => <div key={key}><span className="capitalize text-zinc-500">{key.replaceAll(/([A-Z])/g, " $1")}: </span><strong>{String(value)}</strong></div>)}</div>}</> : <div className="mt-6 grid gap-3">{Object.entries(prescription.details).filter(([, value]) => value).map(([key, value]) => <div className="grid grid-cols-[150px_1fr] border-b pb-2 text-sm" key={key}><span className="font-semibold capitalize text-zinc-500">{key}</span><strong>{String(value)}</strong></div>)}</div>}{prescription.notes && <div className="mt-6 rounded-md bg-zinc-50 p-4 text-sm"><strong>Notes</strong><p className="mt-1 whitespace-pre-wrap">{prescription.notes}</p></div>}{prescription.expiresAt && <p className="mt-4 text-sm"><strong>Expires:</strong> {prescription.expiresAt}</p>}<SignatureArea doctor={prescription.doctor} /></article></DialogContent>;
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
  const documents = useLoad(() => api.get<{ items: DocumentItem[] }>("/documents"));
  const [viewing, setViewing] = React.useState<DocumentItem | null>(null);
  return <div className="page"><div><p className="section-title">Local file storage</p><h1 className="page-title">Documents</h1><p className="page-description">Files live on the clinic server filesystem; metadata stays in SQLite.</p></div><Card className="mt-6"><CardHeader><CardTitle>Patient documents</CardTitle><CardDescription>Upload new files from the patient profile.</CardDescription></CardHeader><CardContent>{documents.loading ? <Skeleton className="h-72" /> : documents.error ? <ErrorState message={documents.error.message} retry={documents.reload} /> : documents.data?.items.length ? <div className="divide-y">{documents.data.items.map((item) => <DocumentRow key={item.id} item={item} onOpen={setViewing} />)}</div> : <EmptyState title="No documents" description="PDF, JPG, PNG, TIFF, DOC and DOCX files uploaded from patient charts appear here." />}</CardContent></Card><DocumentViewer document={viewing} onClose={() => setViewing(null)} /></div>;
}
