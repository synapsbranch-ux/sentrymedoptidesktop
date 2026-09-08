-- Additive migration: preserve legacy coordinates and snapshot the existing state.
ALTER TABLE eye_diagrams ADD COLUMN exam_status TEXT NOT NULL DEFAULT 'unspecified'
    CHECK (exam_status IN ('unspecified','not_examined','no_findings','findings'));

CREATE TABLE clinical_chart_revisions (
    chart_id TEXT NOT NULL REFERENCES eye_diagrams(id),
    version INTEGER NOT NULL,
    annotations_json TEXT NOT NULL,
    notes TEXT,
    exam_status TEXT NOT NULL,
    recorded_at TEXT NOT NULL,
    recorded_by TEXT NOT NULL REFERENCES users(id),
    baseline INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(chart_id, version)
);

INSERT INTO clinical_chart_revisions
SELECT id,version,annotations_json,notes,exam_status,updated_at,updated_by,1 FROM eye_diagrams;

CREATE TRIGGER chart_revision_insert AFTER INSERT ON eye_diagrams
BEGIN
    INSERT INTO clinical_chart_revisions VALUES
        (NEW.id,NEW.version,NEW.annotations_json,NEW.notes,NEW.exam_status,NEW.updated_at,NEW.updated_by,0);
END;

CREATE TRIGGER chart_revision_update AFTER UPDATE ON eye_diagrams
BEGIN
    INSERT INTO clinical_chart_revisions VALUES
        (NEW.id,NEW.version,NEW.annotations_json,NEW.notes,NEW.exam_status,NEW.updated_at,NEW.updated_by,0);
END;

-- Enforce the signature lock inside the write itself, including concurrent requests.
CREATE TRIGGER chart_locked_insert BEFORE INSERT ON eye_diagrams
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=NEW.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'CHART_ENCOUNTER_LOCKED'); END;

CREATE TRIGGER chart_locked_update BEFORE UPDATE ON eye_diagrams
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'CHART_ENCOUNTER_LOCKED'); END;

CREATE TRIGGER chart_locked_delete BEFORE DELETE ON eye_diagrams
WHEN EXISTS (SELECT 1 FROM encounters WHERE id=OLD.encounter_id AND (status='finalized' OR archived_at IS NOT NULL))
BEGIN SELECT RAISE(ABORT, 'CHART_ENCOUNTER_LOCKED'); END;

CREATE TRIGGER chart_revision_no_update BEFORE UPDATE ON clinical_chart_revisions
BEGIN SELECT RAISE(ABORT, 'CHART_REVISION_IMMUTABLE'); END;
CREATE TRIGGER chart_revision_no_delete BEFORE DELETE ON clinical_chart_revisions
BEGIN SELECT RAISE(ABORT, 'CHART_REVISION_IMMUTABLE'); END;
