import * as React from "react";
import { Eye, EyeOff, Server, Wifi, WifiOff } from "lucide-react";
import { toast } from "sonner";
import { APIError } from "../api";
import { useAuth } from "../auth";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Field, Input, Select } from "./ui/input";
import { desktopBridge } from "../native";
import { supportedLanguages, useI18n } from "../i18n";

function Brand() {
  const [logo, setLogo] = React.useState(true);
  const { t } = useI18n();
  return <div className="flex items-center gap-3"><div className="grid h-10 w-10 place-items-center overflow-hidden rounded-[var(--radius)] bg-[var(--primary)] text-[var(--primary-foreground)]">{logo ? <img className="h-full w-full bg-white object-contain p-1" src="/api/v1/public/branding/logo" alt={t("Clinic logo")} onError={() => setLogo(false)} /> : <Server className="h-5 w-5" />}</div><div><div className="font-bold tracking-tight">SentryMed Opti</div><div className="text-xs text-zinc-500">{t("Optical Clinic Management System")}</div></div></div>;
}

export function LoginScreen() {
  const { signIn } = useAuth();
  const { t } = useI18n();
  const [identity, setIdentity] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [visible, setVisible] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [available, setAvailable] = React.useState<boolean | null>(null);
  const [error, setError] = React.useState("");
  React.useEffect(() => { fetch("/health").then((response) => setAvailable(response.ok)).catch(() => setAvailable(false)); }, []);
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setSubmitting(true); setError("");
    try { await signIn(identity, password); } catch (reason) { setError(t(reason instanceof APIError ? reason.body.message : "Could not contact the clinic server.")); }
    finally { setSubmitting(false); }
  };
  return <main className="flex min-h-svh w-full flex-col items-center justify-center gap-6 overflow-x-hidden bg-zinc-50 p-4 sm:p-6 md:p-10">
    <div className="flex w-full max-w-sm flex-col gap-6">
      <div className="self-center"><Brand /></div>
      <Card className="shadow-sm">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">Welcome back</CardTitle>
          <CardDescription>Sign in with your clinic account to continue.</CardDescription>
        </CardHeader>
        <CardContent><form className="grid gap-5" onSubmit={submit}>
        <Field label="Email or username"><Input autoComplete="username" autoFocus value={identity} onChange={(event) => setIdentity(event.target.value)} required /></Field>
        <div className="grid gap-1.5 text-sm font-medium text-zinc-800"><label htmlFor="login-password">{t("Password")}</label><div className="relative"><Input id="login-password" className="pr-11" type={visible ? "text" : "password"} autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required /><button type="button" onClick={() => setVisible(!visible)} aria-label={t(visible ? "Hide password" : "Show password")} aria-pressed={visible} className="absolute right-1 top-1 grid h-9 w-9 place-items-center rounded text-zinc-500 hover:bg-zinc-100">{visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}</button></div></div>
        {error && <div role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800">{error}</div>}
        <Button className="w-full" type="submit" disabled={submitting || available === false}>{submitting ? "Signing in…" : "Sign in"}</Button>
        <div className="flex items-center justify-center gap-2 text-xs text-zinc-500">{available === true ? <Wifi className="h-4 w-4 text-emerald-700" /> : available === false ? <WifiOff className="h-4 w-4 text-red-700" /> : <span className="h-2 w-2 animate-pulse rounded-full bg-zinc-400" />}{t(available === true ? "Clinic server available" : available === false ? "Clinic server unavailable" : "Checking clinic server…")}</div>
      </form></CardContent></Card>
      <p className="px-6 text-center text-xs leading-relaxed text-zinc-500">{t("Local-first clinic access. Your operational data remains on the SentryMed server.")}</p>
    </div>
  </main>;
}

