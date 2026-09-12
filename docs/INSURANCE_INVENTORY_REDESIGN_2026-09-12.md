# Insurance, Inventory, Currency & Download Redesign — 2026-09-12

Final report for the full audit-and-redesign pass covering: insurance/assurance
workflows, inventory management, the money-formatting bug, the desktop
download-restart bug, dead quotation code, and RBAC/audit hardening.
Branch: `claude/loving-bell-83ojcu`.

## 1. Bugs discovered and root causes

### 1.1 Currency: "650 HTG" stored/displayed as "6.5 HTG"

**Root cause:** the backend was never wrong — all money is stored as integer
minor units (`*_minor` columns, e.g. `costMinor`, `totalMinor`) and arithmetic
was already integer-only. The bug was in the frontend: several forms bound a
plain `<input type="number">` directly to a `*Minor` field, so a staff member
typing `"650"` (meaning 650 HTG) produced the integer `650`, which is 6.50 HTG
once divided by 100 for display — a 100x data-entry error baked into the
stored row, not a display artifact. This pattern was repeated independently in
inventory, purchasing, billing, and POS forms.

**Fix:** a centralized money layer so no page hand-rolls minor-unit math or
parsing again:
- `apps/web/src/money.ts` — `formatMoney`, `minorToInputValue`,
  `parseMoneyInput` (digit-string based; a single separator followed by
  exactly 3 digits is treated as thousands grouping rather than a decimal
  point, so `"6,500"` parses as 6500 major units, not 6.5), `addMoney` /
  `subtractMoney` / `sumMoney` / `calculateBalance` / `splitByPercent`.
- `apps/web/src/components/ui/money-input.tsx` — `<MoneyInput>`: keeps local
  text state while focused (so it doesn't fight the user mid-keystroke) and
  reformats canonically on blur; renders the currency code as a badge inside
  the field instead of in the label.
- `internal/server/money.go` — `applyPercentMinor` (round-half-up) and
  `splitMinor` (returns `first, remainder` such that `first+remainder` always
  equals the input exactly — no rounding leak) for the few places the backend
  itself splits a minor-unit amount by a percentage (insurance coverage,
  payroll deductions).
- Every raw minor-unit `<Input type="number">` bound to money in
  `inventory.tsx`, `billing.tsx`, and `pos.tsx` was replaced with
  `<MoneyInput>`.
- Tests: `apps/web/src/money.test.ts`, `internal/server/money_test.go`.

### 1.2 Download causing the desktop app to need a restart

**Root cause:** architectural, not a per-button bug. The Wails `main.go` had
no `Linux:` webview options and no download delegate, so on the Linux
(WebKitGTK) build a browser-style download (`<a download>` / navigating to a
file response) fell through to WebKitGTK's own unconfigured download
handling, which can block or wedge the single-threaded GTK/JS event loop badly
enough that the only recovery is restarting the app. Every "download" button
in the app used this same browser-download path, which is why the symptom
showed up across unrelated document types.

**Fix:** a native save path for the desktop build, so downloads never touch
the webview's own download machinery:
- `main.go` — `DesktopBridge.SaveFile(suggestedName string, data []byte)
  (string, error)` using Wails' `runtime.SaveFileDialog` (a native OS Save As
  dialog, opened and closed on the Go side).
- `apps/web/src/native.ts` — bridge interface extended with
  `SaveFile(suggestedName, base64Data)`.
- `apps/web/src/download.ts` — `saveBlob(blob, filename)`: on desktop,
  base64-encodes via `FileReader` and calls the native bridge; on browser/PWA
  (LAN clients with no Wails bridge), falls back to the standard
  Blob + temporary `<a download>` + click, wrapped in `try/finally` with a
  delayed `URL.revokeObjectURL` so the browser path can't leak object URLs
  either.
- Every download call site was migrated to `saveBlob`/`downloadFromApi`
  instead of a bare `<a href>` or ad-hoc blob click: `document-viewer.tsx`,
  `reports.tsx` (CSV export), `system.tsx` (CA certificate download).
  `printDocument()` in `components/printing.tsx` also had its
  `window.print()` call wrapped in `try/finally` so a print failure can't
  leave the print CSS state stuck.
