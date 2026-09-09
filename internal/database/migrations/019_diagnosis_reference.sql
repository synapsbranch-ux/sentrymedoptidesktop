-- Diagnosis coding used to be a short list compiled into the binary, searchable
-- only by exact code or a substring of the official wording. A clinician who
-- knows the patient has a "red eye" but not that ICD calls it conjunctivitis had
-- nothing to type. The reference is now a table: several hundred
-- ophthalmology-relevant codes, each carrying the lay terms a patient actually
-- uses, and a clinic can load its own official code file on top.
CREATE TABLE diagnosis_codes (
    code TEXT PRIMARY KEY,
    -- Which classification the code belongs to, so an ICD-11 or a local code set
    -- can sit beside ICD-10 without one shadowing the other.
    system TEXT NOT NULL DEFAULT 'ICD-10',
    description TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT '',
    -- Semicolon-separated lay terms and presenting complaints. Indexed for
    -- search but never shown as if it were the official wording.
    synonyms TEXT NOT NULL DEFAULT '',
    -- 'builtin' rows are replaced whenever the bundled reference changes;
    -- 'clinic' rows are the clinic's own and are never overwritten.
    source TEXT NOT NULL DEFAULT 'builtin' CHECK (source IN ('builtin', 'clinic')),
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_diagnosis_codes_browse ON diagnosis_codes(active, category, code);
CREATE INDEX idx_diagnosis_codes_system ON diagnosis_codes(system, active);

-- Same tokenizer as the patient index: case and accents fold at index and query
-- time, so "keratite" finds "Keratitis" without a normalized second copy.
CREATE VIRTUAL TABLE diagnosis_codes_search USING fts5(
    content,
    code UNINDEXED,
    tokenize = "unicode61 remove_diacritics 2"
);

CREATE VIEW diagnosis_codes_search_source AS
SELECT code, code || ' ' || description || ' ' || category || ' ' || synonyms AS content
FROM diagnosis_codes WHERE active = 1;

CREATE TRIGGER diagnosis_codes_search_insert AFTER INSERT ON diagnosis_codes BEGIN
    INSERT INTO diagnosis_codes_search(code, content)
    SELECT code, content FROM diagnosis_codes_search_source WHERE code = new.code;
END;

CREATE TRIGGER diagnosis_codes_search_update AFTER UPDATE ON diagnosis_codes BEGIN
    DELETE FROM diagnosis_codes_search WHERE code = new.code;
    INSERT INTO diagnosis_codes_search(code, content)
    SELECT code, content FROM diagnosis_codes_search_source WHERE code = new.code;
END;

CREATE TRIGGER diagnosis_codes_search_delete AFTER DELETE ON diagnosis_codes BEGIN
    DELETE FROM diagnosis_codes_search WHERE code = old.code;
END;

-- Tracks which build of the bundled reference is loaded, so the sync at startup
-- is a no-op on every run but the first after an upgrade.
CREATE TABLE reference_data_versions (
    name TEXT PRIMARY KEY,
    checksum TEXT NOT NULL,
    row_count INTEGER NOT NULL,
    applied_at TEXT NOT NULL
);
