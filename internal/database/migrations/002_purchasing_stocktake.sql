CREATE TABLE purchase_order_receipts (
    id TEXT PRIMARY KEY,
    purchase_order_id TEXT NOT NULL REFERENCES purchase_orders(id),
    notes TEXT,
    received_at TEXT NOT NULL,
    received_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_purchase_receipts_order ON purchase_order_receipts(purchase_order_id, received_at DESC);

CREATE TABLE purchase_order_receipt_items (
    id TEXT PRIMARY KEY,
    receipt_id TEXT NOT NULL REFERENCES purchase_order_receipts(id) ON DELETE CASCADE,
    purchase_order_item_id TEXT NOT NULL REFERENCES purchase_order_items(id),
    quantity_received INTEGER NOT NULL CHECK (quantity_received > 0)
);

CREATE TABLE stock_take_sessions (
    id TEXT PRIMARY KEY,
    stock_take_number TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress','completed','cancelled')),
    category TEXT,
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id),
    completed_by TEXT REFERENCES users(id)
);
CREATE INDEX idx_stock_take_status ON stock_take_sessions(status, started_at DESC);

CREATE TABLE stock_take_items (
    id TEXT PRIMARY KEY,
    stock_take_id TEXT NOT NULL REFERENCES stock_take_sessions(id) ON DELETE CASCADE,
    inventory_item_id TEXT NOT NULL REFERENCES inventory_items(id),
    expected_quantity INTEGER NOT NULL,
    counted_quantity INTEGER,
    reason TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    counted_at TEXT,
    counted_by TEXT REFERENCES users(id),
    UNIQUE(stock_take_id, inventory_item_id)
);
CREATE INDEX idx_stock_take_items_session ON stock_take_items(stock_take_id, inventory_item_id);
