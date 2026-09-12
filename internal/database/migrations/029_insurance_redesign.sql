-- The insurance workflow previously stored little more than an insurer's
-- name and a flat coverage percentage. This migration turns it into a real
-- clinic workflow: a manageable provider profile, a fuller patient policy
-- (verification, dates, subscriber details, an active flag), a place for the
-- patient's insurance-card photographs, a documented forms/requests
-- workflow, and richer claim tracking — without breaking any existing row,
-- and without touching the CHECK-constrained status/type columns that other
-- tables already reference by foreign key.

-- ---- Providers (payers) ----------------------------------------------------

ALTER TABLE payers ADD COLUMN website TEXT;
ALTER TABLE payers ADD COLUMN notes TEXT;
-- Free text on purpose: what a provider covers, how it wants to be billed and
-- how to file a claim varies far too much between insurers to model as a
-- fixed set of columns right now. A logo is a photo, so it reuses the
-- existing entity_images gallery (see images.go) rather than a new column.
ALTER TABLE payers ADD COLUMN accepted_coverage TEXT;
ALTER TABLE payers ADD COLUMN billing_info TEXT;
ALTER TABLE payers ADD COLUMN claim_instructions TEXT;

-- ---- Patient insurance policies --------------------------------------------

ALTER TABLE patient_insurance ADD COLUMN group_number TEXT;
ALTER TABLE patient_insurance ADD COLUMN subscriber_name TEXT;
ALTER TABLE patient_insurance ADD COLUMN relationship_to_subscriber TEXT
    CHECK (relationship_to_subscriber IS NULL OR relationship_to_subscriber IN ('self','spouse','child','other'));
ALTER TABLE patient_insurance ADD COLUMN effective_date TEXT;
ALTER TABLE patient_insurance ADD COLUMN expiration_date TEXT;
ALTER TABLE patient_insurance ADD COLUMN coverage_notes TEXT;
ALTER TABLE patient_insurance ADD COLUMN notes TEXT;
-- Verification starts manual, deliberately: nothing here should read as an
-- electronic eligibility check unless a real insurer/API integration exists.
ALTER TABLE patient_insurance ADD COLUMN verification_status TEXT NOT NULL DEFAULT 'not_verified'
    CHECK (verification_status IN ('not_verified','pending_verification','verified','expired','rejected'));
ALTER TABLE patient_insurance ADD COLUMN verified_by TEXT REFERENCES users(id);
ALTER TABLE patient_insurance ADD COLUMN verified_at TEXT;
ALTER TABLE patient_insurance ADD COLUMN verification_reference TEXT;
ALTER TABLE patient_insurance ADD COLUMN verification_contact TEXT;
ALTER TABLE patient_insurance ADD COLUMN verification_notes TEXT;
-- Deactivating a policy (expired coverage, patient switched insurer) keeps
-- its history instead of deleting it; removal stays available separately
-- when a policy was recorded in error and nothing references it yet.
ALTER TABLE patient_insurance ADD COLUMN is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1));

CREATE INDEX idx_patient_insurance_patient_active ON patient_insurance(patient_id, is_active);

-- At most one active primary policy per patient. Demote every primary policy
-- except the most recently recorded one first, so this rule can never fail
-- to apply because of data that predates it.
UPDATE patient_insurance SET is_primary = 0
WHERE is_primary = 1
  AND created_at <> (SELECT MAX(p2.created_at) FROM patient_insurance p2 WHERE p2.patient_id = patient_insurance.patient_id AND p2.is_primary = 1);
CREATE UNIQUE INDEX idx_patient_insurance_one_primary ON patient_insurance(patient_id) WHERE is_primary = 1 AND is_active = 1;

-- ---- Insurance cards --------------------------------------------------------

-- Exactly two named sides, each replaceable independently. Bytes live under
-- <data-directory>/insurance-cards, following the same pattern as documents
-- and entity_images: metadata and a checksum here, content on disk, served
-- only through the authenticated API (never a public path).
CREATE TABLE patient_insurance_cards (
    id TEXT PRIMARY KEY,
    patient_insurance_id TEXT NOT NULL REFERENCES patient_insurance(id),
    side TEXT NOT NULL CHECK (side IN ('front', 'back')),
    storage_name TEXT NOT NULL UNIQUE,
    media_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    checksum_sha256 TEXT NOT NULL,
    uploaded_at TEXT NOT NULL,
    uploaded_by TEXT NOT NULL REFERENCES users(id),
    UNIQUE (patient_insurance_id, side)
);

-- ---- Insurance forms and document requests ---------------------------------

-- One table for both concerns, because they are the same lifecycle at
-- different stages: a provider's blank claim form (payer_id set, nothing
-- else) starts REQUIRED; asking a specific patient for their own copy sets
-- patient_id and moves it to REQUESTED; it becomes RECEIVED once a file is
-- attached, then COMPLETED/SUBMITTED as staff process it, or REJECTED /
-- EXPIRED. A claim's supporting paperwork uses the same row shape with
-- claim_id set. existing_document_id lets a document already on file (e.g.
-- a prescription or a patient upload) be associated here instead of
-- duplicated.
CREATE TABLE insurance_documents (
    id TEXT PRIMARY KEY,
    payer_id TEXT REFERENCES payers(id),
    patient_id TEXT REFERENCES patients(id),
    claim_id TEXT REFERENCES insurance_claims(id),
    patient_insurance_id TEXT REFERENCES patient_insurance(id),
    document_type TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('required', 'requested', 'received', 'completed', 'submitted', 'rejected', 'expired')),
    storage_name TEXT UNIQUE,
    display_name TEXT,
    media_type TEXT,
    size_bytes INTEGER,
    checksum_sha256 TEXT,
    existing_document_id TEXT REFERENCES documents(id),
    requested_by TEXT REFERENCES users(id),
    requested_at TEXT,
    received_at TEXT,
    expiration_date TEXT,
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id),
    CHECK (payer_id IS NOT NULL OR patient_id IS NOT NULL OR claim_id IS NOT NULL)
);
CREATE INDEX idx_insurance_documents_payer ON insurance_documents(payer_id, status);
CREATE INDEX idx_insurance_documents_patient ON insurance_documents(patient_id, status);
CREATE INDEX idx_insurance_documents_claim ON insurance_documents(claim_id);

-- ---- Insurance claims: richer context and approval tracking ----------------

-- What the insurer actually approved, distinct from what was claimed — a
-- partial approval is claim_amount_minor > approved_amount_minor, so this is
-- tracked as an amount rather than adding another claim status. Every
-- existing claim keeps approved_amount_minor NULL until a doctor records one.
ALTER TABLE insurance_claims ADD COLUMN approved_amount_minor INTEGER;
ALTER TABLE insurance_claims ADD COLUMN response_date TEXT;
ALTER TABLE insurance_claims ADD COLUMN notes TEXT;
ALTER TABLE insurance_claims ADD COLUMN encounter_id TEXT REFERENCES encounters(id);
ALTER TABLE insurance_claims ADD COLUMN appointment_id TEXT REFERENCES appointments(id);
ALTER TABLE insurance_claims ADD COLUMN prescription_id TEXT REFERENCES prescriptions(id);
CREATE INDEX idx_insurance_claims_encounter ON insurance_claims(encounter_id);

-- entity_images already supports a photo gallery for any owner named in its
-- imageOwners map (see images.go); "payer" is added there in code, not here —
-- no schema change is needed for a provider logo/photos.
