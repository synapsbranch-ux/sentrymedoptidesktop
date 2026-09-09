-- Finance could only record money going out. Every payment the clinic took was
-- visible as a payment against an invoice, but there was no income ledger to
-- read revenue from, and no way at all to record money that arrives outside the
-- till — a bank transfer, an insurer's remittance, a grant.
CREATE TABLE income_entries (
    id TEXT PRIMARY KEY,
    -- Where the money came from. Anything but 'manual' is generated from a
    -- record that already exists, so the ledger cannot drift from the till.
    source TEXT NOT NULL CHECK (source IN ('pos_payment', 'refund', 'insurance_payment', 'manual')),
    category TEXT NOT NULL,
    description TEXT NOT NULL,
    -- A refund is negative income rather than a second table to reconcile, so a
    -- period's revenue is one SUM.
    amount_minor INTEGER NOT NULL CHECK (amount_minor <> 0),
    currency TEXT NOT NULL,
    exchange_rate TEXT NOT NULL DEFAULT '1',
    received_on TEXT NOT NULL,
    payment_method_id TEXT REFERENCES payment_methods(id),
    payer TEXT,
    payment_id TEXT REFERENCES payments(id),
    refund_id TEXT REFERENCES refunds(id),
    claim_payment_id TEXT REFERENCES insurance_claim_payments(id),
    invoice_id TEXT REFERENCES invoices(id),
    patient_id TEXT REFERENCES patients(id),
    notes TEXT,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_income_date ON income_entries(received_on, source);
-- One income row per payment, per refund, per insurer remittance: the unique
-- indexes are what stop a re-run or a second trigger double-counting revenue.
CREATE UNIQUE INDEX idx_income_payment ON income_entries(payment_id) WHERE payment_id IS NOT NULL;
CREATE UNIQUE INDEX idx_income_refund ON income_entries(refund_id) WHERE refund_id IS NOT NULL;
CREATE UNIQUE INDEX idx_income_claim_payment ON income_entries(claim_payment_id) WHERE claim_payment_id IS NOT NULL;

-- Generated in the database rather than in a handler, so a payment recorded by
-- any path — POS, an invoice payment, a future one — lands in the ledger.
CREATE TRIGGER income_from_payment AFTER INSERT ON payments BEGIN
    INSERT INTO income_entries(id, source, category, description, amount_minor, currency, exchange_rate, received_on, payment_method_id, payment_id, invoice_id, patient_id, created_at, created_by)
    SELECT lower(hex(randomblob(16))), 'pos_payment', 'Clinic sales',
           'Payment ' || NEW.receipt_number || ' on invoice ' || i.invoice_number,
           NEW.amount_minor, NEW.currency, NEW.exchange_rate, substr(NEW.received_at, 1, 10),
           NEW.payment_method_id, NEW.id, NEW.invoice_id, i.patient_id, NEW.created_at, NEW.created_by
    FROM invoices i WHERE i.id = NEW.invoice_id;
END;

CREATE TRIGGER income_from_refund AFTER INSERT ON refunds BEGIN
    INSERT INTO income_entries(id, source, category, description, amount_minor, currency, exchange_rate, received_on, payment_method_id, refund_id, invoice_id, patient_id, notes, created_at, created_by)
    SELECT lower(hex(randomblob(16))), 'refund', 'Clinic sales',
           'Refund of payment ' || p.receipt_number,
           -NEW.amount_minor, p.currency, p.exchange_rate, substr(NEW.refunded_at, 1, 10),
           p.payment_method_id, NEW.id, p.invoice_id, i.patient_id, NEW.reason, NEW.refunded_at, NEW.created_by
    FROM payments p JOIN invoices i ON i.id = p.invoice_id WHERE p.id = NEW.payment_id;
END;

CREATE TRIGGER income_from_insurer_payment AFTER INSERT ON insurance_claim_payments BEGIN
    INSERT INTO income_entries(id, source, category, description, amount_minor, currency, exchange_rate, received_on, payer, claim_payment_id, invoice_id, patient_id, notes, created_at, created_by)
    SELECT lower(hex(randomblob(16))), 'insurance_payment', 'Insurance remittances',
           'Remittance from ' || py.name,
           NEW.amount_minor, c.currency, c.exchange_rate, NEW.payment_date,
           py.name, NEW.id, c.invoice_id, c.patient_id, NEW.notes, NEW.created_at, NEW.created_by
    FROM insurance_claims c JOIN payers py ON py.id = c.payer_id WHERE c.id = NEW.claim_id;
END;

-- Backfill from the records that already exist, so the ledger opens with the
-- clinic's real history rather than from today.
INSERT INTO income_entries(id, source, category, description, amount_minor, currency, exchange_rate, received_on, payment_method_id, payment_id, invoice_id, patient_id, created_at, created_by)
SELECT lower(hex(randomblob(16))), 'pos_payment', 'Clinic sales',
       'Payment ' || p.receipt_number || ' on invoice ' || i.invoice_number,
       p.amount_minor, p.currency, p.exchange_rate, substr(p.received_at, 1, 10),
       p.payment_method_id, p.id, p.invoice_id, i.patient_id, p.created_at, p.created_by
FROM payments p JOIN invoices i ON i.id = p.invoice_id;

INSERT INTO income_entries(id, source, category, description, amount_minor, currency, exchange_rate, received_on, payment_method_id, refund_id, invoice_id, patient_id, notes, created_at, created_by)
SELECT lower(hex(randomblob(16))), 'refund', 'Clinic sales', 'Refund of payment ' || p.receipt_number,
       -r.amount_minor, p.currency, p.exchange_rate, substr(r.refunded_at, 1, 10),
       p.payment_method_id, r.id, p.invoice_id, i.patient_id, r.reason, r.refunded_at, r.created_by
FROM refunds r JOIN payments p ON p.id = r.payment_id JOIN invoices i ON i.id = p.invoice_id;

INSERT INTO income_entries(id, source, category, description, amount_minor, currency, exchange_rate, received_on, payer, claim_payment_id, invoice_id, patient_id, notes, created_at, created_by)
SELECT lower(hex(randomblob(16))), 'insurance_payment', 'Insurance remittances', 'Remittance from ' || py.name,
       cp.amount_minor, c.currency, c.exchange_rate, cp.payment_date,
       py.name, cp.id, c.invoice_id, c.patient_id, cp.notes, cp.created_at, cp.created_by
FROM insurance_claim_payments cp JOIN insurance_claims c ON c.id = cp.claim_id JOIN payers py ON py.id = c.payer_id;

-- A quote is what the clinic offers before there is anything to owe. It becomes
-- an invoice only when the patient accepts, and the conversion is recorded on
-- both sides so a quote can never be billed twice.
CREATE TABLE quotes (
    id TEXT PRIMARY KEY,
    quote_number TEXT NOT NULL UNIQUE,
    patient_id TEXT REFERENCES patients(id),
    -- A quote may be for somebody who is not a patient yet.
    customer_name TEXT,
    status TEXT NOT NULL CHECK (status IN ('draft', 'sent', 'accepted', 'declined', 'expired', 'converted')),
    currency TEXT NOT NULL,
    exchange_rate TEXT NOT NULL DEFAULT '1',
    subtotal_minor INTEGER NOT NULL DEFAULT 0,
    discount_minor INTEGER NOT NULL DEFAULT 0,
    tax_minor INTEGER NOT NULL DEFAULT 0,
    total_minor INTEGER NOT NULL DEFAULT 0,
    valid_until TEXT,
    notes TEXT,
    converted_invoice_id TEXT REFERENCES invoices(id),
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_quotes_status ON quotes(status, created_at DESC);
CREATE INDEX idx_quotes_patient ON quotes(patient_id, created_at DESC);
CREATE UNIQUE INDEX idx_quotes_converted ON quotes(converted_invoice_id) WHERE converted_invoice_id IS NOT NULL;

CREATE TABLE quote_items (
    id TEXT PRIMARY KEY,
    quote_id TEXT NOT NULL REFERENCES quotes(id),
    inventory_item_id TEXT REFERENCES inventory_items(id),
    description TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_minor INTEGER NOT NULL,
    discount_minor INTEGER NOT NULL DEFAULT 0,
    tax_minor INTEGER NOT NULL DEFAULT 0,
    line_total_minor INTEGER NOT NULL,
    procedure_code TEXT
);
CREATE INDEX idx_quote_items_quote ON quote_items(quote_id);
