-- D2: appointment reasons and prescription items become catalogs the clinic
-- edits from the interface, instead of lists compiled into the application.
CREATE TABLE catalog_entries (
    id TEXT PRIMARY KEY,
    catalog TEXT NOT NULL CHECK (catalog IN ('appointment_reason', 'prescription_item')),
    label TEXT NOT NULL,
    -- Per-catalog defaults; a prescription item carries strength, dosage,
    -- frequency, route, duration and instructions to prefill a prescription.
    details_json TEXT NOT NULL DEFAULT '{}',
    sort_order INTEGER NOT NULL DEFAULT 0,
    -- Retired entries are deactivated, never deleted, so a record that already
    -- refers to one keeps its meaning.
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    -- Null for the entries this migration seeds: they predate any user account,
    -- because migrations run before first-run setup creates one.
    updated_by TEXT REFERENCES users(id)
);
CREATE UNIQUE INDEX idx_catalog_entries_label ON catalog_entries(catalog, lower(label));
CREATE INDEX idx_catalog_entries_active ON catalog_entries(catalog, active, sort_order, label);

-- Seeded only with the four visit types the application already hardcoded, so
-- nothing the clinic sees today disappears. The prescription catalog starts
-- empty on purpose: inventing a medication list without clinic confirmation
-- would put clinical content into the record that nobody approved.
INSERT INTO catalog_entries(id, catalog, label, sort_order, created_at, updated_at) VALUES
  ('seed-reason-eye-exam', 'appointment_reason', 'Eye examination', 10, datetime('now'), datetime('now')),
  ('seed-reason-follow-up', 'appointment_reason', 'Follow-up', 20, datetime('now'), datetime('now')),
  ('seed-reason-contact-lens', 'appointment_reason', 'Contact lens', 30, datetime('now'), datetime('now')),
  ('seed-reason-optical-delivery', 'appointment_reason', 'Optical delivery', 40, datetime('now'), datetime('now'));
