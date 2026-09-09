-- Atomic safeguards for concurrent scheduling and locked clinical records.

CREATE TRIGGER appointment_no_overlap_insert BEFORE INSERT ON appointments
WHEN NEW.archived_at IS NULL AND NEW.status NOT IN ('cancelled','no_show','completed')
 AND EXISTS (SELECT 1 FROM appointments a WHERE a.id <> NEW.id AND a.archived_at IS NULL
 AND a.status NOT IN ('cancelled','no_show','completed')
 AND COALESCE(a.practitioner_id,'') = COALESCE(NEW.practitioner_id,'')
 AND datetime(a.starts_at) < datetime(NEW.starts_at, '+'||NEW.duration_minutes||' minutes')
 AND datetime(a.starts_at, '+'||a.duration_minutes||' minutes') > datetime(NEW.starts_at))
BEGIN SELECT RAISE(ABORT, 'APPOINTMENT_CONFLICT'); END;

CREATE TRIGGER appointment_no_overlap_update BEFORE UPDATE ON appointments
WHEN NEW.archived_at IS NULL AND NEW.status NOT IN ('cancelled','no_show','completed')
 AND EXISTS (SELECT 1 FROM appointments a WHERE a.id <> NEW.id AND a.archived_at IS NULL
 AND a.status NOT IN ('cancelled','no_show','completed')
 AND COALESCE(a.practitioner_id,'') = COALESCE(NEW.practitioner_id,'')
 AND datetime(a.starts_at) < datetime(NEW.starts_at, '+'||NEW.duration_minutes||' minutes')
 AND datetime(a.starts_at, '+'||a.duration_minutes||' minutes') > datetime(NEW.starts_at))
BEGIN SELECT RAISE(ABORT, 'APPOINTMENT_CONFLICT'); END;

CREATE TRIGGER pretests_locked_insert BEFORE INSERT ON pretests
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=NEW.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER pretests_locked_update BEFORE UPDATE ON pretests
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER pretests_locked_delete BEFORE DELETE ON pretests
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER encounter_sections_locked_insert BEFORE INSERT ON encounter_sections
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=NEW.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER encounter_sections_locked_update BEFORE UPDATE ON encounter_sections
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER encounter_sections_locked_delete BEFORE DELETE ON encounter_sections
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER diagnoses_locked_insert BEFORE INSERT ON diagnoses
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=NEW.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER diagnoses_locked_update BEFORE UPDATE ON diagnoses
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER diagnoses_locked_delete BEFORE DELETE ON diagnoses
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER encounter_locked_update BEFORE UPDATE ON encounters
WHEN OLD.status='finalized'
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;
CREATE TRIGGER encounter_locked_delete BEFORE DELETE ON encounters
WHEN OLD.status='finalized'
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;
CREATE INDEX idx_login_attempts_identity_time ON login_attempts(identity COLLATE NOCASE, attempted_at);

CREATE TRIGGER recordings_locked_insert BEFORE INSERT ON consultation_recordings
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=NEW.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER recordings_locked_update BEFORE UPDATE ON consultation_recordings
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

CREATE TRIGGER recordings_locked_delete BEFORE DELETE ON consultation_recordings
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'ENCOUNTER_FINALIZED'); END;

-- Discard executable settings inherited from older installs. Configure the
-- trusted transcription tool through the server environment instead.
UPDATE settings SET value_json=json_remove(value_json,'$.transcriptionCommand') WHERE key='clinical' AND json_valid(value_json);
ALTER TABLE backup_records ADD COLUMN assets_checksum TEXT NOT NULL DEFAULT '';

CREATE TABLE backup_status(id INTEGER PRIMARY KEY CHECK(id=1), checked_at TEXT NOT NULL, error TEXT NOT NULL);

CREATE TABLE idempotency_records(scope TEXT PRIMARY KEY, request_hash TEXT NOT NULL, response_json TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE invoice_return_items(invoice_item_id TEXT PRIMARY KEY REFERENCES invoice_items(id), refund_id TEXT NOT NULL REFERENCES refunds(id));

CREATE TRIGGER users_keep_active_doctor BEFORE UPDATE OF active,archived_at,role ON users
WHEN OLD.role='doctor' AND OLD.active=1 AND OLD.archived_at IS NULL
 AND (NEW.active=0 OR NEW.archived_at IS NOT NULL OR NEW.role<>'doctor')
 AND NOT EXISTS(SELECT 1 FROM users WHERE id<>OLD.id AND role='doctor' AND active=1 AND archived_at IS NULL)
BEGIN SELECT RAISE(ABORT,'LAST_DOCTOR_REQUIRED'); END;
