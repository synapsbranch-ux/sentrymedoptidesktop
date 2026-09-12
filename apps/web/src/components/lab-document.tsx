import * as React from "react";
import { Printer } from "lucide-react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { dateTime, money } from "../lib";
import type { LabOrderDocument } from "../types";
import { ImageGallery, StoredImageView } from "./image-gallery";
import { PrintHeader, triggerPrint } from "./print";
import { Button } from "./ui/button";
import { DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "./ui/dialog";
import { ErrorState, Skeleton } from "./ui/data";

/**
 * The sheet that leaves the clinic with an order.
 *
 * For a glazing company it has to answer one question — what exactly to make —
 * so it carries the powers, the lens specification, the fitting measurements and
 * a photograph of the frame. For a laboratory it carries the exams asked for.
 * Both are printed through the standard-paper path, which is also how the
 * operating system offers "Save as PDF"; there is no second, PDF-only layout to
 * drift out of step with this one.
 */

/**
 * The consultation composer stores powers under lowercase prefixed keys
 * (`odsphere`), while a prescription issued outside a consultation stores them
 * flat (`sphere`). Reading both is what lets one document print either.
 */
const valueFor = (values: Record<string, string> | undefined, eye: "od" | "os", field: string) =>
  values?.[field] ?? values?.[`${eye}${field}`] ?? values?.[`${eye}${field}`.toLowerCase()] ?? "—";

const opticalFields = ["sphere", "cylinder", "axis", "add", "prism", "base"] as const;

function Line({ label, value }: { label: string; value?: React.ReactNode }) {
  return <div><div className="text-[10px] font-bold uppercase tracking-wide text-zinc-500">{label}</div><div className="mt-0.5 text-sm">{value || "—"}</div></div>;
}

function Heading({ children }: { children: React.ReactNode }) {
  return <h3 className="mb-2 border-b border-zinc-300 pb-1 text-xs font-bold uppercase tracking-wider">{children}</h3>;
}

export function LabOrderDocumentDialog({ orderId }: { orderId: string }) {
  const document = useLoad(() => api.get<LabOrderDocument>(`/lab-orders/${orderId}`), [orderId]);
  const data = document.data;
  return <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto">
    <DialogHeader className="no-print">
      <div className="flex flex-wrap items-center justify-between gap-3 pr-8">
        <div>
          <DialogTitle>{data ? `${data.order.kind === "medical" ? "Laboratory request" : "Optical order"} ${data.order.orderNumber}` : "Lab order"}</DialogTitle>
          <DialogDescription>Print on paper, or save as PDF from the same dialog.</DialogDescription>
        </div>
        <Button variant="outline" disabled={!data} onClick={triggerPrint}><Printer className="h-4 w-4" />Print / PDF</Button>
      </div>
    </DialogHeader>
    {/* Reference photographs live with the order. The upload control stays off
        the paper; the pictures themselves are printed with the frame. */}
    <div className="no-print rounded-lg border p-4">
      <h3 className="mb-3 text-xs font-bold uppercase tracking-wider">Reference images</h3>
      <ImageGallery entityType="lab_order" entityId={orderId} canEdit emptyHint="No reference images on this order yet. A photograph of the frame is printed on the order." />
    </div>
    {document.loading && !data ? <Skeleton className="h-96" />
      : document.error ? <ErrorState message={document.error.message} retry={document.reload} />
      : data ? <LabOrderSheet document={data} /> : null}
    <DialogFooter className="no-print">
      <Button disabled={!data} onClick={triggerPrint}><Printer className="h-4 w-4" />Print / PDF</Button>
    </DialogFooter>
  </DialogContent>;
}

export function LabOrderSheet({ document: data }: { document: LabOrderDocument }) {
  const { order, patient, prescription, encounter, supplier, frame, lens, invoice, tests, imageIds } = data;
  const medical = order.kind === "medical";
  const measurements = Object.entries(order.measurements ?? {}).filter(([, value]) => value !== "" && value != null);
  return <article className="print-area document-print space-y-5 bg-white p-3 text-black sm:p-8">
    <PrintHeader
      documentTitle={medical ? "Laboratory examination request" : "Optical laboratory order"}
      number={order.orderNumber}
      date={order.orderedAt ? dateTime(order.orderedAt) : ""}
    />

    <section className="grid grid-cols-2 gap-4 border-y border-zinc-300 py-3 sm:grid-cols-4">
      <Line label="Patient" value={patient.name} />
      <Line label="Record" value={<span className="font-mono">{patient.medicalRecordNumber}</span>} />
      <Line label="Date of birth" value={patient.dateOfBirth} />
      <Line label="Phone" value={patient.phone} />
      <Line label="Status" value={order.status.replaceAll("_", " ")} />
      <Line label="Ordered" value={order.orderedAt ? dateTime(order.orderedAt) : ""} />
      <Line label="Expected" value={order.expectedAt ? dateTime(order.expectedAt) : ""} />
      <Line label="Consultation" value={encounter ? <span className="font-mono">{encounter.encounterNumber}</span> : ""} />
    </section>

    {!medical && (
      <section>
        <Heading>Prescription</Heading>
        {prescription ? <>
          <p className="mb-2 text-xs text-zinc-600">
            <span className="font-mono font-bold">{prescription.prescriptionNumber}</span>
            {prescription.doctor && ` · ${prescription.doctor}`}
            {prescription.issuedAt && ` · issued ${dateTime(prescription.issuedAt)}`}
          </p>
          {/* Every row is printed even when a power was not given: a workshop
              reading "—" knows it was left blank, where a missing row reads as a
              lens with no cylinder. */}
          <div className="grid grid-cols-[1fr_1fr_1fr] overflow-hidden rounded-md border border-zinc-300 text-sm">
            <div className="bg-zinc-50 p-2" />
            <div className="bg-zinc-50 p-2 text-center font-mono font-bold">OD</div>
            <div className="bg-zinc-50 p-2 text-center font-mono font-bold">OS</div>
            {opticalFields.map((field) => <React.Fragment key={field}>
              <div className="border-t border-zinc-300 bg-zinc-50 p-2 font-semibold capitalize">{field}</div>
              <div className="border-l border-t border-zinc-300 p-2 text-center font-mono">{valueFor(prescription.od, "od", field)}</div>
              <div className="border-l border-t border-zinc-300 p-2 text-center font-mono">{valueFor(prescription.os, "os", field)}</div>
            </React.Fragment>)}
          </div>
          {Object.entries(prescription.details ?? {}).filter(([, value]) => value).length > 0 && (
            <div className="mt-3 grid gap-2 rounded-md border border-zinc-300 p-3 text-sm sm:grid-cols-3">
              {Object.entries(prescription.details).filter(([, value]) => value).map(([key, value]) => (
                <div key={key}><span className="capitalize text-zinc-500">{key.replaceAll(/([A-Z])/g, " $1")}: </span><strong className="font-mono">{value}</strong></div>
              ))}
            </div>
          )}
          {prescription.notes && <p className="mt-2 whitespace-pre-wrap text-sm">{prescription.notes}</p>}
        </> : <p className="rounded-md border border-zinc-300 p-3 text-sm">No prescription is attached to this order — make to the frame and lenses specified below.</p>}
      </section>
    )}

    {medical ? (
      <section>
        <Heading>Examinations requested</Heading>
        {tests.length ? (
          <table className="w-full border-collapse text-sm">
            <thead><tr className="bg-zinc-50 text-left">
              <th className="border border-zinc-300 p-2">Examination</th>
              <th className="border border-zinc-300 p-2">Code</th>
              <th className="border border-zinc-300 p-2">Specimen</th>
              <th className="border border-zinc-300 p-2">Instructions</th>
            </tr></thead>
            <tbody>{tests.map((test, index) => <tr key={test.id ?? index}>
              <td className="border border-zinc-300 p-2 font-semibold">{test.label}</td>
              <td className="border border-zinc-300 p-2 font-mono">{test.code || "—"}</td>
              <td className="border border-zinc-300 p-2">{test.specimen || "—"}</td>
              <td className="border border-zinc-300 p-2">{test.notes || "—"}</td>
            </tr>)}</tbody>
          </table>
        ) : <p className="text-sm text-zinc-500">No examinations listed.</p>}
      </section>
    ) : (
      <>
        <section>
          <Heading>Lens specification</Heading>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
            <Line label="Lens" value={lens ? `${lens.name}${lens.brand ? ` · ${lens.brand}` : ""}` : order.lensType} />
            <Line label="Type" value={order.lensType} />
            <Line label="Material / index" value={order.material} />
            <Line label="Tint" value={order.tint} />
            <Line label="Coatings" value={order.coatings?.join(", ")} />
            <Line label="Treatments" value={order.treatments?.join(", ")} />
          </div>
        </section>

        <section>
          <Heading>Frame</Heading>
          <div className="flex flex-wrap items-start gap-5">
            <div className="grid min-w-52 flex-1 grid-cols-2 gap-3">
              <Line label="Frame" value={frame?.name} />
              <Line label="Brand" value={frame?.brand} />
              <Line label="Model" value={frame?.model} />
              <Line label="Reference" value={frame?.sku && <span className="font-mono">{frame.sku}</span>} />
            </div>
            {/* The workshop glazes the frame in front of it. The photograph is
                fetched through the authenticated client, like every other stored
                image, and printed beside the specification. */}
            {imageIds.length > 0 && (
              <div className="flex gap-2">
                {imageIds.slice(0, 2).map((id) => (
                  <figure key={id} className="w-40 overflow-hidden rounded-md border border-zinc-300">
                    <StoredImageView id={id} alt="Frame supplied for this order" className="grid h-32 w-full place-items-center bg-white" />
                  </figure>
                ))}
              </div>
            )}
          </div>
        </section>

        <section>
          <Heading>Fitting measurements</Heading>
          {measurements.length ? (
            <div className="grid grid-cols-2 gap-px border border-zinc-300 bg-zinc-300 sm:grid-cols-3">
              {measurements.map(([key, value]) => (
                <div className="bg-white p-2" key={key}>
                  <div className="text-[10px] font-bold uppercase text-zinc-500">{key.replaceAll(/([A-Z])/g, " $1")}</div>
                  <div className="font-mono text-sm">{String(value)}</div>
                </div>
              ))}
            </div>
          ) : <p className="text-sm text-zinc-500">No fitting measurements recorded.</p>}
        </section>
      </>
    )}

    {invoice && (
      <section>
        <Heading>Paid at the clinic</Heading>
        <p className="mb-2 text-xs text-zinc-600"><span className="font-mono font-bold">{invoice.invoiceNumber}</span> · {dateTime(invoice.date)}</p>
        <table className="w-full border-collapse text-sm">
          <tbody>
            {invoice.lines.map((line, index) => <tr key={index}>
              <td className="border border-zinc-300 p-2">{line.description}</td>
              <td className="border border-zinc-300 p-2 text-center font-mono">{line.quantity}</td>
              <td className="border border-zinc-300 p-2 text-right font-mono">{money(line.lineTotalMinor, invoice.currency)}</td>
            </tr>)}
            <tr className="font-bold">
              <td className="border border-zinc-300 p-2" colSpan={2}>Total</td>
              <td className="border border-zinc-300 p-2 text-right font-mono">{money(invoice.totalMinor, invoice.currency)}</td>
            </tr>
            {invoice.balanceMinor > 0 && <tr>
              <td className="border border-zinc-300 p-2" colSpan={2}>Still owed</td>
              <td className="border border-zinc-300 p-2 text-right font-mono">{money(invoice.balanceMinor, invoice.currency)}</td>
            </tr>}
          </tbody>
        </table>
      </section>
    )}

    <section className="grid gap-4 sm:grid-cols-2">
      <div>
        <Heading>{medical ? "Laboratory" : "Glazing company"}</Heading>
        {supplier ? <div className="text-sm">
          <div className="font-semibold">{supplier.company}</div>
          {supplier.contactPerson && <div>{supplier.contactPerson}</div>}
          {[supplier.phone, supplier.email].filter(Boolean).length > 0 && <div className="text-zinc-600">{[supplier.phone, supplier.email].filter(Boolean).join(" · ")}</div>}
          {supplier.address && <div className="whitespace-pre-line text-zinc-600">{supplier.address}</div>}
        </div> : <p className="text-sm text-zinc-500">Not assigned.</p>}
      </div>
      <div>
        <Heading>{medical ? "Clinical notes" : "Workshop notes"}</Heading>
        <p className="min-h-16 whitespace-pre-wrap border border-zinc-300 p-2 text-sm">{order.notes || "—"}</p>
      </div>
    </section>

    <div className="grid grid-cols-3 gap-6 pt-10 text-xs">
      <div className="border-t border-black pt-2">Released by (clinic)</div>
      <div className="border-t border-black pt-2">Received by ({medical ? "laboratory" : "workshop"})</div>
      <div className="border-t border-black pt-2">{medical ? "Result returned" : "Quality control"}</div>
    </div>
  </article>;
}
