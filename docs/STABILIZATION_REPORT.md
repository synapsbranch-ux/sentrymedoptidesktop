# Stabilisation and bug-fix report

Covers the brief's sections 3–6. Every item below was reproduced before it was
fixed and verified after. One item, one commit; each commit message carries the
item ID and its own root-cause note.

## 0. The context the brief asked for, filled in

Section 0 of the brief was handed over with its placeholders empty. These are the
values as read from the repository, not assumptions — correct anything that is
wrong before the next round.

| | |
|---|---|
| Repository / branch | `synapsbranch-ux/sentrymedoptidesktop`, branch `claude/optical-clinic-stabilization-1tvhlt` |
| Stack | Go 1.25 (chi, `database/sql`), SQLite via `modernc.org/sqlite`, React 19 + TypeScript + Vite 7, Tailwind 4 with Radix primitives |
| Shape | Modular monolith: one Go process owns the database, serves the PWA to the clinic LAN and backs the Wails 2.15 desktop shell |
| Environments | Local only. `make api` (Go, `--dev`), `make dev` (Vite on :5173), `make build`, `make desktop`. No staging or production URL exists in the repository |
| Seed data | `make seed` (`go run ./cmd/sentrymed --seed`) creates `doctor.dev` and `nurse.dev` |
| Roles | `doctor` and `nurse` only |
| Browsers | Chrome, Edge and Safari on desktop and mobile, plus the desktop shell's own WebView (WebView2 on Windows, WebKitGTK on Linux, WKWebView on macOS) |
| Bug tracker | **None specified and none found.** This file is the log until one is named |

## 1. Bug log

Severity: **S1** takes the application down, **S2** blocks a workflow, **S3** is a
defect with a workaround, **S4** is cosmetic or hygiene.

### Fixed

| ID | Module | Sev | Steps to reproduce | Expected | Actual | Status |
|---|---|---|---|---|---|---|
| A1 | Consultations → Evolution | S1 | Open any consultation, select the Evolution tab | The section renders | React aborts the whole tree with "Rendered more hooks than during the previous render"; the application unmounts to a blank screen | Fixed |
| A1-b | Consultations → Evolution | S2 | Evolution for a patient whose trends payload has no `thresholds` | Section renders | `TypeError` from `?.thresholds.elevatedIOPMmHg` — optional chaining guarded only `data` | Fixed |
| A1-c | Consultations → Evolution | S3 | Evolution with a null or malformed visit date | Blank or dash | "Invalid Date"; NaN measurements produced NaN SVG coordinates | Fixed |
| A1-d | Whole application | S1 | Any render error anywhere | Only that panel fails | No error boundary existed; any throw blanked the app | Fixed |
| A2 | Consultation → Recording | S2 | Open SentryMed over `http://<lan-ip>` on a phone and press Start recording | A message saying why, and how to fix it | Raw browser string "The request is not allowed by the user agent or platform in the current context" | Fixed |
| A2-b | Consultation → Recording | S3 | Record for 30 s and stop, then look at the saved duration | 30 s | Always 0 s — `recorder.onstop` closed over `elapsed` from the render that started the recording | Fixed |
| A2-c | Consultation → Recording | S3 | Stop a recording | Whole recording saved | Tracks were stopped before MediaRecorder flushed, truncating the final chunk | Fixed |
| A3 | Appointments, lab orders | S2 | Open any form with a time field in the desktop shell | An hour selector | None. The only time entry was the time half of `datetime-local`, which WebKitGTK/WKWebView degrade to a plain text box | Fixed |
| A3-b | Optical lab → new order | S3 | Set an expected date, reopen the form | The chosen date | Blank. The field held an ISO 8601 string, which no date/time input accepts as a value | Fixed |
| A4 | Documents, patient record | S2 | Upload a file to a patient, then try to open it | An in-app viewer | No viewer existed anywhere, and the patient record listed no documents at all | Fixed |
| A4-b | Documents (desktop shell) | S2 | Press Download in the desktop application | The file | 401. A plain `<a href>` cannot carry the shell's in-memory session header | Fixed |
| A4-c | Documents | S3 | Upload a TIFF scan | Accepted | 415 — Go's sniffer has no TIFF entry and reported `application/octet-stream` | Fixed |
| B1 | Patients | S2 | Search 5,000 patients | Fast, indexed, accent-insensitive | `LIKE '%term%'` across five columns, which no index can serve; "jean" did not find "Jéan" | Fixed |
| B1-b | Appointments, consultations, POS, lab, insurance | S2 | Pick a patient in any of these five forms | Any patient reachable | A `<select>` filled from `/patients?limit=100`; patient 101 of 5,000 was unreachable | Fixed |
| B2 | Waiting room | S2 | Register a walk-in | One short form | Two dialogs and a dropdown; the walk-in form's own help text said to register the patient elsewhere first | Fixed |
| B2-b | Waiting room | S3 | Look at who is waiting | Reason and practitioner visible | Neither was shown; `queue_entries` had no reason of its own | Fixed |
| C1 | Calendar, appointment list, waiting room | S3 | Compare two appointment statuses | Distinguishable at a glance | Every status rendered as the same grey pill showing the raw value with underscores; the waiting room used a different badge for the same state | Fixed |
| Q1 | Patient picker (introduced by B1) | S2 | Click a search result with a real mouse or finger | The patient is selected | Nothing happened. The picker sat inside `Field`, which renders a `<label>`; browsers forward clicks anywhere inside a label to its one labelable control, so every result click landed on the search input | Fixed |
| Q1-b | Patient picker (introduced by B1) | S3 | Select a result with the keyboard | Selection works | Selection was bound to `mousedown` only, which keyboard and assistive-technology activation never produce | Fixed |
| Q2 | Prescriptions | S2 | `POST /prescriptions` with an `encounterId` belonging to another patient | Rejected | Accepted — the document was filed against someone else's visit | Fixed |
| Q3 | Consultations → Evolution | S4 | Read the IOP chart legend in the English UI | English | French: "— seuil 21 mmHg" | Fixed |

