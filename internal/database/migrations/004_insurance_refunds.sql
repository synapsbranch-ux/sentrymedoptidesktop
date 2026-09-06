CREATE TABLE insurance_claim_payments (
    id TEXT PRIMARY KEY,
    claim_id TEXT NOT NULL REFERENCES insurance_claims(id),
    amount_minor INTEGER NOT NULL CHECK(amount_minor > 0),
    payment_date TEXT NOT NULL,
    reference TEXT,
    notes TEXT,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_claim_payments_claim ON insurance_claim_payments(claim_id, payment_date);
CREATE INDEX idx_claims_status_updated ON insurance_claims(status, updated_at DESC);
CREATE INDEX idx_claims_patient ON insurance_claims(patient_id, created_at DESC);
ALTER TABLE insurance_claims ADD COLUMN member_number TEXT;
ALTER TABLE insurance_claims ADD COLUMN policy_number TEXT;
ALTER TABLE insurance_claims ADD COLUMN currency TEXT NOT NULL DEFAULT 'HTG';
ALTER TABLE insurance_claims ADD COLUMN exchange_rate TEXT NOT NULL DEFAULT '1';

CREATE TABLE credit_notes (
    id TEXT PRIMARY KEY,
    credit_number TEXT NOT NULL UNIQUE,
    invoice_id TEXT NOT NULL REFERENCES invoices(id),
    refund_id TEXT NOT NULL REFERENCES refunds(id),
    amount_minor INTEGER NOT NULL CHECK(amount_minor > 0),
    reason TEXT NOT NULL,
    restocked INTEGER NOT NULL DEFAULT 0 CHECK(restocked IN (0,1)),
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_credit_notes_invoice ON credit_notes(invoice_id, created_at DESC);
