-- The inventory catalogue could record a name and a price but not the unit
-- it is sold in, a batch/lot for anything perishable, or an expiration date —
-- and the stock ledger had no way to record something expiring on the shelf,
-- only being sold, adjusted, damaged or lost. This migration adds both,
-- additively: no existing column, CHECK constraint or row is rewritten
-- except the one safe stock_movements rebuild described below.

ALTER TABLE inventory_items ADD COLUMN unit TEXT NOT NULL DEFAULT 'unit';
ALTER TABLE inventory_items ADD COLUMN batch_number TEXT;
ALTER TABLE inventory_items ADD COLUMN expiration_date TEXT;
ALTER TABLE inventory_items ADD COLUMN notes TEXT;
CREATE INDEX idx_inventory_items_expiration ON inventory_items(expiration_date) WHERE expiration_date IS NOT NULL;

-- stock_movements.movement_type is CHECK-constrained and nothing references
-- this table by foreign key (it only references inventory_items), so unlike
-- insurance_claims it can be safely rebuilt to add a value the CHECK does
-- not yet allow — an expired item is neither a sale, a return, damage nor a
-- loss, and lumping it into "adjustment" lost exactly the distinction this
-- redesign needs for an "expired stock" report.
CREATE TABLE stock_movements_new (
    id TEXT PRIMARY KEY,
    item_id TEXT NOT NULL REFERENCES inventory_items(id),
    movement_type TEXT NOT NULL CHECK (movement_type IN ('purchase','sale','return','adjustment','damage','loss','transfer','correction','expired')),
    previous_quantity INTEGER NOT NULL,
    quantity_change INTEGER NOT NULL,
    resulting_quantity INTEGER NOT NULL,
    reason TEXT NOT NULL,
    reference_type TEXT,
    reference_id TEXT,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
INSERT INTO stock_movements_new SELECT * FROM stock_movements;
DROP TABLE stock_movements;
ALTER TABLE stock_movements_new RENAME TO stock_movements;
CREATE INDEX idx_movements_item ON stock_movements(item_id, created_at DESC);