### Found, not fixed — see §4 for the reasoning

| ID | Module | Sev | Steps to reproduce | Expected | Actual | Status |
|---|---|---|---|---|---|---|
| N1 | Consultation → diagnosis and procedure codes | S3 | Search a code, then select it with the keyboard | Selection works | `CodeCombobox` selects on `mousedown` only — the same defect as Q1-b, in a component outside this brief's scope | Logged |
| N2 | Consultation → code search | S4 | Type in the code search box | One request when typing stops | One request per keystroke; no debounce | Logged |
| N3 | Documents, Prescriptions | S3 | Open either list in a clinic with years of records | A page at a time | Both endpoints return every row; no `limit`/`offset` | Logged |
| N4 | Patients, appointments, medical history forms | S3 | Start editing, then navigate away without saving | A warning | None. Only the consultation's chart drafts warn | Logged |
| N5 | Queue, documents and prescription list handlers | S4 | A row that fails to scan | An error | The row is silently dropped (`if rows.Scan(...) == nil`) | Logged |
| N6 | Frontend test setup | S4 | Use a `@testing-library/jest-dom` matcher in a test | It works | "Invalid Chai property". The package is a dependency but is never loaded — `vitest.setupFiles` is not configured | Logged |
| N7 | `chart-backdrops.tsx`, `inventory.tsx`, `stock-takes.tsx` | S4 | `tsc --noUnusedLocals` | Clean | Unused imports (`React`, `Boxes`, `CardContent`, `Eye`) | Logged |
| N8 | Build output | S4 | `npm run build` | Chunks under 500 kB | `chart-eye-3d` is 519 kB, over Vite's warning threshold | Logged |
| N9 | `e2e/chart-multiview.spec.ts` | S4 | Run the suite where the installed Chromium differs from the pinned Playwright build | Runs | `test.use({ launchOptions })` replaces config-level launch options, so an `executablePath` override is discarded. Environment fragility, not an application defect | Logged |
| N10 | e2e suite | S4 | Run the whole suite in one go | Order-independent | Specs share one server and one database, so any two that book the same appointment slot collide with a 409. Hit once during this round and fixed in the new sweep; the same trap remains for future specs | Fixed here, noted |

## 2. Root cause, change and verification, per item

