# HTTP API

All application endpoints use `/api/v1`. JSON failures have the form:

```json
{
  "code": "CONCURRENT_MODIFICATION",
  "message": "The record changed since it was opened."
}
```

Authentication is an opaque `HttpOnly`, `SameSite=Strict` session cookie. `Secure` is enabled when served over TLS. Unless marked public, endpoints require a session. Doctor-only rules are enforced server-side.

## Public and authentication

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/health` | Public | Server/database health and live connection count |
| GET | `/api/v1/setup/status` | Public | Whether first-run setup is required |
| GET | `/api/v1/public/branding/logo` | Public | Current clinic logo for UI and print assets |
| GET | `/api/v1/public/display` | Public when enabled | Privacy-filtered queue and today's appointments |
| GET | `/api/v1/public/events` | Public when enabled | Live display SSE invalidation stream |
| POST | `/api/v1/setup/complete` | Server computer | Create clinic settings and initial doctor |
| POST | `/api/v1/auth/login` | Public/rate-limited | Start session |
| POST | `/api/v1/auth/logout` | Authenticated | Invalidate session |
| GET | `/api/v1/auth/me` | Authenticated | Current user without password hash |
| GET | `/api/v1/events` | Authenticated | SSE update stream |

## Patients, appointments and queue

| Methods | Path | Access |
|---|---|---|
| GET, POST | `/patients` | Doctor, nurse |
| GET, PUT, PATCH, DELETE | `/patients/{id}` | Doctor, nurse; delete archives |
| GET, PUT | `/patients/{id}/history` | Doctor, nurse; versioned |
| GET | `/patients/{id}/timeline` | Doctor, nurse |
| GET, POST | `/appointments` | Doctor, nurse |
| PUT | `/appointments/{id}` | Doctor, nurse; reschedule/versioned |
| PATCH | `/appointments/{id}/status` | Doctor, nurse; versioned |
| GET | `/queue` | Doctor, nurse |
| POST | `/queue/check-in` | Doctor, nurse; appointment optional |
| PATCH | `/queue/{id}` | Doctor, nurse; versioned |

Patient listing accepts `q`, `page`, and `limit` (maximum 100; pickers request 10). Search covers prefix names, MRN, exact patient ID, email, normalized phone and supported birth-date queries, and excludes archived patients by default. Appointments accept `date=YYYY-MM-DD` or `from`/`to` range filters.

Patient `PUT` and `PATCH` both merge only supplied editable fields. Omitted fields are preserved, explicit `null` clears optional fields, and `version` is mandatory on every update. Read-only fields such as `id`, `medicalRecordNumber`, `createdAt`, `updatedAt` and `updatedBy` must not be sent. Example: `{"sex":"female","version":4}`. Invalid values return 422, malformed/unknown fields 400, missing records 404, and stale versions 409. History updates also preserve omitted fields and use their own required version.

Appointment writes require an active patient, an active doctor if assigned, a supported type, 10–240 minutes and an RFC3339 timestamp. New appointments default to 30 minutes when duration is omitted/zero. Times are normalized to UTC; atomic database guards reject overlapping bookings with 409, including concurrent requests and reactivation.

## Encounters, prescriptions and documents

| Methods | Path | Access |
|---|---|---|
| GET, POST | `/encounters` | Doctor, nurse |
| GET, PUT | `/encounters/{id}` | Doctor/nurse view; clinical update checks role/state |
| PUT | `/encounters/{id}/pretest` | Doctor, nurse; separate version |
| POST | `/encounters/{id}/pretest/skip` | Doctor, nurse; gated by the clinic's pre-test policy |
| PUT | `/encounters/{id}/sections/{section}` | Section-aware authorization/version |
| POST | `/encounters/{id}/diagnoses` | Doctor |
| POST | `/encounters/{id}/finalize` | Doctor |
| POST | `/encounters/{id}/addenda` | Doctor, finalized encounter only |
| GET | `/prescriptions` | Doctor, nurse |
| POST | `/prescriptions` | Doctor |
| GET, POST | `/documents` | Doctor, nurse |
| GET | `/documents/{id}/download` | Doctor, nurse |
| DELETE | `/documents/{id}` | Doctor, nurse; archives metadata |

A consultation carries its own `workflowStage`: it opens in `pre_test` and moves to `doctor_exam` when the pre-test is completed or skipped, one way only. A doctor who starts the consultation themself begins past that point. Skipping is refused when the clinic's `pretestPolicy` setting is `required`.

Finalizing is the doctor's sign-off, not a completeness check. Missing diagnoses, prescriptions or examination sections come back as `FINALIZE_WARNINGS` with the list; reposting with `acknowledgeWarnings` signs the consultation and records what was acknowledged in the audit log.

`POST /prescriptions` accepts `newPatient` instead of `patientId` and registers that person in the same transaction, so a prescription can be written for somebody who is not on file yet. A near-match comes back as `POSSIBLE_DUPLICATE_PATIENT` carrying the existing patient's id.

## Diagnosis reference

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/codes/icd10` | Authenticated | Type-ahead over the diagnosis reference, by code or plain language |
| GET | `/diagnosis-codes` | Authenticated | Browse the reference, paged, filtered by `q` and `category` |
| GET | `/diagnosis-codes/categories` | Authenticated | Reference categories with their code counts |
| POST | `/diagnosis-codes/import` | Doctor | Load a clinic's own CSV code file (`code,description[,category[,synonyms]]`) |

