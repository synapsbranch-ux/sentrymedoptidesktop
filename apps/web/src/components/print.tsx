import * as React from "react";
import { useClinicIdentity } from "../clinic";

export function PrintHeader({ documentTitle, number, date }: { documentTitle: string; number: string; date: string }) {
  const clinic = useClinicIdentity(); const [logo, setLogo] = React.useState(true);
  return <header className="flex items-start justify-between gap-6 border-b-2 border-black pb-5"><div className="flex min-w-0 items-start gap-4">{logo && <img className="h-16 w-16 shrink-0 object-contain" src="/api/v1/branding/logo" alt="" onError={() => setLogo(false)} />}<div><h1 className="text-xl font-bold">{clinic.name || "Clinic"}</h1><p className="mt-1 max-w-md whitespace-pre-line text-xs text-zinc-600">{[clinic.address, clinic.phone, clinic.email].filter(Boolean).join(" · ")}</p>{clinic.nif && <p className="mt-1 font-mono text-xs">NIF: {clinic.nif}</p>}</div></div><div className="shrink-0 text-right"><div className="text-xs font-bold uppercase tracking-wider text-zinc-500">{documentTitle}</div><div className="mt-1 font-mono text-sm font-bold">{number}</div><div className="mt-1 text-xs text-zinc-500">{date}</div></div></header>;
}

export function SignatureArea({ doctor }: { doctor?: string }) { return <div className="ml-auto mt-14 w-64 border-t border-black pt-2 text-center text-xs"><strong>{doctor || "Doctor"}</strong><div className="mt-1 text-zinc-500">Signature</div></div>; }

export function triggerPrint() { window.setTimeout(() => window.print(), 50); }