### A1 — Evolution crashed the whole application
**Root cause.** `ClinicalEvolution` called `React.useState` *after* two early
returns for the loading and error states. The first render ran 12 hooks; the
render after the request resolved ran 13. React aborts the entire tree on a
changed hook count, so one tab unmounted the application. A codebase-wide scan
found no second instance of this pattern.
**Changed.** State moved above every conditional return; the malformed-record
paths the panel had no defence for were guarded; a reusable `ErrorBoundary` now
wraps the routed page outlet and each consultation tab, with `useOptionalI18n` so
the fallback cannot throw while rendering itself.
**Verified.** `clinical-evolution.test.tsx` reproduced the crash before the fix
and now covers 0, 1 and 60 entries, a record with null/missing/malformed fields,
and boundary containment.

### A2 — "The request is not allowed by the user agent"
**Root cause, in the order the brief asked for.** (1) **Insecure context — this is
the one.** The server serves HTTPS in production but not under `--dev` or with
`SENTRYMED_AUTO_TLS=false`, and the Vite dev server is plain HTTP. A phone opening
`http://<lan-ip>:5173` or `:8787` has no `navigator.mediaDevices` at all, and
Safari raises `NotAllowedError` with exactly this wording. (2) Site permission is
handled as a separately worded case. (3) `Permissions-Policy: microphone=(self)`
permits the clinic's own origin and `X-Frame-Options: DENY` rules out an
embedding frame, so the header was left unchanged — the client now detects a
denial if one is ever introduced.
**Changed.** `src/microphone.ts` inspects the environment before requesting the
device and maps every `DOMException` name to a specific cause and a repair
instruction; the recorder shows it inline and disables Start where the
environment cannot record. Two defects found while reproducing were fixed
(A2-b, A2-c).
**Verified.** `microphone.test.ts` asserts no raw browser string reaches the user
and that one `NotAllowedError` is attributed to whichever cause actually applies.

### A3 — No time picker anywhere
**Root cause.** There is no hour-selector component in the codebase. The only
time entry was the time half of `<input type="datetime-local">`, and the WebKit
engines behind the desktop shell do not implement date/time widgets — they
silently degrade the field to a plain text box. It failed on every screen because
one shared control was missing.
**Changed.** `components/ui/date-time.tsx` provides `TimePicker` and
`DateTimeInput`, built from native `<select>` elements. A native select paints its
list at the OS level, so it cannot be clipped by a modal's `overflow-y-auto` or
hidden behind its stacking context, and it is keyboard- and touch-operable with
no custom popup code. Both `datetime-local` fields now use it. A3-b was found
while replacing the lab field.
**Verified.** 8 tests: parsing, the split/join round trip, both halves reporting a
complete `HH:MM`, redisplay of a saved value, and operation inside a modal.

