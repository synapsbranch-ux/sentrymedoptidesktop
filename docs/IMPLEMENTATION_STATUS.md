# Implementation status

This file distinguishes persisted, authorized workflows from visual placeholders. It is updated whenever a workflow becomes operational.

## Operational end-to-end

The following areas have a responsive UI, Go API, SQLite persistence, server-side authorization, validation/error handling, audit/realtime behavior where applicable, and automated coverage:

- first-run setup, doctor/nurse accounts, Argon2id authentication, sessions, rate limiting and RBAC;
- clinic identity, app-wide uploaded logo, configurable shadcn theme palettes/modes, server-persisted language selection, currencies/payment methods/timezone and user/password administration;
- patient registration, duplicate warning, demographics, structured history, archive, documents and longitudinal timeline;
- day/week/month/agenda appointments, conflict detection, rescheduling, walk-ins, check-in and waiting-room stages;
- independent nurse pre-test and doctor encounter sections, complete OD/OS examination fields, diagnoses, finalization, locking and addenda;
- spectacle, contact-lens and medication prescriptions with dedicated branded print layouts;
- suppliers, purchase orders, partial/full receipts and transactional inventory movements;
- inventory catalogue and ledger, low-stock alerts and counted-versus-expected stock-take sessions;
- POS, invoices, immutable partial payments, stock deduction, cash-register sessions and expenses;
- manual insurance workflow for Haitian clinics: insurers, authorizations, patient/payer portions, claim stages, remittances and receivable aging;
- guided refunds with immutable credit notes and transactional return-to-stock selection;
- laboratory and optical orders in one section: spectacle orders raised from a prescription that carry the powers, the consultation and the till sale to the glazing company, examination requests raised from an editable test list, advanced fitting measurements, a completed QC checklist gating optical delivery, and a branded printed document with the frame photograph on it;
- live doctor/nurse dashboards, visit/revenue/status/sales charts, finance reports, CSV and branded browser PDF/print output;
- searchable diagnosis reference of 502 ophthalmology ICD-10 codes with lay-language synonyms, browsable by category and extendable by CSV import;
- income ledger generated from every payment, refund and insurer remittance, manual income entries, and an end-to-end profit and loss view;
- POS service types shared with scheduling, prescription-to-cart lookup, split tenders, parked sales, barcode scanning, sales history and the register Z-report;
- human resources: staff records, positions, attendance, payroll runs, payslips and templated contract generation;
- images on stock items and lab orders, batch lab dispatch and a batched printed requisition;
- separate thermal-receipt and standard-paper print paths with their own page geometries;
- audit log, manual and scheduled SQLite-safe backups, configurable backup folder, retention and guarded restore;
- one responsive mobile-first PWA shared by desktop and LAN clients, touch-scroll navigation and bottom-sheet dialogs, SSE with polling recovery, QR access and persistent local certificate authority;
- privacy-filtered live public queue/appointment display with TV URL, QR access and clinic announcement;
- Windows per-user NSIS installer workflow and local installer build script.
- native Windows system-tray menu with server state, connected users, mobile access, backup, stop and exit actions.

## Localization coverage

The clinic language is selected during first-run setup or under **System → Language** and is propagated live to desktop and LAN/PWA clients. Supported locale codes are English (`en`), French (`fr`), Haitian Creole (`ht`), Portuguese (`pt`), Spanish (`es`), German (`de`), Simplified Chinese (`zh-CN`), Russian (`ru`), Japanese (`ja`), Korean (`ko`) and Indonesian (`id`). Navigation, authentication, shared controls, connection states, language settings, common workflow terms and the public waiting-room display use the translation catalogue. Untranslated specialist text safely falls back to English instead of showing broken keys.

## Release validation still required on target hardware

Before putting real patient data into service, the clinic or deployment partner must:

1. build the installer in CI or on the target Windows toolchain and perform a clean install/upgrade test;
2. approve the Windows firewall prompt, install the generated local CA on authorized LAN devices and verify the QR URL;
3. test touch behavior and printing on the clinic's actual phones, tablets, printer and paper sizes;
4. exercise backup and restore with the selected external destination;

The repository's automated suite covers database migrations, authentication/RBAC, optimistic concurrency, concurrent doctor/nurse writes, encounter locking, appointments, purchasing, stock takes, transactional POS/inventory, partial payments, backup/restore and the principal browser workflow.
