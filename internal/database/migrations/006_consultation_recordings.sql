CREATE TABLE consultation_recordings (
    id TEXT PRIMARY KEY,
    encounter_id TEXT NOT NULL REFERENCES encounters(id),
    storage_name TEXT NOT NULL UNIQUE,
    media_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    checksum_sha256 TEXT NOT NULL,
    consent_confirmed INTEGER NOT NULL DEFAULT 0 CHECK (consent_confirmed IN (0,1)),
    transcript_status TEXT NOT NULL DEFAULT 'pending' CHECK (transcript_status IN ('pending','processing','done','failed','unavailable')),
    transcript_text TEXT,
    transcript_edited INTEGER NOT NULL DEFAULT 0 CHECK (transcript_edited IN (0,1)),
    transcript_error TEXT,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_recordings_encounter ON consultation_recordings(encounter_id, created_at DESC);