- Tests: `apps/web/src/download.test.ts` exercises repeated downloads of
  different document types in one session (the original failure mode) against
  both the desktop and browser branches.

`printThermalReceipt` / `printReceipt` / `receiptMarkup` in `printing.tsx`
were left as-is: confirmed dead code, unreferenced by any page, superseded by
the server-driven Bluetooth thermal-printer path. Documented here rather than
silently deleted since removing dead code was in scope for quotes but not
explicitly requested here.

## 2. Dead quotation code

A "Quote"/"Quotation" feature existed as a full vertical slice (backend
handler, frontend page, nav entry, locale strings) despite not being part of
the current product. Removed completely, not hidden:
- Deleted `internal/server/quotes.go`, `internal/server/quotes_test.go`,
  `apps/web/src/pages/quotes.tsx`.
- Removed route registration (`s.registerQuoteRoutes`) from
  `internal/server/operations.go`.
- Removed all navigation/menu/feature-flag wiring from `App.tsx`,
  `app-shell.tsx`, `app-shell.test.ts`, `features.ts`.
- Removed the 27 quote-only i18n keys from all 10 locale files.
- Updated prose in `README.md`, `docs/API.md`, `docs/IMPLEMENTATION_STATUS.md`.
- `internal/database/migrations/021_income_and_quotes.sql` was **left in
  place** — migrations already applied to existing databases are never
  edited or removed; the quote tables it created are simply unused now.

No quotation entity, table, route, PDF, status, or workflow was reintroduced
anywhere in this branch's other work (insurance and inventory both use their
own vocabularies — "claim" and "purchase order" — never "quote").

## 3. Insurance / assurance redesign

New migration: `internal/database/migrations/029_insurance_redesign.sql`
(additive only — new columns via `ALTER TABLE`, two new tables, one partial
unique index; nothing destructive).

**Providers (payers)** — full CRUD (`internal/server/insurance.go`):
name, contact info, logo (via the existing generic image-gallery
infrastructure, `entityType: "payer"`), accepted coverage, billing info,
claim instructions, default coverage percent, active flag, plus a
provider-scoped "required forms" checklist backing the document workflow.

**Patient insurance (patient_insurance)** — coverage details (member/policy/
group number, subscriber, relationship, authorization, coverage percent,
effective/expiration dates), set-primary (enforced as at-most-one active
primary policy per patient both transactionally in the handler and at the DB
level via a partial unique index:
`WHERE is_primary=1 AND is_active=1`), deactivate/reactivate, delete (blocked
while any card or document still references the policy), and manual
verification (`not_verified` / `pending_verification` / `verified` /
`expired` / `rejected`, with verifier name, verification date, reference, and
notes — explicitly a manual/staff-attested workflow, no pretense of an
electronic eligibility check).

**Insurance cards** (`patient_insurance_cards`, new table) — front/back
upload and replace, mobile camera capture via
`<input type="file" accept="image/*" capture="environment">` (no custom
camera code needed), delete. Stored through the same content-sniffed,
SHA-256-checksummed, UUID-named file pattern as the rest of the app's
uploads (`os.OpenFile` with `O_EXCL`, never trusting the client's filename or
extension) and served from a non-public directory
(`internal/app/config.go` / `internal/backup/assets.go` both updated so the
new `insurance-cards` directory is created at startup and included in
backups).

**Insurance documents** (`insurance_documents`, new table,
`internal/server/insurance_documents.go`) — the REQUIRED → REQUESTED →
RECEIVED → COMPLETED / SUBMITTED / REJECTED / EXPIRED status lifecycle, file
attachment, and an `existing_document_id` column so a document requirement
can point at a file already stored in the generic `documents` table instead
of duplicating it.

**Compact insurance indicators** — `apps/web/src/components/
insurance-indicator.tsx`, a small non-blocking badge wired into the
consultation/encounter header in `clinical.tsx` (not rolled out to the
appointment list rows — see Limitations).

**Billing split** (`internal/server/billing.go`) — invoices now expose
`insuranceClaims`, `insuranceExpectedMinor`, `insuranceReceivedMinor`, and
`patientResponsibilityMinor` alongside the existing total/paid/balance, and
settlement is computed from patient payments **and** insurer claim payments
together (`invoiceSettledMinor` / `syncInvoiceStatusAfterInsurancePayment`),
so an invoice only reaches fully-paid once both sides are accounted for — it
is never marked paid merely because the patient covered their own portion.
`apps/web/src/pages/billing.tsx` renders this split.