### A4 — Documents could not be opened
**Root cause, two of them.** There was no viewer of any kind — the Documents page
offered only a download link and the patient record listed no documents at all.
And that link is dead in the desktop shell: WebKit custom URI schemes do not
persist `Set-Cookie`, so the desktop session lives in an in-memory
`Authorization` header that no `<a href>` or `<img src>` can carry.
**Changed.** Server: `GET /documents/{id}/content` serves the file inline behind
the same session middleware, with `Cache-Control: private, no-store`; TIFF is
accepted by checking its magic number directly; CSP gains `media-src`/`frame-src
blob:` and an explicit `object-src 'none'`. Client: `api.blob()` fetches with the
session header attached; `DocumentViewer` opens PDFs in an embedded frame (the
browser's own viewer supplies page navigation) and images with zoom and rotate,
with download and print on both, and a download offer instead of a dead link for
a format it cannot paint.
**Verified.** 3 Go tests (inline vs attachment disposition, byte-identical
content, TIFF in both endiannesses, 401 without a session, mismatched-content
rejection) and 5 component tests.

### B1 — Patient search at 5,000 records
**Root cause.** `LIKE '%term%'` across five columns — a pattern no index can
serve and that cannot ignore accents — with no pagination, no filters and no way
to reach a patient the first page missed. Five separate forms picked a patient
from a `<select>` filled by `/patients?limit=100`.
**Changed.** Migration 012 adds `patients_search`, an FTS5 index over names, file
number, phones, email and date of birth, kept current by triggers. The
`unicode61` tokenizer with `remove_diacritics 2` folds case and accents at both
index and query time, so "jean" finds "Jéan" with no normalised copy in
application code. Phone numbers are indexed as written and with separators
stripped, and a run of four or more digits is additionally matched *inside* the
stored number, because staff type the tail of a number for a record saved with a
country code. Typed input is quoted per term so FTS operators typed by accident
are matched literally. Filters: age range (derived from date of birth, never
stored), sex, civil status, insurance, practitioner, last-visit range, and
active/archived/all. Each row carries its last visit. Indexes added on
`updated_at`, `date_of_birth`, `archived_at`, `patient_insurance(patient_id)`,
`encounters(doctor_id)` and `appointments(practitioner_id)`. The list debounces
at 300 ms and pages 25 at a time; `PatientPicker` replaced all five dropdowns.
**Verified.** 7 Go tests including a 5,000-record volume test. Measured on this
machine: 4.5 ms for a name prefix, 5.9 ms for a full surname, 5.1 ms for a name
plus three filters, 27.7 ms for the digit-substring phone path, 1.3 ms unfiltered
— against a 500 ms budget. 7 component tests cover the debounce and the query
construction.

### B2 — Walk-in and waiting room
**Root cause.** Registering a walk-in meant creating the patient in one dialog and
then reopening "Walk-in" to find them in a dropdown; the waiting room was a narrow
column beside the calendar showing neither the reason nor the practitioner, and
offered only "Move to <next stage>" one step at a time.
**Changed.** `POST /queue/walk-in` registers and queues in one transaction, reuses
an existing record when one is chosen, and returns a 409 naming the match rather
than silently creating a duplicate. `queue_entries` gains its own `visit_reason`
(migration 013), backfilled from any linked appointment. `WaitingRoomScreen` lists
everyone in arrival order with a self-advancing wait timer, the reason, the
practitioner, priority and walk-in markers, long waits flagged at 30 and 60
minutes in colour *and* words, and one-click stage changes with no confirmations.
It refreshes from the live stream and re-reads once a minute as a safety net.
**Verified.** 5 Go tests, 11 component tests, and an e2e that walks a patient from
arrival to completed counting the clicks — **it takes 5, and the test fails above
that.**

### C1 — Status colours
**Root cause.** Every appointment status rendered as the same grey pill showing
the raw value with underscores, and the waiting room used a separate badge, so
one state read differently on two screens.
**Changed.** `components/status.tsx` is the single definition, with the seven
colours the brief specified, a pill style, a dense chip style for calendar cells
and a left-edge style for list rows. Colour is never the only signal: every
appearance carries its own label and icon, and no-show is additionally striped so
it is distinguishable from cancelled without seeing that one red is darker — in
print as well as for a colour-blind reader. Waiting-room stages map onto the same
seven, so "in consultation" is literally the same appearance object on both
screens.
**Verified.** 8 tests asserting the required hue per status, that no-show differs
by stripe/icon/label rather than shade, that an unmapped status still renders
readable words, and that the legend covers all seven.

### D1 — Civil status and religion
Migration 014 adds three nullable columns, so no existing record changes meaning
and "not recorded" stays distinct from "prefer not to say". Both fields validate
against a closed list server-side, so a typo cannot create a category; the free
text is dropped when a listed religion is chosen. They appear in the registration
form, stay editable, show in the patient overview, and are in the printed patient
summary — which is the clinic's patient export. Also completes B1's civil-status
filter. 5 Go tests, 4 component tests.

### D2 — Admin-managed catalogs
Migration 015 adds `catalog_entries` for both lists with per-entry defaults,
ordering and an active flag. Retiring an entry deactivates rather than deletes it,
so a record already referring to one keeps its meaning. Reads are open to any
signed-in user (a nurse books appointments); writes are doctor-only. System →
Catalogs edits both, effective immediately through the existing live stream with
no deployment. The appointment and walk-in forms suggest reasons while still
accepting anything typed; the medication prescription gets a picker that fills
its six fields from the entry's defaults, all still editable. 5 Go tests, 7
component tests.

### D3 — Doctor signature
Migration 016 adds `user_signatures` keyed by user, plus four columns on
prescriptions recording who signed, when, and a snapshot of the image applied.
Ownership is enforced, not assumed: a signature is only ever written through
`/me/signature`, which resolves the owner from the session; there is no route that
writes another user's signature or returns an arbitrary user's current one; and
the signature stamped on a prescription is looked up by the issuing doctor's own
session id. The image behind an issued prescription is served from that
prescription's snapshot, so replacing or deleting a signature never retroactively
changes a document already issued. Upload accepts PNG (preferred, transparent) or
JPEG, content-sniffed rather than trusted by extension, capped at 2 MB, stored
0600 with a SHA-256 checksum. The pad uses pointer events, so mouse, trackpad and
finger take one path — which is what makes it work on a tablet — with
`touch-action: none` so the page does not scroll instead of drawing. 4 Go tests,
8 component tests.

### D4 — Prescription outside a consultation
The server already accepted a prescription with no encounter, but nothing could
create one and nothing recorded that a document had no visit behind it.
Prescriptions now report `standalone`; the register shows each one's origin and
the printed page states "Issued outside a consultation". A doctor-only flow picks
the patient by search, fills the values (a medication can be prefilled from the
D2 catalog), issues — applying the D3 signature — and opens the document ready to
print. Q2 was found while adding this. 4 Go tests plus an e2e covering the flag in
the register, the printed page and the patient's history.

### D5 — Floating recording widget
Recording state moved out of the Recording tab into a `RecordingProvider` around
the whole consultation, so switching tabs or scrolling can no longer hide or
interrupt it. The widget is portalled to `document.body` because the consultation
dialog is centred with a CSS transform, which would otherwise become the
containing block for a fixed child and pin the widget inside the scrolling panel.
It shows a pulsing indicator, elapsed time, the patient, and pause/resume/stop
(pause only where MediaRecorder implements it); it is draggable, clamped inside
the viewport, and collapsible. Leaving the page while recording raises the
browser's confirmation. Nothing captured is lost: MediaRecorder runs with a
one-second timeslice and every chunk is written to IndexedDB as it arrives, so a
closed tab, lost connection or crash leaves at most the last second unrecorded;
reopening that consultation offers the audio back with its duration. A failed
upload deliberately keeps the local copy. Failures of the local store itself are
swallowed — losing the safety net must never stop the recording in progress. 8
tests.

## 3. QA pass (section 6)

Automated rather than a one-off manual sweep, so the results stay true.

- **Per module.** Create, read, update, delete and empty state exercised across
  patients, appointments, queue, consultations, prescriptions, documents,
  catalogs, signatures and users. Every list endpoint asserted to return `[]` and
  never `null` on an empty clinic — a JSON null is what makes a list screen throw
  on `.map`.
- **Forms.** Required-field validation, unparseable dates, a stale second save
  (refused with 409 rather than silently overwriting) and a double check-in
  (refused rather than queueing the patient twice).
- **Edge data.** Accents, apostrophes, hyphens, 120-character names, non-Latin
  script, missing optional fields, patients with no history, and dates in 1901 and
  2099. Apostrophes matter twice over: they are FTS operator characters.
- **UI.** No route scrolls sideways at 360 px or 768 px with long names, long
  reasons and tag lists present; modal actions stay inside a 360 px screen on the
  patient, appointment and prescription forms. Mixed-language text: one French
  string in the English IOP legend was fixed (Q3). The kiosk's French strings are
  a deliberate `en`/`fr` table for a patient-facing screen, not a defect.
- **Permissions.** A nurse is refused every doctor-only endpoint by direct URL,
  reads and writes alike, and every endpoint is refused when signed out. The
  server guards match the client's route guards.
- **Performance.** The 5,000-patient volume test above. List views were loaded
  with data present rather than five demo records.
- **Console.** Every route, both waiting-room and calendar screens, and all five
  consultation tabs are walked while failing on any uncaught error, React or
  framework warning, or tripped error boundary. Currently clean. The only
  non-2xx responses are `/auth/me` before login and the clinic logo in a clinic
  that has not uploaded one, which has an `onError` fallback.

## 4. Found but not fixed, and why

Rule 7 of the brief: findings outside the list are logged with a severity, not
silently fixed. Each of N1–N9 in §1 is outside the requested scope and none of
them takes anything down.

- **N1/N2 (code combobox).** N1 is genuinely the same defect as Q1-b and I would
  fix it in one line. It is in the diagnosis/procedure code picker, which this
  brief does not cover, and touching it would widen the diff into a module with no
  acceptance test here. **Recommend fixing next; say the word.**
- **N3 (unpaginated document and prescription lists).** Real, and it will bite at
  the same scale B1 was about. It needs the same treatment B1 got — server paging
  plus a paged UI — which is a piece of work, not a patch.
- **N4 (no unsaved-changes warning on ordinary forms).** Section 6 lists it as
  something to check, not something to build; adding it across every form is a
  feature, and rule 1 says not to build what is not written down.
- **N5 (silently dropped rows).** Pre-existing pattern in three list handlers.
  Low risk, but it hides a real database problem behind a short list.
- **N6–N8.** Hygiene: a test dependency that is installed but never loaded,
  unused imports, and one oversized bundle chunk.
- **N9.** An environment fragility in an existing e2e file. Not an application
  defect; noted so the next person does not lose an hour to it.
- **N10.** Worth knowing before writing the next spec: the e2e suite shares one
  server and one database across all files, so fixed timestamps collide. This
  round's console sweep booked the same `now + 1 hour` slot as the existing
  clinic-flow spec and made it fail with a 409 — a defect in my test, not in the
  application, now fixed by booking 60 days out.

## 5. Questions — asked, not guessed in code

1. **The religion list (D1).** Seeded for this clinic's population: Catholic,
   Protestant/Evangelical, Baptist, Adventist, Pentecostal, Methodist, Jehovah's
   Witness, Vodou, Muslim, Jewish, No religion, Other, Prefer not to say. It is a
   closed list in code, because the brief authorises admin catalogs for D2 only.
   Confirm or correct it — it is a one-line change in
   `internal/server/patient_demographics.go` and `apps/web/src/components/demographics.ts`.
2. **The catalog starting lists (D2).** The brief says to confirm these with
   clinic staff first, so appointment reasons were seeded with **exactly** the four
   visit types the application already hardcoded (nothing the clinic sees today
   disappeared), and the **prescription catalog was left empty on purpose** —
   inventing a medication list would put unapproved clinical content into the
   record. Both are now filled in from the interface, so answering this needs no
   code change at all.
3. **"Assigned practitioner" (B1 filter).** The schema has no usual-practitioner
   column and the brief does not authorise adding one, so the filter means "the
   clinicians who have seen or are scheduled to see this patient". Confirm that is
   what the front desk means by it.
4. **PDF page navigation in the Linux desktop shell (A4).** PDFs open in an
   embedded frame, so Chrome, Edge, Safari and Firefox supply page navigation,
   zoom, print and download from their own built-in viewer — the acceptance test
   passes on every browser it names. WebKitGTK, the engine behind the Linux
   desktop build, has no built-in PDF viewer, so a PDF there falls back to
   download. Closing that gap means adding `pdf.js`, a new dependency, which rule
   3 says to raise before doing. **Do you want it?**
5. **Where should bugs be tracked?** Section 0 left this blank. This file is the
   log until a tracker is named.

## 6. Definition of done — status

- No screen crashes and none produces an uncaught console error or framework
  warning: **enforced by a test that walks every route, both waiting-room screens
  and all five consultation tabs.**
- Every acceptance test in sections 3, 4 and 5: **passing**, including the two the
  brief stated numerically — search under 500 ms at 5,000+ records (measured
  4.5–27.7 ms) and arrival-to-checkout in five clicks or fewer (measured exactly
  5, with the test failing above that).
- Section 6 checklist completed and its findings logged: **above**.
- No feature exists that was not requested: **nothing outside sections 3–5 was
  built.** The one judgement call is `FieldGroup`, a non-label wrapper added to fix
  Q1; it is a bug fix, not a feature.

Two acceptance criteria remain **untested here and need a device pass before
release**, because this environment has one browser engine and no microphone:
A2's "recording works in Chrome, Edge and Safari, on desktop and on mobile, over
HTTPS", and A4's viewer on the clinic's own hardware.
