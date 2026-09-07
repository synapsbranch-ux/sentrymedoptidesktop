-- A vision test session is the shared state between the screen in the exam lane and
-- the phone driving it. Both devices read and write the same row, so `revision` is a
-- change counter the display watches, not an optimistic lock: two devices writing in
-- turn is the intended use, and a lock would only manufacture conflicts.
CREATE TABLE vision_test_sessions (
    id TEXT PRIMARY KEY,
    room TEXT NOT NULL,
    encounter_id TEXT REFERENCES encounters(id),
    state_json TEXT NOT NULL DEFAULT '{}',
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id),
    closed_at TEXT
);
CREATE INDEX idx_vision_sessions_open ON vision_test_sessions(closed_at, updated_at DESC);
CREATE INDEX idx_vision_sessions_encounter ON vision_test_sessions(encounter_id);