Search terms are quoted before they reach FTS5, so punctuation in a code — or an accidental MATCH expression — is never read as query syntax. Imported rows own the codes they name and survive later upgrades of the bundled reference.

## Operations and optical lab

| Methods | Path | Access |
|---|---|---|
| GET, POST | `/inventory` | Doctor, nurse |
| GET, POST | `/inventory/{id}/movements` | Doctor, nurse; versioned stock write |
| GET, POST | `/suppliers` | Read both roles; write doctor |
| PUT, DELETE | `/suppliers/{id}` | Doctor; delete archives |
| GET, POST | `/purchase-orders` | Doctor |
| GET | `/purchase-orders/{id}` | Doctor |
| PATCH | `/purchase-orders/{id}/status` | Doctor; versioned |
| POST | `/purchase-orders/{id}/receive` | Doctor |
| GET, POST | `/stock-takes` | Read both roles; create doctor |
| GET | `/stock-takes/{id}` | Doctor, nurse |
| PUT | `/stock-takes/{id}/items/{itemId}` | Doctor, nurse; count/versioned |
| POST | `/stock-takes/{id}/finalize` | Doctor; transactional adjustments |
| GET, POST | `/lab-orders` | Doctor, nurse |
| PATCH | `/lab-orders/{id}/status` | Doctor, nurse; versioned |
| POST | `/lab-orders/{id}/quality-control` | Doctor, nurse |
| POST | `/lab-orders/bulk-status` | Doctor, nurse; all-or-nothing |
| GET | `/lab-orders/requisition` | Doctor, nurse |

Marking a lab order `ready` fails until a QC row exists.

A batch moves together or not at all: `bulk-status` accepts up to 200 order ids and rolls the whole batch back if any of them is already delivered or cancelled. Delivery and quality control stay per-order decisions and cannot be done in bulk. `/lab-orders/requisition?orderIds=` builds one printed requisition covering the batch, grouped by lab.

A service is an inventory item with `durationMinutes` and `bookable`, so a price set once is what the schedule quotes and the till charges. Only a service can be made bookable.

## Billing and finance