**Claims** (`insurance_claims`, extended in place) — linkable to an
encounter/appointment/prescription, claim amount vs. patient portion vs.
payer portion, an additive `approved_amount_minor` column distinct from the
claimed amount so a partial approval is representable without touching the
existing CHECK-constrained status column (see §5 for why), PDF generation
reusing the existing print/`document-print` CSS architecture rather than a
new PDF engine, and document attachment per claim.

**Insurance payments** — `handleClaimPayment` records PATIENT vs. INSURANCE
payment sources; a payment ceiling now uses `approved_amount_minor` when the
claim was partially approved (previously it incorrectly used the full
`payer_portion_minor`, which would have let an insurer's payment "overshoot"
what was actually approved). A partial payment leaves the remaining
difference visibly outstanding rather than silently closing the claim —
covered by `TestPartialInsurancePaymentLeavesADisposableDifference`.

## 4. Inventory redesign

New migration: `internal/database/migrations/030_inventory_redesign.sql` —
additive columns on `inventory_items` (`unit`, `batch_number`,
`expiration_date`, `notes`); `stock_movements` rebuilt (SQLite's
create-copy-drop-rename pattern, safe here because nothing holds a foreign
key into it) solely to extend its CHECK constraint with `'expired'`.

**Products** (`internal/server/inventory.go`) — full CRUD was previously
missing Update/Archive/Delete entirely; added `handleInventoryGet`,
`handleInventoryUpdate`, `handleInventoryArchive` / `handleInventoryReactivate`
(soft-delete, optimistic-locked via `version`), and a permanent
`handleInventoryDelete` that is refused (422) if the item has any stock
movement history, so a product that was ever received, sold, or adjusted can
only be archived, never erased.

**Stock movements** — vocabulary now covers `initial_stock`, `purchase`
(receiving), `manual_adjustment` (in/out via signed quantity), `sale`,
`return`, `damage`, `expired` (newly added), `loss`, `transfer` (in/out).
Receiving stock always creates a movement row and derives the new quantity
from `previous_quantity + change` under the row's `version` check — it never
overwrites `quantity` directly, so every change to on-hand stock has a
traceable cause. Damage/expired/loss movements accept a positive quantity
(as a person would type "2 damaged") and the server coerces the sign, always
subtracting.

**Concurrency safety** — every stock mutation is gated by the same
optimistic-locking pattern as the rest of the app
(`UPDATE ... WHERE id=? AND version=?`; zero rows affected → 409
`CONCURRENT_MODIFICATION`). `TestConcurrentStockMovementsCannotCorruptQuantity`
fires two simultaneous "sale of 3" requests against the same starting
version and asserts exactly one succeeds, the other gets 409, and the final
quantity reflects exactly one deduction — never a lost update, never a
double deduction.

**History/traceability** — `GET /inventory/{id}/movements` lists every
movement with previous/change/resulting quantity, reason, reference, actor,
and timestamp; surfaced in the frontend as a History tab in the product
detail dialog (`inventory.tsx`).

**Reports** — added `low-stock` (items at or below reorder level) and
`stock-movements` report kinds to the existing `/reports/{report}`
infrastructure (`internal/server/finance.go`), which already provided
CSV/print/JSON output — reused rather than building new report plumbing.
The existing inventory-valuation report was left as-is.

## 5. A deliberate SQLite migration constraint

SQLite migrations in this codebase run inside an already-open transaction
(`tx.ExecContext`), so `PRAGMA foreign_keys=OFF` mid-migration is a no-op.
That means a CHECK-constrained column with an *incoming* foreign key (e.g.
`insurance_claims`, referenced by `insurance_claim_payments`) cannot safely
have its CHECK enum extended via the usual SQLite
create-new-table/copy/drop/rename rebuild — the rebuild would need FK
enforcement suspended, which isn't possible here. `stock_movements` has no
incoming FK, so it was safely rebuilt to add `'expired'` to its CHECK
constraint. `insurance_claims` was not rebuilt; "partial approval" is instead
represented as an additive `approved_amount_minor` column compared against
the claimed amount, rather than as a new status value. This is a real
constraint of the existing migration runner, not a shortcut — future status
enum changes on tables with incoming FKs will need the same additive-column
treatment, or a migration-runner change to suspend FK enforcement outside the
transaction.

