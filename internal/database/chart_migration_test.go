package database

import (
	"database/sql"
	"testing"
)

func TestChartRevisionMigrationPreservesLegacyContent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys=ON;
	CREATE TABLE users(id TEXT PRIMARY KEY);
	CREATE TABLE encounters(id TEXT PRIMARY KEY,status TEXT,archived_at TEXT);
	CREATE TABLE eye_diagrams(id TEXT PRIMARY KEY,encounter_id TEXT,version INTEGER,annotations_json TEXT,notes TEXT,updated_at TEXT,updated_by TEXT);
	INSERT INTO users VALUES('doctor');
	INSERT INTO encounters VALUES('visit','finalized',NULL);
	INSERT INTO eye_diagrams VALUES('chart','visit',4,'[{"x":12.5,"y":30.1,"label":"old finding"}]','old notes','2025-01-01T00:00:00Z','doctor');`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := migrationFiles.ReadFile("migrations/011_chart_revisions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	var raw, notes, status string
	var version, baseline int
	if err = db.QueryRow("SELECT annotations_json,notes,exam_status,version,baseline FROM clinical_chart_revisions").Scan(&raw, &notes, &status, &version, &baseline); err != nil {
		t.Fatal(err)
	}
	if raw != `[{"x":12.5,"y":30.1,"label":"old finding"}]` || notes != "old notes" || status != "unspecified" || version != 4 || baseline != 1 {
		t.Fatalf("migration changed legacy data: %s %s %s %d %d", raw, notes, status, version, baseline)
	}
}
