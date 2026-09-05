import * as React from "react";
import { Eye, EyeOff, Server, Wifi, WifiOff } from "lucide-react";
import { toast } from "sonner";
import { APIError } from "../api";
import { useAuth } from "../auth";
import { Button } from "./ui/button";
import { Field, Input, Select } from "./ui/input";

function Brand() {
  return <div className="flex items-center gap-3"><div className="grid h-10 w-10 place-items-center rounded-lg bg-black text-white"><Server className="h-5 w-5" /></div><div><div className="font-bold tracking-tight">SentryMed Opti</div><div className="text-xs text-zinc-500">Optical Clinic Management System</div></div></div>;
}

export function LoginScreen() {
  const { signIn } = useAuth();
  const [identity, setIdentity] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [visible, setVisible] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [available, setAvailable] = React.useState<boolean | null>(null);
  const [error, setError] = React.useState("");
  React.useEffect(() => { fetch("/health").then((response) => setAvailable(response.ok)).catch(() => setAvailable(false)); }, []);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setSubmitting(true); setError("");
    try { await signIn(identity, password); } catch (reason) { setError(reason instanceof APIError ? reason.body.message : "Could not contact the clinic server."); }
    finally { setSubmitting(false); }
  };
  return <main className="grid min-h-screen bg-white lg:grid-cols-2">
    <section className="flex items-center justify-center p-6 sm:p-10"><div className="w-full max-w-sm">
      <Brand />
      <div className="mt-12"><h1 className="text-3xl font-bold tracking-tight">Welcome back</h1><p className="mt-2 text-sm text-zinc-500">Sign in with your clinic account to continue.</p></div>
      <form className="mt-8 grid gap-5" onSubmit={submit}>
        <Field label="Email or username"><Input autoComplete="username" autoFocus value={identity} onChange={(event) => setIdentity(event.target.value)} required /></Field>
        <Field label="Password"><div className="relative"><Input className="pr-11" type={visible ? "text" : "password"} autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required /><button type="button" onClick={() => setVisible(!visible)} aria-label={visible ? "Hide password" : "Show password"} className="absolute right-1 top-1 grid h-9 w-9 place-items-center rounded text-zinc-500 hover:bg-zinc-100">{visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}</button></div></Field>
        {error && <div role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800">{error}</div>}
        <Button type="submit" disabled={submitting || available === false}>{submitting ? "Signing in…" : "Sign in"}</Button>
      </form>
      <div className="mt-6 flex items-center gap-2 text-xs text-zinc-500">{available === true ? <Wifi className="h-4 w-4 text-emerald-700" /> : available === false ? <WifiOff className="h-4 w-4 text-red-700" /> : <span className="h-2 w-2 animate-pulse rounded-full bg-zinc-400" />}{available === true ? "Clinic server available" : available === false ? "Clinic server unavailable" : "Checking clinic server…"}</div>
    </div></section>
    <aside className="relative hidden overflow-hidden bg-black p-12 text-white lg:flex lg:flex-col lg:justify-between"><div className="absolute inset-0 opacity-20" style={{ backgroundImage: "linear-gradient(#fff 1px,transparent 1px),linear-gradient(90deg,#fff 1px,transparent 1px)", backgroundSize: "48px 48px" }} /><div className="relative text-xs font-bold uppercase tracking-[.2em] text-zinc-400">Local-first · Private · Resilient</div><div className="relative max-w-xl"><p className="text-4xl font-semibold leading-tight tracking-[-.045em]">One coherent record from patient arrival to optical delivery.</p><p className="mt-5 max-w-lg text-zinc-400">Your clinic remains operational on the local network even when the Internet does not.</p></div><div className="relative font-mono text-xs text-zinc-500">SENTRYMED / CLINIC SERVER</div></aside>
  </main>;
}

export function SetupScreen() {
  const { completeSetup } = useAuth();
  const [step, setStep] = React.useState(1);
  const [submitting, setSubmitting] = React.useState(false);
  const [form, setForm] = React.useState({ clinicName: "", address: "", phone: "", email: "", doctorName: "", doctorUsername: "", doctorEmail: "", password: "", currency: "HTG", timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "America/Port-au-Prince" });
  const update = (key: keyof typeof form) => (event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setForm({ ...form, [key]: event.target.value });
  const finish = async () => { setSubmitting(true); try { await completeSetup(form); toast.success("Clinic setup complete. Sign in with the doctor account."); } catch (reason) { toast.error(reason instanceof Error ? reason.message : "Setup failed"); } finally { setSubmitting(false); } };
  return <main className="min-h-screen bg-zinc-50 p-4 sm:p-10"><div className="mx-auto max-w-2xl rounded-xl border border-zinc-200 bg-white p-6 shadow-sm sm:p-10"><Brand /><div className="mt-10"><div className="font-mono text-xs text-zinc-500">STEP {step} / 3</div><h1 className="mt-2 text-2xl font-bold tracking-tight">{step === 1 ? "Clinic information" : step === 2 ? "Doctor account" : "Local defaults"}</h1></div>
    <div className="mt-7 grid gap-5">
      {step === 1 && <><Field label="Clinic name"><Input value={form.clinicName} onChange={update("clinicName")} required /></Field><div className="grid gap-5 sm:grid-cols-2"><Field label="Phone"><Input value={form.phone} onChange={update("phone")} /></Field><Field label="Email"><Input type="email" value={form.email} onChange={update("email")} /></Field></div><Field label="Address"><Input value={form.address} onChange={update("address")} /></Field></>}
      {step === 2 && <><Field label="Doctor full name"><Input value={form.doctorName} onChange={update("doctorName")} required /></Field><div className="grid gap-5 sm:grid-cols-2"><Field label="Username"><Input value={form.doctorUsername} onChange={update("doctorUsername")} required /></Field><Field label="Email"><Input type="email" value={form.doctorEmail} onChange={update("doctorEmail")} /></Field></div><Field label="Password" hint="At least 12 characters. No universal password is installed."><Input type="password" minLength={12} value={form.password} onChange={update("password")} required /></Field></>}
      {step === 3 && <><Field label="Base currency"><Select value={form.currency} onChange={update("currency")}><option value="HTG">HTG — Haitian gourde</option><option value="USD">USD — US dollar</option></Select></Field><Field label="Clinic timezone"><Input value={form.timezone} onChange={update("timezone")} required /></Field><div className="rounded-lg border border-zinc-200 bg-zinc-50 p-4 text-sm"><strong>Backup destination</strong><p className="mt-1 text-zinc-500">The secure local SentryMed data directory will be used initially. You can add an external disk or network folder in Settings.</p></div></>}
    </div>
    <div className="mt-8 flex justify-between"><Button variant="outline" disabled={step === 1} onClick={() => setStep(step - 1)}>Back</Button>{step < 3 ? <Button onClick={() => setStep(step + 1)}>Continue</Button> : <Button disabled={submitting} onClick={finish}>{submitting ? "Preparing clinic…" : "Finish setup"}</Button>}</div>
  </div></main>;
}

