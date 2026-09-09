-- Staff administration had no home at all: contracts, hours and pay lived
-- outside the system entirely. These tables follow the same shape as the rest of
-- the application — an entity, its versioned record, and generated documents.

-- What a person is employed as. A position holds the default pay so a new hire
-- does not need it retyped, but every employee keeps their own agreed figure.
CREATE TABLE hr_positions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    default_salary_minor INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'HTG',
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id)
);
CREATE UNIQUE INDEX idx_hr_positions_title ON hr_positions(lower(title));

CREATE TABLE employees (
    id TEXT PRIMARY KEY,
    employee_number TEXT NOT NULL UNIQUE,
    -- An employee may also have a login, but most will not: a cleaner or a
    -- security guard is staff without ever being a system user.
    user_id TEXT REFERENCES users(id),
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    position_id TEXT REFERENCES hr_positions(id),
    employment_type TEXT NOT NULL DEFAULT 'employee' CHECK (employment_type IN ('employee', 'contractor')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'on_leave', 'suspended', 'terminated')),
    started_on TEXT,
    ended_on TEXT,
    -- Salaried staff are paid their base figure per period; hourly staff are
    -- paid for the hours attendance actually records.
    pay_basis TEXT NOT NULL DEFAULT 'monthly' CHECK (pay_basis IN ('monthly', 'biweekly', 'weekly', 'hourly')),
    base_salary_minor INTEGER NOT NULL DEFAULT 0,
    hourly_rate_minor INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'HTG',
    -- Named deductions applied to every payslip: [{"name":"…","type":"fixed|percent","value":n}]
    deductions_json TEXT NOT NULL DEFAULT '[]',
    phone TEXT,
    email TEXT,
    address TEXT,
    national_id TEXT,
    bank_account TEXT,
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_employees_status ON employees(status, archived_at, last_name, first_name);
CREATE INDEX idx_employees_position ON employees(position_id);

-- Hours, however they are captured: clocked at the door or entered afterwards
-- by whoever keeps the book. Leave and absence are recorded here too, so a
-- period's payroll can see them without a second source.
CREATE TABLE attendance_entries (
    id TEXT PRIMARY KEY,
    employee_id TEXT NOT NULL REFERENCES employees(id),
    work_date TEXT NOT NULL,
    clock_in TEXT,
    clock_out TEXT,
    minutes_worked INTEGER NOT NULL DEFAULT 0 CHECK (minutes_worked >= 0),
    entry_type TEXT NOT NULL DEFAULT 'manual' CHECK (entry_type IN ('clocked', 'manual', 'leave', 'absent', 'holiday')),
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
-- One open shift per person per day: a second clock-in without a clock-out
-- would otherwise silently double a day's hours.
CREATE UNIQUE INDEX idx_attendance_open_shift ON attendance_entries(employee_id, work_date) WHERE clock_out IS NULL AND entry_type = 'clocked';
CREATE INDEX idx_attendance_period ON attendance_entries(employee_id, work_date);

CREATE TABLE payroll_runs (
    id TEXT PRIMARY KEY,
    run_number TEXT NOT NULL UNIQUE,
    period_start TEXT NOT NULL,
    period_end TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved', 'paid', 'cancelled')),
    currency TEXT NOT NULL DEFAULT 'HTG',
    gross_minor INTEGER NOT NULL DEFAULT 0,
    deductions_minor INTEGER NOT NULL DEFAULT 0,
    net_minor INTEGER NOT NULL DEFAULT 0,
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    approved_at TEXT,
    approved_by TEXT REFERENCES users(id),
    paid_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_payroll_runs_period ON payroll_runs(period_start DESC, status);

-- A payslip is the computed record for one person in one run. The breakdown is
-- kept as it was computed, so a slip re-read next year still shows the figures
-- the employee was actually paid on, whatever has changed since.
CREATE TABLE payslips (
    id TEXT PRIMARY KEY,
    payroll_run_id TEXT NOT NULL REFERENCES payroll_runs(id),
    employee_id TEXT NOT NULL REFERENCES employees(id),
    gross_minor INTEGER NOT NULL DEFAULT 0,
    deductions_minor INTEGER NOT NULL DEFAULT 0,
    net_minor INTEGER NOT NULL DEFAULT 0,
    minutes_worked INTEGER NOT NULL DEFAULT 0,
    days_present INTEGER NOT NULL DEFAULT 0,
    breakdown_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_payslips_run_employee ON payslips(payroll_run_id, employee_id);

-- Contracts, letters and anything else the clinic hands an employee. The body
-- is stored as generated, so a document already given to someone never changes
-- underneath them when a template is edited.
CREATE TABLE hr_documents (
    id TEXT PRIMARY KEY,
    employee_id TEXT NOT NULL REFERENCES employees(id),
    kind TEXT NOT NULL CHECK (kind IN ('contract', 'letter', 'certificate', 'other')),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    generated_at TEXT NOT NULL,
    generated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_hr_documents_employee ON hr_documents(employee_id, generated_at DESC);

-- Editable templates, filled from the employee's own record. Seeded with a
-- plain employment contract so the module is usable on day one; a clinic edits
-- the wording to whatever its jurisdiction and lawyer require.
CREATE TABLE hr_document_templates (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('contract', 'letter', 'certificate', 'other')),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id)
);

INSERT INTO hr_document_templates(id, kind, title, body, created_at, updated_at) VALUES (
  'seed-template-contract', 'contract', 'Employment contract',
  'EMPLOYMENT CONTRACT

Between {{clinicName}}, {{clinicAddress}} (the employer)
and {{employeeName}}, {{employeeAddress}} (the employee).

1. Position. The employee is engaged as {{position}}, on a {{employmentType}} basis.
2. Start date. Employment begins on {{startedOn}}.
3. Remuneration. The employee is paid {{payDescription}}, in {{currency}}.
4. Hours. Working hours are as agreed between the parties and recorded in the clinic''''s attendance register.
5. Termination. Either party may end this contract in accordance with the law applicable at {{clinicAddress}}.

This template is a starting point and is not legal advice. Have it reviewed
against the employment law that applies to this clinic before it is signed.

Signed at {{clinicName}} on {{today}}.

_______________________          _______________________
Employer                          {{employeeName}}',
  datetime('now'), datetime('now')
);
