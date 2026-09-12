-- The lab section is named for two jobs and only ever did one. A doctor orders
-- glasses from the glazing company, and a doctor orders laboratory exams; both
-- leave the clinic, travel to an outside firm, come back and are handed to the
-- patient. They share a number series, a status board, a batch dispatch and a
-- printed request, so they share a table and differ by kind rather than living
-- in two parallel copies of the same workflow.
ALTER TABLE lab_orders ADD COLUMN kind TEXT NOT NULL DEFAULT 'optical';

-- An optical order reaches the consultation through its prescription. An exam
-- request has no prescription in between, so it needs the link of its own.
ALTER TABLE lab_orders ADD COLUMN encounter_id TEXT REFERENCES encounters(id);

CREATE INDEX idx_lab_kind ON lab_orders(kind, status, expected_at);

-- The exams one request asks for.
CREATE TABLE lab_order_tests (
    id TEXT PRIMARY KEY,
    lab_order_id TEXT NOT NULL REFERENCES lab_orders(id),
    -- Copied from the catalog at order time rather than referenced. Retiring an
    -- entry must never change what a laboratory was already asked to perform.
    label TEXT NOT NULL,
    code TEXT,
    specimen TEXT,
    notes TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_lab_order_tests_order ON lab_order_tests(lab_order_id, sort_order);

-- Adding a catalog should cost a seed row, not a table rebuild. The same
-- reasoning as migration 009: the CHECK constraint had to be rewritten for every
-- new list, so it is dropped here and `catalogNames` in
-- internal/server/catalogs.go becomes the single source of truth for which
-- catalogs exist. Nothing references this table, so the rebuild is local.
CREATE TABLE catalog_entries_open (
    id TEXT PRIMARY KEY,
    catalog TEXT NOT NULL,
    label TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}',
    sort_order INTEGER NOT NULL DEFAULT 0,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id)
);

INSERT INTO catalog_entries_open (id, catalog, label, details_json, sort_order, active, version, created_at, updated_at, updated_by)
SELECT id, catalog, label, details_json, sort_order, active, version, created_at, updated_at, updated_by
FROM catalog_entries;

DROP TABLE catalog_entries;
ALTER TABLE catalog_entries_open RENAME TO catalog_entries;
CREATE UNIQUE INDEX idx_catalog_entries_label ON catalog_entries(catalog, lower(label));
CREATE INDEX idx_catalog_entries_active ON catalog_entries(catalog, active, sort_order, label);

-- The three lens types the interface already hardcoded are seeded so nothing the
-- clinic sees today disappears, exactly as migration 015 reasoned. Materials,
-- coatings, tints and treatments are commercial options rather than clinical
-- content, and an empty list would leave the order form unusable on the first
-- day, so the standard optical set is seeded too. The clinic edits all of it
-- from System -> Catalogs.
INSERT INTO catalog_entries(id, catalog, label, sort_order, created_at, updated_at) VALUES
  ('seed-lens-type-single', 'lens_type', 'Single vision', 10, datetime('now'), datetime('now')),
  ('seed-lens-type-progressive', 'lens_type', 'Progressive', 20, datetime('now'), datetime('now')),
  ('seed-lens-type-bifocal', 'lens_type', 'Bifocal', 30, datetime('now'), datetime('now')),

  ('seed-lens-material-cr39', 'lens_material', 'CR-39 (index 1.50)', 10, datetime('now'), datetime('now')),
  ('seed-lens-material-poly', 'lens_material', 'Polycarbonate (index 1.59)', 20, datetime('now'), datetime('now')),
  ('seed-lens-material-161', 'lens_material', 'High index 1.61', 30, datetime('now'), datetime('now')),
  ('seed-lens-material-167', 'lens_material', 'High index 1.67', 40, datetime('now'), datetime('now')),
  ('seed-lens-material-174', 'lens_material', 'High index 1.74', 50, datetime('now'), datetime('now')),
  ('seed-lens-material-trivex', 'lens_material', 'Trivex', 60, datetime('now'), datetime('now')),

  ('seed-lens-coating-ar', 'lens_coating', 'Anti-reflective', 10, datetime('now'), datetime('now')),
  ('seed-lens-coating-hard', 'lens_coating', 'Scratch-resistant hard coat', 20, datetime('now'), datetime('now')),
  ('seed-lens-coating-uv', 'lens_coating', 'UV protection', 30, datetime('now'), datetime('now')),
  ('seed-lens-coating-blue', 'lens_coating', 'Blue light filter', 40, datetime('now'), datetime('now')),
  ('seed-lens-coating-oleo', 'lens_coating', 'Anti-smudge', 50, datetime('now'), datetime('now')),

  ('seed-lens-tint-none', 'lens_tint', 'Clear (no tint)', 10, datetime('now'), datetime('now')),
  ('seed-lens-tint-photogray', 'lens_tint', 'Photogray', 20, datetime('now'), datetime('now')),
  ('seed-lens-tint-photobrown', 'lens_tint', 'Photobrown', 30, datetime('now'), datetime('now')),
  ('seed-lens-tint-grey-solid', 'lens_tint', 'Solid grey', 40, datetime('now'), datetime('now')),
  ('seed-lens-tint-brown-solid', 'lens_tint', 'Solid brown', 50, datetime('now'), datetime('now')),
  ('seed-lens-tint-grey-gradient', 'lens_tint', 'Gradient grey', 60, datetime('now'), datetime('now')),
  ('seed-lens-tint-brown-gradient', 'lens_tint', 'Gradient brown', 70, datetime('now'), datetime('now')),

  ('seed-lens-treatment-photochromic', 'lens_treatment', 'Photochromic', 10, datetime('now'), datetime('now')),
  ('seed-lens-treatment-polarised', 'lens_treatment', 'Polarised', 20, datetime('now'), datetime('now')),
  ('seed-lens-treatment-mirror', 'lens_treatment', 'Mirror finish', 30, datetime('now'), datetime('now')),
  ('seed-lens-treatment-prism', 'lens_treatment', 'Prism ground in', 40, datetime('now'), datetime('now')),
  ('seed-lens-treatment-edge-polish', 'lens_treatment', 'Edge polish', 50, datetime('now'), datetime('now'));

-- The exam catalog starts empty on purpose, for the reason migration 015 left
-- the prescription catalog empty: inventing a list of laboratory tests without
-- clinic confirmation would put clinical content nobody approved into requests
-- that leave the building.