| Methods | Path | Access |
|---|---|---|
| GET, POST | `/invoices` | Doctor, nurse |
| GET | `/invoices/{id}` | Doctor, nurse |
| POST | `/invoices/{id}/payments` | Doctor, nurse |
| POST | `/payments/{id}/refunds` | Doctor |
| POST | `/pos/checkout` | Doctor, nurse |
| GET | `/payment-methods` | Doctor, nurse |
| GET | `/cash-register` | Doctor, nurse |
| POST | `/cash-register/open` | Doctor, nurse |
| POST | `/cash-register/{id}/close` | Doctor, nurse |
| GET, POST | `/expenses` | Doctor |
| GET | `/finance/summary` | Doctor |
| GET | `/reports/{report}` | Doctor |

| GET, POST | `/income` | Doctor |
| GET | `/finance/profit-and-loss` | Doctor |
| GET, POST | `/quotes` | Doctor, nurse |
| GET | `/quotes/{id}` | Doctor, nurse |
| PATCH | `/quotes/{id}/status` | Doctor, nurse |
| POST | `/quotes/{id}/convert` | Doctor, nurse |

Refund requests accept `amountMinor`, `reason`, and optional `restockItemIds`. The refund, credit note, invoice status and selected stock returns commit atomically.

Sales, refunds and insurer remittances are written into the income ledger by database triggers as they happen, so it cannot drift from the till; a refund is a negative entry rather than a second table to reconcile. `POST /income` records money the till never saw. A quote converts to an invoice exactly once — a unique index on `converted_invoice_id` makes a double click harmless — and converting raises the debt without moving stock.

