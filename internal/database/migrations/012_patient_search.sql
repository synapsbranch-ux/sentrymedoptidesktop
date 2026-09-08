-- B1: server-side patient search that stays fast at 5,000+ records.
--
-- A LIKE '%term%' scan can use no index and cannot ignore accents, so the
-- searchable text lives in an FTS5 index instead. The unicode61 tokenizer with
-- remove_diacritics 2 folds case and accents at both index and query time, so
-- "jean" finds "Jéan" without a normalised copy of the data in application code.

CREATE VIRTUAL TABLE patients_search USING fts5(
    content,
    patient_id UNINDEXED,
    tokenize = "unicode61 remove_diacritics 2"
);

-- Phone numbers are indexed twice: as written, and with separators stripped, so
-- that both "3456 7890" and "34567890" find the same patient.
CREATE VIEW patients_search_source AS
SELECT
    id AS patient_id,
    COALESCE(first_name, '') || ' ' || COALESCE(middle_name, '') || ' ' || COALESCE(last_name, '') || ' ' ||
    COALESCE(preferred_name, '') || ' ' || COALESCE(medical_record_number, '') || ' ' ||
    COALESCE(phone, '') || ' ' ||
    replace(replace(replace(replace(replace(replace(COALESCE(phone, ''), ' ', ''), '-', ''), '(', ''), ')', ''), '+', ''), '.', '') || ' ' ||
    COALESCE(alternate_phone, '') || ' ' ||
    replace(replace(replace(replace(replace(replace(COALESCE(alternate_phone, ''), ' ', ''), '-', ''), '(', ''), ')', ''), '+', ''), '.', '') || ' ' ||
    COALESCE(email, '') || ' ' || COALESCE(date_of_birth, '') AS content
FROM patients;

INSERT INTO patients_search(patient_id, content) SELECT patient_id, content FROM patients_search_source;

CREATE TRIGGER patients_search_insert AFTER INSERT ON patients BEGIN
    INSERT INTO patients_search(patient_id, content)
    SELECT patient_id, content FROM patients_search_source WHERE patient_id = new.id;
END;

CREATE TRIGGER patients_search_update AFTER UPDATE ON patients BEGIN
    DELETE FROM patients_search WHERE patient_id = new.id;
    INSERT INTO patients_search(patient_id, content)
    SELECT patient_id, content FROM patients_search_source WHERE patient_id = new.id;
END;

CREATE TRIGGER patients_search_delete AFTER DELETE ON patients BEGIN
    DELETE FROM patients_search WHERE patient_id = old.id;
END;

-- Indexes for the structured filters and the default ordering. idx_patients_name
-- and idx_patients_phone already exist; these cover what the filters actually
-- narrow on and what a joined lookup needs.
CREATE INDEX idx_patients_updated ON patients(updated_at DESC);
CREATE INDEX idx_patients_dob ON patients(date_of_birth);
CREATE INDEX idx_patients_archived ON patients(archived_at, updated_at DESC);
CREATE INDEX idx_patient_insurance_patient ON patient_insurance(patient_id, is_primary DESC);
CREATE INDEX idx_encounters_doctor ON encounters(doctor_id, created_at DESC);
CREATE INDEX idx_appointments_practitioner ON appointments(practitioner_id, starts_at DESC);
