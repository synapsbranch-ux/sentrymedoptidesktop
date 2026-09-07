import * as React from "react";
import { CalendarCheck, CheckCircle2, Glasses, Loader2 } from "lucide-react";

// The kiosk deliberately does not use the shared clinic i18n context: that context's
// `language` is the clinic-wide default (staff devices, the waiting-room display) and
// switching it triggers a refetch of `/public/localization` that would immediately
// snap back over a visitor's one-off choice here. A kiosk visitor's language pick is
// local to this browser session only, so it gets its own tiny, self-contained dictionary.
const kioskStrings = {
  en: {
    selfServiceCheckIn: "Self-service check-in",
    welcomeTitle: "Welcome. Let's find your appointment.",
    welcomeBody: "Enter the phone number and last name on file to check yourself in.",
    phone: "Phone number",
    lastName: "Last name",
    findAppointment: "Find my appointment",
    needHelp: "Need help? Ask the front desk.",
    hello: "Hello",
    alreadyCheckedIn: "You're already checked in. Please have a seat in the waiting room.",
    confirmArrival: "Confirm your arrival for today's appointment:",
    appointment: "Appointment",
    walkInInstead: "None of these — check in as a walk-in",
    checkInWalkIn: "Check in as a walk-in",
    startOver: "Start over",
    checkedInTitle: "You're checked in!",
    checkedInBody: "Please have a seat. You'll be called shortly.",
    done: "Done",
  },
  fr: {
    selfServiceCheckIn: "Enregistrement libre-service",
    welcomeTitle: "Bienvenue. Retrouvons votre rendez-vous.",
    welcomeBody: "Entrez le numéro de téléphone et le nom de famille au dossier pour vous enregistrer.",
    phone: "Numéro de téléphone",
    lastName: "Nom de famille",
    findAppointment: "Trouver mon rendez-vous",
    needHelp: "Besoin d'aide ? Demandez à la réception.",
    hello: "Bonjour",
    alreadyCheckedIn: "Vous êtes déjà enregistré. Merci de patienter en salle d'attente.",
    confirmArrival: "Confirmez votre arrivée pour le rendez-vous d'aujourd'hui :",
    appointment: "Rendez-vous",
    walkInInstead: "Aucun de ceux-ci — m'enregistrer sans rendez-vous",
    checkInWalkIn: "M'enregistrer sans rendez-vous",
    startOver: "Recommencer",
    checkedInTitle: "Vous êtes enregistré !",
    checkedInBody: "Merci de patienter. On vous appellera bientôt.",
    done: "Terminé",
  },
} as const;

type KioskLanguage = keyof typeof kioskStrings;

async function kioskFetch<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const contentType = response.headers.get("content-type") ?? "";
  const parsed: unknown = contentType.includes("application/json") ? await response.json() : await response.text();
  if (!response.ok) {
    const message = typeof parsed === "object" && parsed !== null && "message" in parsed ? String((parsed as { message: unknown }).message) : "Something went wrong.";
    throw new Error(message);
  }
  return parsed as T;
}

interface LookupResult {
  patientId: string;
  firstName: string;
  lastInitial: string;
  alreadyCheckedIn: boolean;
  appointments: { id: string; startsAt: string; reason: string; status: string }[];
}

type Screen = "welcome" | "confirm" | "success";