## Point of sale

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/pos/service-types` | Authenticated | Sellable clinic services; `bookable=true` for appointment types |
| GET | `/pos/prescription-cart` | Authenticated | A prescription priced and stock-checked against the catalogue |
| GET, POST | `/pos/parked` | Authenticated | List or park a cart |
| POST | `/pos/parked/{id}/resume` | Authenticated | Resume a hold once, repriced against today's catalogue |
| DELETE | `/pos/parked/{id}` | Authenticated | Discard a hold |
| GET | `/pos/sales` | Authenticated | Sales history filtered by date, cashier, method, patient or status |
| GET | `/cash-register/{id}/report` | Authenticated | Z-report; readable while the register is still open |

`POST /pos/checkout` accepts `payments` — one entry per tender — so a sale settles across cash, card and insurance as separate payments against the one invoice. The single `payment` field still works. An invoice may name the `appointmentId` or `encounterId` it paid for; neither is ever required, and one belonging to a different patient is refused.

## Human resources

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET, POST | `/hr/positions` | Doctor | Positions and their default pay |
| GET, POST | `/hr/employees` | Doctor | Staff records, paged and searchable |
| GET, PUT | `/hr/employees/{id}` | Doctor | One staff record, with optimistic concurrency |
| GET, POST | `/hr/attendance` | Doctor | Attendance register; clock in or enter a day |
| POST | `/hr/attendance/clock-out` | Doctor | Close the open shift and derive its minutes |
| GET, POST | `/hr/payroll-runs` | Doctor | Payroll runs; creating one computes every payslip |
| GET | `/hr/payroll-runs/{id}` | Doctor | A run with its payslips and breakdowns |
| PATCH | `/hr/payroll-runs/{id}/status` | Doctor | draft → approved → paid, or cancelled |
| GET | `/hr/document-templates` | Doctor | Contract and letter templates |
| GET, POST | `/hr/documents` | Doctor | Generated staff documents |

## Images

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET, POST | `/images` | Authenticated | List or attach images for `inventory_item` or `lab_order` |
| GET | `/images/{id}/content` | Authenticated | The stored image bytes |
| DELETE | `/images/{id}` | Authenticated | Remove an image and its file |

An image is at most 8 MB, capped at 12 per record, and its media type is taken from the file's own bytes rather than its name or the header sent with it.

## Insurance

| Methods | Path | Access |
|---|---|---|
| GET | `/insurance/payers` | Doctor, nurse |
| POST, PUT | `/insurance/payers[/{id}]` | Doctor |
| GET, POST | `/insurance/claims` | Doctor, nurse |
| GET | `/insurance/claims/summary` | Doctor, nurse |
| GET | `/insurance/claims/proposal` | Doctor, nurse |
| GET | `/insurance/policies` | Doctor, nurse |
| POST | `/patients/{patientId}/insurance` | Doctor |
| PATCH | `/insurance/claims/{id}/status` | Doctor |
| POST | `/insurance/claims/{id}/payments` | Doctor |

A policy carries the percentage of a bill its insurer takes, and `/insurance/claims/proposal` reads it, subtracts what has already been claimed against the invoice, and returns the proposed claim and its split. A claim may name a single `invoiceItemId` rather than a whole invoice. The headline counts come from `/insurance/claims/summary` rather than from filtering a page of claims.

This is an offline/manual workflow: staff record authorization references and insurer remittances from paper, telephone, email, cheque or bank records. No insurer API is required.

Reports support `from`, `to`, and `format=csv`. Available report keys are `sales`, `patients`, `appointments`, `clinical`, `inventory`, and `lab`.

## System

| Methods | Path | Access |
|---|---|---|
| GET | `/dashboard` | Doctor, nurse; role-shaped response |
| GET | `/search?q=` | Doctor, nurse |
| GET | `/network` | Doctor, nurse |
| GET | `/network/local-ca` | Doctor, nurse; local trust certificate |
| GET | `/settings` | Doctor, nurse |
| PUT | `/settings/{key}` | Doctor; versioned |
| GET, POST, PATCH | `/users[/{id}]` | Doctor |
| POST | `/users/{id}/password` | Doctor; invalidates sessions |
| GET | `/audit` | Doctor |
| GET, POST | `/backups` | Doctor |
| POST | `/backups/restore` | Doctor; maintenance lock |
| POST | `/backups/validate-destination` | Doctor |

Writable setting keys are explicitly allow-listed. `printing` sets the document paper (A4 or Letter) and the receipt roll width (58 mm or 80 mm) independently, because they are two different printers. `clinical.pretestPolicy` decides whether staff may skip the nurse pre-test on a consultation. `appearance` accepts supported shadcn base/accent palettes, mode and radius. `public_display` controls enablement, privacy mode, appointment visibility and the waiting-room announcement. The public-display response never includes patient IDs, record numbers, clinical data, contact details or billing data.

Every list endpoint pages on the server: `page` and `limit` are read from the query string, a limit past an endpoint's ceiling falls back to its default rather than being honoured, and nonsense values read as a request for the default page. Paged responses carry `page`, `limit`, `total` and `hasMore` alongside `items`.

## Status and error semantics

- `400`: malformed JSON or missing request structure
- `401`: missing/expired session
- `403`: role restriction
- `404`: entity or route not found
- `409`: optimistic concurrency, duplicate or scheduling conflict
- `422`: domain validation failure
- `423`: finalized encounter is locked
- `429`: authentication throttled
- `500/503`: internal persistence or availability failure

## Safe retries and network boundaries

Invoice creation, POS checkout, invoice payments and refunds accept `Idempotency-Key` (up to 128 characters). Reuse the same unpredictable key and body when retrying an uncertain result. The reservation, mutation and saved response commit in one transaction; replay returns the original result. A reused key with different data is rejected with 422. The maintained UI supplies keys and reuses them after network/server failure. Keys are held in memory: after a reload or sign-in, check the ledger before re-entering an uncertain financial action. Older clients omitting keys do not receive this protection.

API responses use `Cache-Control: private, no-store`. Browser writes require same-origin Origin/Fetch Metadata checks, and both SSE endpoints validate Origin. The authenticated stream rechecks session/account validity on each event and heartbeat. Public events carry only generic display invalidations. Wails' exemption is an in-process context marker, never a client header.

The settings API cannot configure a transcription executable. See [deployment](DEPLOYMENT.md) for the server environment setting. Backup responses include `assetsChecksumSha256`; backup listing also exposes the last automatic check/error in `status`. Restore invalidates all sessions and requires a fresh login.
