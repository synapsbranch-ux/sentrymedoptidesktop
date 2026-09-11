CREATE TABLE print_jobs (
    id TEXT PRIMARY KEY,
    invoice_id TEXT REFERENCES invoices(id),
    receipt_id TEXT NOT NULL,
    printer_id TEXT NOT NULL,
    printer_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('printed','failed')),
    error_code TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_print_jobs_created ON print_jobs(created_at DESC);
CREATE INDEX idx_print_jobs_invoice ON print_jobs(invoice_id,created_at DESC);
