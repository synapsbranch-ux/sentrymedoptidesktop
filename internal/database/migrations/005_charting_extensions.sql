CREATE TABLE clinical_macros (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'general',
    body TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_macros_category ON clinical_macros(category, label);

CREATE TABLE eye_diagrams (
    id TEXT PRIMARY KEY,
    encounter_id TEXT NOT NULL REFERENCES encounters(id),
    eye TEXT NOT NULL CHECK (eye IN ('OD','OS')),
    annotations_json TEXT NOT NULL DEFAULT '[]',
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id),
    UNIQUE(encounter_id, eye)
);

ALTER TABLE inventory_items ADD COLUMN procedure_code TEXT;
ALTER TABLE invoice_items ADD COLUMN procedure_code TEXT;
ALTER TABLE invoices ADD COLUMN encounter_id TEXT REFERENCES encounters(id);

ALTER TABLE queue_entries ADD COLUMN source TEXT NOT NULL DEFAULT 'staff' CHECK (source IN ('staff','kiosk'));

CREATE TABLE kiosk_lookup_attempts (
    id TEXT PRIMARY KEY,
    ip_address TEXT NOT NULL,
    attempted_at TEXT NOT NULL
);
CREATE INDEX idx_kiosk_attempts_ip ON kiosk_lookup_attempts(ip_address, attempted_at DESC);
