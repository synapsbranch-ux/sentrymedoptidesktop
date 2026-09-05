# Implementation status

This document prevents a rendered screen or schema-only table from being mistaken for a finished clinical feature.

## Implemented end to end

These areas have UI, API, persistence, authorization, validation/error handling and tests or acceptance coverage:

- setup, login/logout, doctor/nurse RBAC and audit;
- patient demographics, duplicate warning, archive, structured medical/ocular history and timeline;
- appointments, conflict detection, walk-in/check-in and waiting queue;
- encounter creation, independent pre-test, OD/OS refraction, diagnosis, prescription, finalization and addenda model;
- local patient documents;
- inventory catalog, quantity ledger, low stock and stock adjustment;
- POS invoice/payment/stock transaction, partial payment, receipts and cash sessions;
- lab order status board, fitting values, QC requirement and delivery;
- expenses, finance summaries, reports and CSV;
- clinic/users/network/QR/audit settings;
- manual/automatic backup, retention and guarded restore;
- SSE synchronization and responsive PWA shell.

## Backend present; UI depth still limited

- suppliers and purchase-order receiving have transactional APIs but no dedicated purchasing workspace;
- contact-lens and medication prescription types persist, but their specialty forms/print layouts need expansion;
- anterior/posterior clinical data can be stored in versioned encounter sections, but the current doctor UI focuses on refraction and assessment;
- refunds exist as doctor-only API behavior but do not yet have a guided UI;
- stock-take can be represented by correction movements, but a counted-versus-expected batch screen is not present;
- insurance/payer tables exist, but claims API/UI and receivable aging are not yet implemented;
- reports provide live tables, CSV and browser print; server-generated branded PDF export is not implemented.
- appointment scheduling currently provides a responsive agenda/work-queue view; dedicated day/week/month calendar grids are not yet implemented;
- dashboard metrics and operational lists are live, but the full requested financial/visit chart suite is not yet implemented.

## Required before a production clinic release

- complete the limited workspaces above and add their browser acceptance tests;
- add a true native system-tray menu (current stable Wails 2 build minimizes and keeps the server running);
- package a guided certificate/DNS enrollment flow (TLS certificate/key and public URL are currently environment-configured);
- finish native print integrations and dedicated A4/A5 layouts for every required document;
- run Playwright on all supported targets and validate touch behavior on physical iOS/Android devices;
- perform threat modeling, dependency/security review, data-retention review and jurisdiction-specific clinical/legal validation;
- build and sign installers for each target OS and test upgrades/restores using real packaged builds.

The checked-in CI runs the mobile acceptance flow. Go tests, frontend tests, strict builds and E2E typecheck passed in the authoring environment; its local Chromium download timed out, and the complete Playwright runtime flow was subsequently validated by the green GitHub Actions pipeline.