## 6. RBAC / permissions

All sensitive actions are authorized on the backend (`internal/server/
operations.go` route registration), not merely hidden in the UI. Doctor-only
(`requireRole` / doctor gate) on the backend:
- Payer create/update; payer/card/document deletion.
- Patient insurance create/update/set-primary/deactivate/reactivate/verify/delete.
- Insurance claim update/status-change/payment.
- Inventory archive/reactivate/permanent-delete.

Not doctor-gated (routine clinical/front-desk operations, still authenticated):
reads of payers/policies/claims/inventory; insurance card/document upload
and status progression (front-desk staff commonly handle intake paperwork);
inventory item create/update and stock movements (routine receiving and
adjustment is front-desk/inventory-clerk work in this clinic's workflow, not
a doctor-only action — noted as a judgment call, not a gap, since it mirrors
how purchase orders were already gated before this redesign).

The frontend mirrors these same gates for UX (buttons hidden/disabled for
non-doctors), but every one of the above is re-checked server-side regardless
of what the UI shows, so a crafted request from a nurse session cannot bypass
it.

## 7. Audit trail

Every mutation above calls `s.audit(...)` with an action, entity type, entity
id, and a description free of sensitive medical detail (e.g. "Recorded
insurance policy with Acme Insurance", never diagnosis/treatment content):
`create`/`update`/`delete`/`status`/`payment`/`set_primary`/`verify` on
`payer`, `patient_insurance`, `patient_insurance_card`, `insurance_claim`,
`insurance_document`, and `create`/`update`/`delete`/archive-reactivate/
`stock_movement` on `inventory_item`. This follows the codebase's existing
lowercase action/entity-type convention (matching how every other module
already audits) rather than the uppercase example names in the original
brief (e.g. `PATIENT_INSURANCE_CREATED`) — the underlying who/what/when/
related-entity information captured is equivalent; only the casing style
differs, and consistency with the rest of the app's audit log was prioritized
over matching the example strings literally.

## 8. Data integrity / transactions

Financial and inventory mutations that touch more than one row (recording a
payment and updating a claim's paid total; recording a stock movement and
updating the item's quantity; setting a policy primary by demoting the
previous primary first) are performed inside a single DB transaction, so a
crash or error mid-operation cannot leave a payment recorded without its
balance update, or a stock change recorded without its resulting quantity.
Optimistic-locking (`version`) failures roll the transaction back entirely
rather than partially applying.

## 9. Tests added

- `internal/server/money_test.go`, `apps/web/src/money.test.ts` — exact
  integer arithmetic, round-half-up percentage splits, no floating-point
  drift.
- `apps/web/src/download.test.ts` — repeated downloads across document types
  in one session, both the desktop-bridge and browser-fallback paths.
- `internal/server/insurance_redesign_test.go` (~10 tests) — primary-policy
  uniqueness, card upload/replace/delete permissions, policy deletion blocked
  while cards/documents exist, document request→receive→status workflow,
  patient+insurance payments together settling an invoice, partial insurance
  payment leaving a visible outstanding difference.
- `internal/server/inventory_redesign_test.go` — full product CRUD (create,
  update, archive, reactivate, permanent-delete blocked/allowed), receiving
  stock creates an attributable movement, damage/expired/loss always reduce
  stock regardless of sign typed, **concurrent stock movements cannot
  corrupt quantity**, low-stock report correctness.
- `apps/web/src/pages/pos.test.tsx` — updated for `MoneyInput`'s decimal
  major-unit entry instead of raw minor units.

All of the above pass, alongside the full pre-existing suite: `go test
./internal/...` (all packages), `npx tsc --noEmit` (clean), `npx vitest run`
(31 files / 184 tests passing), `go build ./...` (clean), `gofmt -l` (clean).

## 10. Files changed (by area)

- **Money:** `apps/web/src/money.ts`, `apps/web/src/components/ui/
  money-input.tsx`, `apps/web/src/lib.ts`, `internal/server/money.go`,
  `internal/server/insurance.go`, `internal/server/hr_payroll.go`,
  `apps/web/src/pages/billing.tsx`, `apps/web/src/pages/pos.tsx`.
- **Downloads:** `main.go`, `apps/web/src/native.ts`,
  `apps/web/src/download.ts`, `apps/web/src/components/printing.tsx`,
  `apps/web/src/components/document-viewer.tsx`,
  `apps/web/src/pages/reports.tsx`, `apps/web/src/pages/system.tsx`.
- **Quotes removed:** `internal/server/quotes.go` (deleted),
  `internal/server/quotes_test.go` (deleted),
  `apps/web/src/pages/quotes.tsx` (deleted), `internal/server/operations.go`,
  `apps/web/src/App.tsx`, `apps/web/src/components/app-shell.tsx`,
  `apps/web/src/features.ts`, 10 locale files, `README.md`, `docs/API.md`,
  `docs/IMPLEMENTATION_STATUS.md`.
- **Insurance:** `internal/database/migrations/029_insurance_redesign.sql`,
  `internal/server/insurance.go`, `internal/server/insurance_documents.go`,
  `internal/server/billing.go`, `internal/server/images.go`,
  `internal/app/config.go`, `internal/backup/assets.go`,
  `internal/server/operations.go`, `apps/web/src/types.ts`,
  `apps/web/src/pages/insurance.tsx`, `apps/web/src/pages/patients.tsx`,
  `apps/web/src/components/insurance-indicator.tsx`,
  `apps/web/src/components/image-gallery.tsx`.
- **Inventory:** `internal/database/migrations/030_inventory_redesign.sql`,
  `internal/server/inventory.go`, `internal/server/finance.go`,
  `internal/server/operations.go`, `apps/web/src/types.ts`,
  `apps/web/src/pages/inventory.tsx`, `apps/web/src/pages/reports.tsx`.
- **Tests:** `internal/server/money_test.go`, `apps/web/src/money.test.ts`,
  `apps/web/src/download.test.ts`,
  `internal/server/insurance_redesign_test.go`,
  `internal/server/inventory_redesign_test.go`,
  `internal/server/server_test.go`, `apps/web/src/pages/pos.test.tsx`.

## 11. Remaining limitations

- Stock movement vocabulary keeps `manual_adjustment` and `transfer` as
  single types distinguished by the sign of the quantity change, rather than
  splitting each into explicit `_IN`/`_OUT` variants as one reading of the
  brief's vocabulary suggested — a deliberate scope trim; the sign is always
  visible in `quantityChange` and the History tab color-codes direction.
- No shortcut from an invoice directly into "create claim" pre-filled; staff
  create a claim from the Insurance page and pick the invoice from a
  dropdown.
- `InsuranceIndicator` was wired into the consultation/encounter header only,
  not into appointment-list rows.
- Inventory categories remain the existing fixed enum (frame /
  ophthalmic_lens / contact_lens / accessory / service) — a dynamic/
  manageable category list was not built.
- `printThermalReceipt` / `printReceipt` / `receiptMarkup` in `printing.tsx`
  were left in place despite a similar unguarded-native-dialog shape, since
  they are confirmed dead code with no callers.
- Electronic insurance eligibility verification does not exist and was not
  built — verification is explicitly manual/staff-attested, per the brief.

## 12. Recommended future improvements

- Extend `InsuranceIndicator` to appointment-list and queue rows for earlier
  front-desk visibility of coverage status.
- Add an "Insurance" quick-action from an invoice's insurance-split panel
  straight into claim creation, pre-filled with the invoice and patient.
- If a future migration needs to extend a CHECK-constrained enum on a table
  with incoming foreign keys, extend the migration runner to run that one
  migration's table-rebuild outside the ambient transaction so
  `PRAGMA foreign_keys=OFF` actually takes effect, rather than continuing to
  route around it with additive columns.
- Consider a lightweight electronic-eligibility integration as an optional,
  clearly-labeled enhancement layered on top of the existing manual
  verification workflow, never replacing it (still local-first, so this
  would only apply where a clinic has real network connectivity to a payer).
