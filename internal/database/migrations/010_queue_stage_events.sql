-- Queue rows only retain their current stage. This append-only transition history makes
-- wait estimates, bottlenecks and actual consultation duration reproducible instead of
-- inferring them from the row's latest updated_at timestamp.
CREATE TABLE queue_stage_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    queue_entry_id TEXT NOT NULL REFERENCES queue_entries(id) ON DELETE CASCADE,
    stage TEXT NOT NULL,
    entered_at TEXT NOT NULL,
    exited_at TEXT
);

CREATE INDEX idx_queue_stage_events_entry ON queue_stage_events(queue_entry_id, entered_at);
CREATE INDEX idx_queue_stage_events_stage ON queue_stage_events(stage, entered_at, exited_at);

-- Existing records receive one explicitly approximate event. New transitions are exact.
INSERT INTO queue_stage_events(queue_entry_id, stage, entered_at, exited_at)
SELECT id, stage, arrived_at,
       CASE WHEN completed_at IS NOT NULL AND stage <> 'completed' THEN completed_at ELSE NULL END
FROM queue_entries;

CREATE TRIGGER queue_stage_event_after_insert
AFTER INSERT ON queue_entries
BEGIN
    INSERT INTO queue_stage_events(queue_entry_id, stage, entered_at)
    VALUES(NEW.id, NEW.stage, NEW.arrived_at);
END;

CREATE TRIGGER queue_stage_event_after_change
AFTER UPDATE OF stage ON queue_entries
WHEN OLD.stage <> NEW.stage
BEGIN
    UPDATE queue_stage_events
       SET exited_at = NEW.updated_at
     WHERE id = (
        SELECT id FROM queue_stage_events
         WHERE queue_entry_id = NEW.id AND exited_at IS NULL
         ORDER BY id DESC LIMIT 1
     );
    INSERT INTO queue_stage_events(queue_entry_id, stage, entered_at)
    VALUES(NEW.id, NEW.stage, NEW.updated_at);
END;
