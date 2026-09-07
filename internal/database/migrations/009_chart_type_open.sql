-- Adding a chart type should cost a drawing, not a table rebuild. The chart_type CHECK
-- constraint had to be rewritten for every new view, so it is dropped here and the
-- `chartEyes` map in internal/server/clinical_charts.go becomes the single source of
-- truth for which charts exist and which eyes each is recorded for.
CREATE TABLE eye_diagrams_open (
    id TEXT PRIMARY KEY,
    encounter_id TEXT NOT NULL REFERENCES encounters(id),
    chart_type TEXT NOT NULL DEFAULT 'anterior',
    eye TEXT NOT NULL CHECK (eye IN ('OD','OS','OU')),
    annotations_json TEXT NOT NULL DEFAULT '[]',
    notes TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id),
    UNIQUE(encounter_id, chart_type, eye)
);

INSERT INTO eye_diagrams_open (id, encounter_id, chart_type, eye, annotations_json, notes, version, created_at, updated_at, created_by, updated_by)
SELECT id, encounter_id, chart_type, eye, annotations_json, notes, version, created_at, updated_at, created_by, updated_by
FROM eye_diagrams;

DROP TABLE eye_diagrams;
ALTER TABLE eye_diagrams_open RENAME TO eye_diagrams;

CREATE INDEX idx_eye_diagrams_encounter ON eye_diagrams(encounter_id, chart_type);
