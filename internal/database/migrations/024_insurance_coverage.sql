-- A policy says how much of a bill the insurer takes. Until now that percentage
-- lived only in whoever's head was typing the split, so the patient and payer
-- portions on every claim were arithmetic done by hand and never checked.
ALTER TABLE patient_insurance ADD COLUMN coverage_percent REAL NOT NULL DEFAULT 0
    CHECK (coverage_percent >= 0 AND coverage_percent <= 100);

-- What the insurer covers by default, so a policy added without its own figure
-- still proposes a split rather than nothing.
ALTER TABLE payers ADD COLUMN default_coverage_percent REAL NOT NULL DEFAULT 0
    CHECK (default_coverage_percent >= 0 AND default_coverage_percent <= 100);

-- A claim can be for one line of an invoice — a single procedure a policy
-- covers where the rest of the sale is the patient's own — rather than only for
-- a whole invoice.
ALTER TABLE insurance_claims ADD COLUMN invoice_item_id TEXT REFERENCES invoice_items(id);
CREATE INDEX idx_claims_invoice_item ON insurance_claims(invoice_item_id);
