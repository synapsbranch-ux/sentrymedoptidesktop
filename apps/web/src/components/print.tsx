import * as React from "react";
import { clinicPrinting, DEFAULT_CLINIC_NAME, useClinicIdentity } from "../clinic";
import { printDocument } from "./printing";

export function PrintHeader({ documentTitle, number, date }: { documentTitle: string; number: string; date: string }) {
  const clinic = useClinicIdentity();
  const [logo, setLogo] = React.useState(true);
  return <header className="flex items-start justify-between gap-6 border-b-2 border-black pb-5">
    <div className="flex min-w-0 items-start gap-4">
      {logo && <img className="h-16 w-16 shrink-0 object-contain" src="/api/v1/public/branding/logo" alt="" onError={() => setLogo(false)} />}
      <div data-i18n-skip><h1 className="text-xl font-bold">{clinic.name || DEFAULT_CLINIC_NAME}</h1><p className="mt-1 max-w-md whitespace-pre-line text-xs text-zinc-600">{[clinic.address, clinic.phone, clinic.email].filter(Boolean).join(" · ")}</p>{clinic.nif && <p className="mt-1 font-mono text-xs">NIF: {clinic.nif}</p>}</div>
    </div>
    <div className="shrink-0 text-right"><div className="text-xs font-bold uppercase tracking-wider text-zinc-500">{documentTitle}</div><div className="mt-1 font-mono text-sm font-bold">{number}</div><div className="mt-1 text-xs text-zinc-500">{date}</div></div>
  </header>;
}

/**
 * D3: when the document carries a signature, the image applied at issue time is
 * shown above the line, with who signed and when. `signatureSource` is a URL the
 * caller has already fetched through the authenticated client.
 */
export function SignatureArea({ doctor, signatureSource, signedAt }: { doctor?: string; signatureSource?: string; signedAt?: string }) {
  return <div className="ml-auto mt-14 w-64 text-center text-xs">
    <div className="flex h-16 items-end justify-center">
      {signatureSource && <img alt="" src={signatureSource} className="max-h-16 w-auto object-contain" />}
    </div>
    <div className="border-t border-black pt-2"><strong>{doctor || "Doctor"}</strong>
      <div className="mt-1 text-zinc-500">{signedAt ? `Signed ${signedAt}` : "Signature"}</div>
    </div>
  </div>;
}

/**
 * Prints the standard-paper document already on screen. Receipts do not come
 * through here — they have their own path in `printing.tsx`, because a till roll
 * and a sheet of A4 are different page geometries.
 */
export function triggerPrint() { printDocument(clinicPrinting().documentPaper); }
