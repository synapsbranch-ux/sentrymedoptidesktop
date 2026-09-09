-- A clinic's appointment and consultation types are things it sells, so they
-- stop being labels on one screen and prices retyped on another: a service type
-- is an inventory item of category 'service' that also carries the duration
-- scheduling needs and a flag saying it can be booked.
ALTER TABLE inventory_items ADD COLUMN duration_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE inventory_items ADD COLUMN bookable INTEGER NOT NULL DEFAULT 0 CHECK (bookable IN (0, 1));
CREATE INDEX idx_inventory_bookable ON inventory_items(bookable, archived_at, name);

-- Payment is not a prerequisite for an appointment or a consultation. When one
-- does happen, the sale records what it was for, so revenue is traceable back to
-- the visit without the visit ever depending on the till.
ALTER TABLE invoices ADD COLUMN appointment_id TEXT REFERENCES appointments(id);
CREATE INDEX idx_invoices_appointment ON invoices(appointment_id);

-- An appointment booked from a service type keeps the link, so the counter can
-- see what was agreed and price it without guessing.
ALTER TABLE appointments ADD COLUMN service_item_id TEXT REFERENCES inventory_items(id);

-- A sale that cannot be finished now is parked whole and resumed later, rather
-- than being abandoned and rebuilt from memory.
CREATE TABLE parked_sales (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    patient_id TEXT REFERENCES patients(id),
    currency TEXT NOT NULL,
    -- The cart exactly as the cashier left it, including per-line discounts.
    cart_json TEXT NOT NULL DEFAULT '[]',
    note TEXT,
    total_minor INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    -- A resumed sale is kept, not deleted, so the till has a record of a cart
    -- that was set aside and by whom.
    resumed_at TEXT,
    resumed_by TEXT REFERENCES users(id)
);
CREATE INDEX idx_parked_sales_open ON parked_sales(resumed_at, created_at DESC);

-- Seed the four visit types the clinic already had as unpriced, bookable
-- services, so scheduling keeps working unchanged and the clinic only has to
-- fill in a price. A zero price is a service the clinic has not priced yet, not
-- a free one, and the POS says so.
INSERT INTO inventory_items(id, sku, category, name, attributes_json, cost_minor, sale_price_minor, currency, quantity, reorder_level, track_stock, duration_minutes, bookable, version, created_at, updated_at, updated_by)
SELECT
  'seed-service-' || replace(lower(label), ' ', '-'),
  'SVC-' || upper(substr(replace(label, ' ', ''), 1, 8)),
  'service', label, '{}', 0, 0,
  COALESCE((SELECT json_extract(value_json, '$.currency') FROM settings WHERE key = 'clinic'), 'HTG'),
  0, 0, 0, 30, 1, 1, datetime('now'), datetime('now'),
  (SELECT id FROM users WHERE role = 'doctor' AND archived_at IS NULL ORDER BY created_at LIMIT 1)
FROM catalog_entries
WHERE catalog = 'appointment_reason' AND active = 1
  AND EXISTS (SELECT 1 FROM users WHERE role = 'doctor' AND archived_at IS NULL)
  AND NOT EXISTS (SELECT 1 FROM inventory_items i WHERE lower(i.name) = lower(catalog_entries.label) AND i.category = 'service');
