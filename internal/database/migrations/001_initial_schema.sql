CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL COLLATE NOCASE UNIQUE,
    email TEXT COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('doctor', 'nurse')),
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    last_login_at TEXT,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id)
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL UNIQUE,
    ip_address TEXT,
    user_agent TEXT,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    invalidated_at TEXT
);
CREATE INDEX idx_sessions_token ON sessions(token_hash);
CREATE INDEX idx_sessions_expiry ON sessions(expires_at);

CREATE TABLE login_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    successful INTEGER NOT NULL CHECK (successful IN (0, 1)),
    attempted_at TEXT NOT NULL
);
CREATE INDEX idx_login_attempts_rate ON login_attempts(ip_address, attempted_at);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id)
);

CREATE TABLE sequences (
    name TEXT NOT NULL,
    year INTEGER NOT NULL DEFAULT 0,
    value INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (name, year)
);

CREATE TABLE audit_logs (
    id TEXT PRIMARY KEY,
    user_id TEXT REFERENCES users(id),
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT,
    summary TEXT NOT NULL,
    before_json TEXT,
    after_json TEXT,
    ip_address TEXT,
    user_agent TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_audit_created ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_entity ON audit_logs(entity_type, entity_id, created_at DESC);

CREATE TABLE patients (
    id TEXT PRIMARY KEY,
    medical_record_number TEXT NOT NULL UNIQUE,
    first_name TEXT NOT NULL,
    middle_name TEXT,
    last_name TEXT NOT NULL,
    preferred_name TEXT,
    sex TEXT,
    date_of_birth TEXT,
    phone TEXT,
    alternate_phone TEXT,
    email TEXT,
    address TEXT,
    city TEXT,
    occupation TEXT,
    employer TEXT,
    preferred_language TEXT,
    communication_preference TEXT,
    referral_source TEXT,
    referring_provider TEXT,
    notes TEXT,
    tags_json TEXT NOT NULL DEFAULT '[]',
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_patients_name ON patients(last_name COLLATE NOCASE, first_name COLLATE NOCASE);
CREATE INDEX idx_patients_phone ON patients(phone);
CREATE INDEX idx_patients_created ON patients(created_at DESC);

CREATE TABLE patient_emergency_contacts (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    name TEXT NOT NULL,
    relationship TEXT,
    phone TEXT NOT NULL,
    is_primary INTEGER NOT NULL DEFAULT 1 CHECK (is_primary IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE patient_insurance (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    payer_id TEXT,
    payer_name TEXT NOT NULL,
    member_number TEXT,
    policy_number TEXT,
    authorization TEXT,
    is_primary INTEGER NOT NULL DEFAULT 1 CHECK (is_primary IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE patient_histories (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL UNIQUE REFERENCES patients(id),
    chronic_diseases_json TEXT NOT NULL DEFAULT '[]',
    diabetes INTEGER,
    hypertension INTEGER,
    cardiovascular_notes TEXT,
    neurological_notes TEXT,
    surgeries TEXT,
    pregnancy_notes TEXT,
    tobacco_use TEXT,
    family_medical_history TEXT,
    family_ocular_history TEXT,
    previous_eye_surgery TEXT,
    ocular_trauma TEXT,
    glaucoma_history TEXT,
    cataract_history TEXT,
    retinal_disease TEXT,
    previous_glasses TEXT,
    previous_contact_lenses TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE patient_allergies (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    substance TEXT NOT NULL,
    reaction TEXT,
    severity TEXT,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE patient_medications (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    medication TEXT NOT NULL,
    strength TEXT,
    dosage TEXT,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE appointments (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    practitioner_id TEXT REFERENCES users(id),
    starts_at TEXT NOT NULL,
    duration_minutes INTEGER NOT NULL DEFAULT 30 CHECK (duration_minutes > 0),
    type TEXT NOT NULL,
    reason TEXT,
    notes TEXT,
    status TEXT NOT NULL CHECK (status IN ('scheduled','confirmed','checked_in','waiting','in_consultation','completed','cancelled','no_show')),
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_appointments_start ON appointments(starts_at);
CREATE INDEX idx_appointments_status ON appointments(status, starts_at);
CREATE INDEX idx_appointments_patient ON appointments(patient_id, starts_at DESC);

CREATE TABLE queue_entries (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    appointment_id TEXT REFERENCES appointments(id),
    encounter_id TEXT,
    assigned_doctor_id TEXT REFERENCES users(id),
    arrived_at TEXT NOT NULL,
    stage TEXT NOT NULL CHECK (stage IN ('checked_in','waiting_nurse','pre_test','waiting_doctor','in_consultation','checkout','completed')),
    priority INTEGER NOT NULL DEFAULT 0,
    completed_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_queue_active ON queue_entries(completed_at, priority DESC, arrived_at);

CREATE TABLE encounters (
    id TEXT PRIMARY KEY,
    encounter_number TEXT NOT NULL UNIQUE,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    appointment_id TEXT REFERENCES appointments(id),
    doctor_id TEXT REFERENCES users(id),
    visit_reason TEXT,
    chief_complaint TEXT,
    hpi TEXT,
    assessment TEXT,
    treatment_plan TEXT,
    follow_up TEXT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','finalized')),
    finalized_at TEXT,
    finalized_by TEXT REFERENCES users(id),
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_encounters_patient ON encounters(patient_id, created_at DESC);
CREATE INDEX idx_encounters_status ON encounters(status, created_at DESC);

CREATE TABLE pretests (
    id TEXT PRIMARY KEY,
    encounter_id TEXT NOT NULL UNIQUE REFERENCES encounters(id),
    chief_complaint TEXT,
    vitals_json TEXT NOT NULL DEFAULT '{}',
    visual_acuity_json TEXT NOT NULL DEFAULT '{}',
    autorefraction_json TEXT NOT NULL DEFAULT '{}',
    keratometry_json TEXT NOT NULL DEFAULT '{}',
    iop_json TEXT NOT NULL DEFAULT '{}',
    pupils TEXT,
    eom TEXT,
    cover_test TEXT,
    confrontation_fields TEXT,
    color_vision TEXT,
    stereopsis TEXT,
    pachymetry_json TEXT NOT NULL DEFAULT '{}',
    lensometry_json TEXT NOT NULL DEFAULT '{}',
    completed_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE encounter_sections (
    id TEXT PRIMARY KEY,
    encounter_id TEXT NOT NULL REFERENCES encounters(id),
    section_type TEXT NOT NULL CHECK (section_type IN ('history_review','current_correction','objective_refraction','subjective_refraction','cycloplegic_refraction','anterior_segment','posterior_segment','other_exam')),
    data_json TEXT NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id),
    UNIQUE(encounter_id, section_type)
);

CREATE TABLE diagnoses (
    id TEXT PRIMARY KEY,
    encounter_id TEXT NOT NULL REFERENCES encounters(id),
    diagnosis TEXT NOT NULL,
    code TEXT,
    laterality TEXT,
    notes TEXT,
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE encounter_addenda (
    id TEXT PRIMARY KEY,
    encounter_id TEXT NOT NULL REFERENCES encounters(id),
    body TEXT NOT NULL,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE prescriptions (
    id TEXT PRIMARY KEY,
    prescription_number TEXT NOT NULL UNIQUE,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    encounter_id TEXT REFERENCES encounters(id),
    doctor_id TEXT NOT NULL REFERENCES users(id),
    type TEXT NOT NULL CHECK (type IN ('spectacle','contact_lens','medication')),
    od_json TEXT NOT NULL DEFAULT '{}',
    os_json TEXT NOT NULL DEFAULT '{}',
    details_json TEXT NOT NULL DEFAULT '{}',
    notes TEXT,
    issued_at TEXT NOT NULL,
    expires_at TEXT,
    status TEXT NOT NULL DEFAULT 'final' CHECK (status IN ('draft','final','cancelled')),
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_prescriptions_patient ON prescriptions(patient_id, issued_at DESC);

CREATE TABLE documents (
    id TEXT PRIMARY KEY,
    patient_id TEXT REFERENCES patients(id),
    encounter_id TEXT REFERENCES encounters(id),
    category TEXT NOT NULL,
    display_name TEXT NOT NULL,
    storage_name TEXT NOT NULL UNIQUE,
    media_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    checksum_sha256 TEXT NOT NULL,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_documents_patient ON documents(patient_id, created_at DESC);

CREATE TABLE suppliers (
    id TEXT PRIMARY KEY,
    company TEXT NOT NULL,
    contact_person TEXT,
    phone TEXT,
    email TEXT,
    address TEXT,
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE inventory_items (
    id TEXT PRIMARY KEY,
    sku TEXT NOT NULL COLLATE NOCASE UNIQUE,
    barcode TEXT COLLATE NOCASE UNIQUE,
    category TEXT NOT NULL CHECK (category IN ('frame','ophthalmic_lens','contact_lens','accessory','service')),
    name TEXT NOT NULL,
    brand TEXT,
    model TEXT,
    attributes_json TEXT NOT NULL DEFAULT '{}',
    supplier_id TEXT REFERENCES suppliers(id),
    cost_minor INTEGER NOT NULL DEFAULT 0,
    sale_price_minor INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'HTG',
    quantity INTEGER NOT NULL DEFAULT 0,
    reorder_level INTEGER NOT NULL DEFAULT 0,
    track_stock INTEGER NOT NULL DEFAULT 1 CHECK (track_stock IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_inventory_name ON inventory_items(name COLLATE NOCASE);
CREATE INDEX idx_inventory_stock ON inventory_items(quantity, reorder_level);

CREATE TABLE stock_movements (
    id TEXT PRIMARY KEY,
    item_id TEXT NOT NULL REFERENCES inventory_items(id),
    movement_type TEXT NOT NULL CHECK (movement_type IN ('purchase','sale','return','adjustment','damage','loss','transfer','correction')),
    previous_quantity INTEGER NOT NULL,
    quantity_change INTEGER NOT NULL,
    resulting_quantity INTEGER NOT NULL,
    reason TEXT NOT NULL,
    reference_type TEXT,
    reference_id TEXT,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_movements_item ON stock_movements(item_id, created_at DESC);

CREATE TABLE purchase_orders (
    id TEXT PRIMARY KEY,
    order_number TEXT NOT NULL UNIQUE,
    supplier_id TEXT NOT NULL REFERENCES suppliers(id),
    status TEXT NOT NULL CHECK (status IN ('draft','sent','partial','received','cancelled')),
    currency TEXT NOT NULL,
    expected_at TEXT,
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE purchase_order_items (
    id TEXT PRIMARY KEY,
    purchase_order_id TEXT NOT NULL REFERENCES purchase_orders(id),
    inventory_item_id TEXT NOT NULL REFERENCES inventory_items(id),
    quantity_ordered INTEGER NOT NULL CHECK (quantity_ordered > 0),
    quantity_received INTEGER NOT NULL DEFAULT 0,
    unit_cost_minor INTEGER NOT NULL
);

CREATE TABLE invoices (
    id TEXT PRIMARY KEY,
    invoice_number TEXT NOT NULL UNIQUE,
    patient_id TEXT REFERENCES patients(id),
    status TEXT NOT NULL CHECK (status IN ('draft','issued','partially_paid','paid','overdue','cancelled','refunded')),
    currency TEXT NOT NULL,
    exchange_rate TEXT NOT NULL DEFAULT '1',
    subtotal_minor INTEGER NOT NULL,
    discount_minor INTEGER NOT NULL DEFAULT 0,
    tax_minor INTEGER NOT NULL DEFAULT 0,
    total_minor INTEGER NOT NULL,
    due_at TEXT,
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_invoices_status ON invoices(status, created_at DESC);
CREATE INDEX idx_invoices_patient ON invoices(patient_id, created_at DESC);

CREATE TABLE invoice_items (
    id TEXT PRIMARY KEY,
    invoice_id TEXT NOT NULL REFERENCES invoices(id),
    inventory_item_id TEXT REFERENCES inventory_items(id),
    description TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_minor INTEGER NOT NULL,
    discount_minor INTEGER NOT NULL DEFAULT 0,
    tax_minor INTEGER NOT NULL DEFAULT 0,
    line_total_minor INTEGER NOT NULL,
    cost_minor INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE payment_methods (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE cash_register_sessions (
    id TEXT PRIMARY KEY,
    opened_by TEXT NOT NULL REFERENCES users(id),
    closed_by TEXT REFERENCES users(id),
    currency TEXT NOT NULL,
    opening_float_minor INTEGER NOT NULL DEFAULT 0,
    counted_cash_minor INTEGER,
    expected_cash_minor INTEGER,
    difference_minor INTEGER,
    opening_notes TEXT,
    closing_notes TEXT,
    opened_at TEXT NOT NULL,
    closed_at TEXT
);

CREATE TABLE payments (
    id TEXT PRIMARY KEY,
    receipt_number TEXT NOT NULL UNIQUE,
    invoice_id TEXT NOT NULL REFERENCES invoices(id),
    register_session_id TEXT REFERENCES cash_register_sessions(id),
    payment_method_id TEXT NOT NULL REFERENCES payment_methods(id),
    amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL,
    exchange_rate TEXT NOT NULL DEFAULT '1',
    reference TEXT,
    notes TEXT,
    received_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_payments_invoice ON payments(invoice_id, received_at);

CREATE TABLE refunds (
    id TEXT PRIMARY KEY,
    payment_id TEXT NOT NULL REFERENCES payments(id),
    amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
    reason TEXT NOT NULL,
    refunded_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE expenses (
    id TEXT PRIMARY KEY,
    category TEXT NOT NULL,
    description TEXT NOT NULL,
    amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL,
    exchange_rate TEXT NOT NULL DEFAULT '1',
    expense_date TEXT NOT NULL,
    payment_method_id TEXT REFERENCES payment_methods(id),
    vendor TEXT,
    document_id TEXT REFERENCES documents(id),
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_expenses_date ON expenses(expense_date DESC);

CREATE TABLE lab_orders (
    id TEXT PRIMARY KEY,
    order_number TEXT NOT NULL UNIQUE,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    prescription_id TEXT REFERENCES prescriptions(id),
    invoice_id TEXT REFERENCES invoices(id),
    supplier_id TEXT REFERENCES suppliers(id),
    frame_item_id TEXT REFERENCES inventory_items(id),
    lens_item_id TEXT REFERENCES inventory_items(id),
    lens_type TEXT,
    material TEXT,
    coatings_json TEXT NOT NULL DEFAULT '[]',
    tint TEXT,
    treatments_json TEXT NOT NULL DEFAULT '[]',
    measurements_json TEXT NOT NULL DEFAULT '{}',
    notes TEXT,
    ordered_at TEXT,
    expected_at TEXT,
    cost_minor INTEGER NOT NULL DEFAULT 0,
    sale_price_minor INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('draft','ordered','at_lab','received','edging_mounting','quality_control','ready','delivered','cancelled')),
    delivered_at TEXT,
    delivered_by TEXT REFERENCES users(id),
    received_by_name TEXT,
    after_sales_json TEXT NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_lab_status ON lab_orders(status, expected_at);
CREATE INDEX idx_lab_patient ON lab_orders(patient_id, created_at DESC);

CREATE TABLE lab_status_history (
    id TEXT PRIMARY KEY,
    lab_order_id TEXT NOT NULL REFERENCES lab_orders(id),
    from_status TEXT,
    to_status TEXT NOT NULL,
    notes TEXT,
    changed_at TEXT NOT NULL,
    changed_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE lab_quality_control (
    id TEXT PRIMARY KEY,
    lab_order_id TEXT NOT NULL UNIQUE REFERENCES lab_orders(id),
    prescription_verified INTEGER NOT NULL DEFAULT 0,
    power_verified INTEGER NOT NULL DEFAULT 0,
    axis_verified INTEGER NOT NULL DEFAULT 0,
    frame_condition INTEGER NOT NULL DEFAULT 0,
    lens_condition INTEGER NOT NULL DEFAULT 0,
    fitting_verified INTEGER NOT NULL DEFAULT 0,
    final_cleaning INTEGER NOT NULL DEFAULT 0,
    notes TEXT,
    completed_at TEXT NOT NULL,
    completed_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE payers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    contact_name TEXT,
    phone TEXT,
    email TEXT,
    address TEXT,
    active INTEGER NOT NULL DEFAULT 1,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE insurance_claims (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id),
    payer_id TEXT NOT NULL REFERENCES payers(id),
    invoice_id TEXT REFERENCES invoices(id),
    authorization TEXT,
    claim_amount_minor INTEGER NOT NULL,
    patient_portion_minor INTEGER NOT NULL,
    payer_portion_minor INTEGER NOT NULL,
    amount_paid_minor INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('draft','submitted','pending','approved','partially_paid','paid','rejected','cancelled')),
    submitted_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);

CREATE TABLE backup_records (
    id TEXT PRIMARY KEY,
    filename TEXT NOT NULL,
    path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    checksum_sha256 TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('manual','automatic','pre_restore')),
    verified INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    created_by TEXT REFERENCES users(id)
);

INSERT INTO payment_methods(id, name, active, sort_order) VALUES
('pm_cash', 'Cash', 1, 10),
('pm_card', 'Card', 1, 20),
('pm_transfer', 'Bank transfer', 1, 30),
('pm_mobile', 'Mobile payment', 1, 40),
('pm_other', 'Other', 1, 50);

