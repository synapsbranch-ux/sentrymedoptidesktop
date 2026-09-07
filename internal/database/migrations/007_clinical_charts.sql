-- Generalise the annotated eye diagram into a chart engine that also carries the
-- fundus view, the confrontation visual field grid and the ocular motility grid.
-- SQLite cannot alter the UNIQUE(encounter_id, eye) constraint in place, so the
-- table is rebuilt and the existing rows are classified as anterior-segment views.
CREATE TABLE eye_diagrams_rebuilt (
    id TEXT PRIMARY KEY,
    encounter_id TEXT NOT NULL REFERENCES encounters(id),
    chart_type TEXT NOT NULL DEFAULT 'anterior' CHECK (chart_type IN ('anterior','fundus','field','motility')),
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

INSERT INTO eye_diagrams_rebuilt (id, encounter_id, chart_type, eye, annotations_json, notes, version, created_at, updated_at, created_by, updated_by)
SELECT id, encounter_id, 'anterior', eye, annotations_json, notes, version, created_at, updated_at, created_by, updated_by
FROM eye_diagrams;

DROP TABLE eye_diagrams;
ALTER TABLE eye_diagrams_rebuilt RENAME TO eye_diagrams;

CREATE INDEX idx_eye_diagrams_encounter ON eye_diagrams(encounter_id, chart_type);
