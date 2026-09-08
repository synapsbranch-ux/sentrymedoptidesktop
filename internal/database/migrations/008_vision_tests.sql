-- A vision test session is the shared state between the screen in the exam lane and
-- the phone driving it. `revision` is both the SSE-visible change counter and an
-- optimistic lock: a delayed command must not overwrite a calibration or patient trace
-- that the other device has just written.
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
