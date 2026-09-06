# Implementation status

This file distinguishes persisted, authorized workflows from visual placeholders. It is updated whenever a workflow becomes operational.

## Operational end-to-end

The following areas have a responsive UI, Go API, SQLite persistence, server-side authorization, validation/error handling, audit/realtime behavior where applicable, and automated coverage:

- first-run setup, doctor/nurse accounts, Argon2id authentication, sessions, rate limiting and RBAC;
- clinic identity, uploaded logo, configurable currencies/payment methods/timezone and user/password administration;
- patient registration, duplicate warning, demographics, structured history, archive, documents and longitudinal timeline;
- day/week/month/agenda appointments, conflict detection, rescheduling, walk-ins, check-in and waiting-room stages;
- independent nurse pre-test and doctor encounter sections, complete OD/OS examination fields, diagnoses, finalization, locking and addenda;
- spectacle, contact-lens and medication prescriptions with dedicated branded print layouts;
- suppliers, purchase orders, partial/full receipts and transactional inventory movements;
- inventory catalogue and ledger, low-stock alerts and counted-versus-expected stock-take sessions;
- POS, invoices, immutable partial payments, stock deduction, cash-register sessions and expenses;
- optical laboratory orders, advanced fitting measurements, QC gate, delivery and branded printing;
- live doctor/nurse dashboards, visit/revenue/status/sales charts, finance reports, CSV and branded browser PDF/print output;
- audit log, manual and scheduled SQLite-safe backups, configurable backup folder, retention and guarded restore;
- one responsive mobile-first PWA shared by desktop and LAN clients, SSE updates, QR access and persistent local certificate authority;
- Windows per-user NSIS installer workflow and local installer build script.

## Deliberately deferred from the current delivery

These items are not represented as finished features:

- insurance/third-party claim processing and payer receivable aging;
- a guided refunds/credit-note workspace (the doctor-only refund API foundation remains available);
- a true OS-native tray command menu; closing the Wails window currently keeps the local server available through the supported minimize behavior;
- code-signed installers and OS notarization. The supplied installer is plug-and-play but unsigned until the publisher provides signing certificates;
- formal regulatory certification, penetration testing and jurisdiction-specific clinical/legal approval.

## Release validation still required on target hardware

Before putting real patient data into service, the clinic or deployment partner must:

1. build the installer in CI or on the target Windows toolchain and perform a clean install/upgrade test;
2. approve the Windows firewall prompt, install the generated local CA on authorized LAN devices and verify the QR URL;
3. test touch behavior and printing on the clinic's actual phones, tablets, printer and paper sizes;
4. exercise backup and restore with the selected external destination;
5. establish data-retention, device-security and incident-response procedures appropriate to the clinic's jurisdiction.

The repository's automated suite covers database migrations, authentication/RBAC, optimistic concurrency, concurrent doctor/nurse writes, encounter locking, appointments, purchasing, stock takes, transactional POS/inventory, partial payments, backup/restore and the principal browser workflow.