export function KioskPage() {
  const [lang, setLang] = React.useState<KioskLanguage>("en");
  const s = kioskStrings[lang];
  const [screen, setScreen] = React.useState<Screen>("welcome");
  const [phone, setPhone] = React.useState("");
  const [lastName, setLastName] = React.useState("");
  const [loading, setLoading] = React.useState(false);
  const [error, setError] = React.useState("");
  const [result, setResult] = React.useState<LookupResult | null>(null);

  const reset = React.useCallback(() => {
    setScreen("welcome");
    setPhone("");
    setLastName("");
    setError("");
    setResult(null);
    setLoading(false);
  }, []);

  React.useEffect(() => {
    if (screen !== "success") return;
    const timer = window.setTimeout(reset, 12000);
    return () => window.clearTimeout(timer);
  }, [screen, reset]);

  const lookup = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    setLoading(true);
    try {
      const found = await kioskFetch<LookupResult>("/public/kiosk/lookup", { phone, lastName });
      setResult(found);
      setScreen("confirm");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not find a matching record.");
    } finally {
      setLoading(false);
    }
  };

  const checkIn = async (appointmentId: string) => {
    if (!result) return;
    setLoading(true);
    setError("");
    try {
      await kioskFetch("/public/kiosk/checkin", { patientId: result.patientId, phone, lastName, appointmentId });
      setScreen("success");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not check you in.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="grid min-h-dvh place-items-center bg-zinc-100 p-6">
      <div className="w-full max-w-lg rounded-2xl border bg-white p-8 shadow-xl">
        <div className="mb-6 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="grid h-12 w-12 place-items-center rounded-xl bg-black text-white"><Glasses className="h-6 w-6" /></div>
            <div>
              <div className="text-lg font-bold leading-none">SentryMed Opti</div>
              <div className="mt-1 text-xs text-zinc-500">{s.selfServiceCheckIn}</div>
            </div>
          </div>
          <div className="flex overflow-hidden rounded-md border text-xs font-semibold">
            {(Object.keys(kioskStrings) as KioskLanguage[]).map((code) => (
              <button
                key={code}
                onClick={() => setLang(code)}
                className={lang === code ? "bg-black px-2.5 py-1.5 text-white" : "bg-white px-2.5 py-1.5 text-zinc-500"}
              >
                {code.toUpperCase()}
              </button>
            ))}
          </div>
        </div>

        {screen === "welcome" && (
          <form className="grid gap-4" onSubmit={lookup}>
            <h1 className="text-xl font-bold">{s.welcomeTitle}</h1>
            <p className="text-sm text-zinc-500">{s.welcomeBody}</p>
            <label className="grid gap-1.5 text-sm font-medium">
              <span>{s.phone}</span>
              <input
                className="h-14 rounded-lg border px-4 text-lg"
                type="tel"
                inputMode="tel"
                required
                autoFocus
                value={phone}
                onChange={(event) => setPhone(event.target.value)}
              />
            </label>
            <label className="grid gap-1.5 text-sm font-medium">
              <span>{s.lastName}</span>
              <input
                className="h-14 rounded-lg border px-4 text-lg"
                required
                value={lastName}
                onChange={(event) => setLastName(event.target.value)}
              />
            </label>
            {error && <p className="rounded-md bg-red-50 p-3 text-sm text-red-700">{error}</p>}
            <button
              type="submit"
              disabled={loading}
              className="flex h-14 items-center justify-center gap-2 rounded-lg bg-black text-lg font-semibold text-white disabled:opacity-50"
            >
              {loading && <Loader2 className="h-5 w-5 animate-spin" />}
              {s.findAppointment}
            </button>
            <p className="text-center text-xs text-zinc-400">{s.needHelp}</p>
          </form>
        )}

        {screen === "confirm" && result && (
          <div className="grid gap-4">
            <h1 className="text-xl font-bold">{s.hello}, {result.firstName} {result.lastInitial}</h1>
            {error && <p className="rounded-md bg-red-50 p-3 text-sm text-red-700">{error}</p>}
            {result.alreadyCheckedIn ? (
              <div className="rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-800">{s.alreadyCheckedIn}</div>
            ) : result.appointments.length > 0 ? (
              <div className="grid gap-3">
                <p className="text-sm text-zinc-500">{s.confirmArrival}</p>
                {result.appointments.map((appointment) => (
                  <button
                    key={appointment.id}
                    disabled={loading}
                    onClick={() => checkIn(appointment.id)}
                    className="flex items-center gap-3 rounded-lg border-2 border-black p-4 text-left hover:bg-zinc-50 disabled:opacity-50"
                  >
                    <CalendarCheck className="h-5 w-5 shrink-0" />
                    <div>
                      <div className="font-semibold">{new Date(appointment.startsAt).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}</div>
                      <div className="text-xs text-zinc-500">{appointment.reason || s.appointment}</div>
                    </div>
                  </button>
                ))}
                <button disabled={loading} onClick={() => checkIn("")} className="text-center text-xs text-zinc-400 underline">
                  {s.walkInInstead}
                </button>
              </div>
            ) : (
              <button
                disabled={loading}
                onClick={() => checkIn("")}
                className="flex h-14 items-center justify-center gap-2 rounded-lg bg-black text-lg font-semibold text-white disabled:opacity-50"
              >
                {loading && <Loader2 className="h-5 w-5 animate-spin" />}
                {s.checkInWalkIn}
              </button>
            )}
            <button onClick={reset} className="text-center text-xs text-zinc-400 underline">{s.startOver}</button>
          </div>
        )}

        {screen === "success" && (
          <div className="grid place-items-center gap-4 py-6 text-center">
            <CheckCircle2 className="h-16 w-16 text-emerald-600" />
            <h1 className="text-xl font-bold">{s.checkedInTitle}</h1>
            <p className="text-sm text-zinc-500">{s.checkedInBody}</p>
            <button onClick={reset} className="mt-2 text-xs text-zinc-400 underline">{s.done}</button>
          </div>
        )}
      </div>
    </div>
  );
}
