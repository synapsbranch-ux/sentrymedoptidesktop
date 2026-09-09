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
| PUT | `/encounters/{id}/sections/{section}` | Section-aware authorization/version |
| POST | `/encounters/{id}/diagnoses` | Doctor |
| POST | `/encounters/{id}/finalize` | Doctor |
| POST | `/encounters/{id}/addenda` | Doctor, finalized encounter only |
| GET | `/prescriptions` | Doctor, nurse |
| POST | `/prescriptions` | Doctor |
| GET, POST | `/documents` | Doctor, nurse |
| GET | `/documents/{id}/download` | Doctor, nurse |
| DELETE | `/documents/{id}` | Doctor, nurse; archives metadata |

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

Marking a lab order `ready` fails until a QC row exists.

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

Refund requests accept `amountMinor`, `reason`, and optional `restockItemIds`. The refund, credit note, invoice status and selected stock returns commit atomically.

## Insurance

| Methods | Path | Access |
|---|---|---|
| GET | `/insurance/payers` | Doctor, nurse |
| POST, PUT | `/insurance/payers[/{id}]` | Doctor |
| GET, POST | `/insurance/claims` | Doctor, nurse |
| PATCH | `/insurance/claims/{id}/status` | Doctor |
| POST | `/insurance/claims/{id}/payments` | Doctor |

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

Writable setting keys are explicitly allow-listed. `appearance` accepts supported shadcn base/accent palettes, mode and radius. `public_display` controls enablement, privacy mode, appointment visibility and the waiting-room announcement. The public-display response never includes patient IDs, record numbers, clinical data, contact details or billing data.

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
