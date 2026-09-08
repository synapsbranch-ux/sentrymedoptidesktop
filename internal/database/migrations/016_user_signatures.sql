-- D3: a doctor's signature, owned by exactly one user.
CREATE TABLE user_signatures (
    user_id TEXT PRIMARY KEY REFERENCES users(id),
    -- 'uploaded' from an image file, or 'drawn' on the signature pad.
    method TEXT NOT NULL CHECK (method IN ('uploaded', 'drawn')),
    storage_name TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    checksum_sha256 TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- A prescription keeps a snapshot of the signature that was applied to it, plus
-- who signed and when. Replacing a doctor's signature must never retroactively
-- change a document that has already been issued.
ALTER TABLE prescriptions ADD COLUMN signed_by TEXT REFERENCES users(id);
ALTER TABLE prescriptions ADD COLUMN signed_at TEXT;
ALTER TABLE prescriptions ADD COLUMN signature_storage_name TEXT;
ALTER TABLE prescriptions ADD COLUMN signature_media_type TEXT;
