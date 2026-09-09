-- Frames are bought by eye, and a lab needs to see the frame it is glazing. One
-- attachment table serves both rather than a photo column on each: the storage,
-- the size limits, the backup path and the deletion rules are then written once.
CREATE TABLE entity_images (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('inventory_item', 'lab_order')),
    entity_id TEXT NOT NULL,
    -- The file on disk. Client filenames never reach the filesystem.
    storage_name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    checksum_sha256 TEXT NOT NULL,
    caption TEXT,
    -- Position in the gallery; the first image is the one shown in a list.
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_entity_images_owner ON entity_images(entity_type, entity_id, sort_order, created_at);
