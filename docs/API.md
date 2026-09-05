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
| POST | `/api/v1/setup/complete` | Server computer | Create clinic settings and initial doctor |
| POST | `/api/v1/auth/login` | Public/rate-limited | Start session |
| POST | `/api/v1/auth/logout` | Authenticated | Invalidate session |
| GET | `/api/v1/auth/me` | Authenticated | Current user without password hash |
| GET | `/api/v1/events` | Authenticated | SSE update stream |

## Patients, appointments and queue

| Methods | Path | Access |
|---|---|---|
| GET, POST | `/patients` | Doctor, nurse |
| GET, PUT, DELETE | `/patients/{id}` | Doctor, nurse; delete archives |
| GET, PUT | `/patients/{id}/history` | Doctor, nurse; versioned |
| GET | `/patients/{id}/timeline` | Doctor, nurse |
| GET, POST | `/appointments` | Doctor, nurse |
| PATCH | `/appointments/{id}/status` | Doctor, nurse; versioned |
| GET | `/queue` | Doctor, nurse |
| POST | `/queue/check-in` | Doctor, nurse; appointment optional |
| PATCH | `/queue/{id}` | Doctor, nurse; versioned |

Patient listing accepts `q`, `page`, and `limit`. Appointments accept a `date=YYYY-MM-DD` filter.

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
| GET | `/suppliers` | Doctor, nurse |
| POST | `/suppliers` | Doctor |
| POST | `/purchase-orders` | Doctor |
| POST | `/purchase-orders/{id}/receive` | Doctor |
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

Reports support `from`, `to`, and `format=csv`. Available report keys are `sales`, `patients`, `appointments`, `clinical`, `inventory`, and `lab`.

## System

| Methods | Path | Access |
|---|---|---|
| GET | `/dashboard` | Doctor, nurse; role-shaped response |
| GET | `/search?q=` | Doctor, nurse |
| GET | `/network` | Doctor, nurse |
| GET | `/settings` | Doctor, nurse |
| PUT | `/settings/{key}` | Doctor; versioned |
| GET, POST, PATCH | `/users[/{id}]` | Doctor |
| GET | `/audit` | Doctor |
| GET, POST | `/backups` | Doctor |
| POST | `/backups/restore` | Doctor; maintenance lock |

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