export function SetupScreen() {
  const { completeSetup } = useAuth();
  const i18n = useI18n();
  const [step, setStep] = React.useState(1);
  const [submitting, setSubmitting] = React.useState(false);
  const [form, setForm] = React.useState({ clinicName: "", address: "", phone: "", email: "", doctorName: "", doctorUsername: "", doctorEmail: "", password: "", currency: "HTG", timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "America/Port-au-Prince", backupDirectory: "", language: i18n.language });
  const update = (key: keyof typeof form) => (event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => { const value = event.target.value; setForm({ ...form, [key]: value }); if (key === "language" && supportedLanguages.some(({ code }) => code === value)) i18n.apply(value as typeof i18n.language); };
  const finish = async () => { setSubmitting(true); try { await completeSetup(form); toast.success(i18n.t("Clinic setup complete. Sign in with the doctor account.")); } catch (reason) { toast.error(i18n.t(reason instanceof Error ? reason.message : "Setup failed")); } finally { setSubmitting(false); } };
  return <main className="min-h-screen bg-zinc-50 p-4 sm:p-10"><div className="mx-auto max-w-2xl rounded-xl border border-zinc-200 bg-white p-6 shadow-sm sm:p-10"><Brand /><div className="mt-10"><div className="font-mono text-xs text-zinc-500">{i18n.t("STEP")} {step} / 3</div><h1 className="mt-2 text-2xl font-bold tracking-tight">{i18n.t(step === 1 ? "Clinic information" : step === 2 ? "Doctor account" : "Local defaults")}</h1></div>
    <div className="mt-7 grid gap-5">
      {step === 1 && <><Field label="Clinic name"><Input value={form.clinicName} onChange={update("clinicName")} required /></Field><div className="grid gap-5 sm:grid-cols-2"><Field label="Phone"><Input value={form.phone} onChange={update("phone")} /></Field><Field label="Email"><Input type="email" value={form.email} onChange={update("email")} /></Field></div><Field label="Address"><Input value={form.address} onChange={update("address")} /></Field></>}
      {step === 2 && <><Field label="Doctor full name"><Input value={form.doctorName} onChange={update("doctorName")} required /></Field><div className="grid gap-5 sm:grid-cols-2"><Field label="Username"><Input value={form.doctorUsername} onChange={update("doctorUsername")} required /></Field><Field label="Email"><Input type="email" value={form.doctorEmail} onChange={update("doctorEmail")} /></Field></div><Field label="Password" hint="At least 12 characters. No universal password is installed."><Input type="password" minLength={12} value={form.password} onChange={update("password")} required /></Field></>}
      {step === 3 && <><Field label="Application language"><Select value={form.language} onChange={update("language")}>{supportedLanguages.map((language) => <option key={language.code} value={language.code}>{language.nativeLabel} — {language.label}</option>)}</Select></Field><Field label="Base currency"><Select value={form.currency} onChange={update("currency")}><option value="HTG">HTG — Haitian gourde</option><option value="USD">USD — US dollar</option></Select></Field><Field label="Clinic timezone"><Input value={form.timezone} onChange={update("timezone")} required /></Field><Field label="Backup destination" hint="Leave empty for SentryMed's secure local folder, or choose an external disk."><div className="flex gap-2"><Input className="font-mono" value={form.backupDirectory} onChange={update("backupDirectory")} placeholder="Default local backup folder" /><Button type="button" variant="outline" disabled={!desktopBridge()} onClick={async () => { const path = await desktopBridge()?.SelectBackupFolder(); if (path) setForm({ ...form, backupDirectory: path }); }}>Choose</Button></div></Field><div className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900"><strong>{i18n.t("Network permission")}</strong><p className="mt-1">{i18n.t("When the operating system asks, allow SentryMed Opti on private networks so clinic phones and laptops can connect.")}</p></div></>}
    </div>
    <div className="mt-8 flex justify-between"><Button variant="outline" disabled={step === 1} onClick={() => setStep(step - 1)}>Back</Button>{step < 3 ? <Button onClick={() => setStep(step + 1)}>Continue</Button> : <Button disabled={submitting} onClick={finish}>{submitting ? "Preparing clinic…" : "Finish setup"}</Button>}</div>
  </div></main>;
}
