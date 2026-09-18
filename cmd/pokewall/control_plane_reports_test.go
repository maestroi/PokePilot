package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/farm"
	_ "modernc.org/sqlite"
	"database/sql"
)

func TestControlPlaneFinishReportDoesNotNeedLocalCache(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
CREATE TABLE run_attempts (
 run_id TEXT NOT NULL, attempt INTEGER NOT NULL, report_json BLOB NOT NULL,
 PRIMARY KEY(run_id,attempt)
);
CREATE TABLE artifacts (
 run_id TEXT NOT NULL, attempt INTEGER NOT NULL, kind TEXT NOT NULL, name TEXT NOT NULL,
 metadata_json BLOB NOT NULL, inline_data BLOB
);`); err != nil {
		t.Fatal(err)
	}

	report := farm.FinishReport{RunID: "db-only", Attempt: 2, Reason: "failed", Detail: "boom", SeedBurn: 3}
	raw, _ := json.Marshal(report)
	if _, err := db.Exec(`INSERT INTO run_attempts(run_id,attempt,report_json) VALUES(?,?,?)`, report.RunID, report.Attempt, raw); err != nil {
		t.Fatal(err)
	}

	data := []byte(`{"failure":"typed"}`)
	sum := sha256.Sum256(data)
	meta := farm.Artifact{
		Name: "objective-failures.json", MediaType: "application/json",
		SHA256: hex.EncodeToString(sum[:]),
	}
	metaRaw, _ := json.Marshal(meta)
	if _, err := db.Exec(`INSERT INTO artifacts(run_id,attempt,kind,name,metadata_json,inline_data) VALUES(?,?,'finish',?,?,?)`,
		report.RunID, report.Attempt, meta.Name, metaRaw, data); err != nil {
		t.Fatal(err)
	}

	w := NewWall("")
	cp := &controlPlane{db: db}
	wallControlPlanes.Store(w, cp)
	t.Cleanup(func() { wallControlPlanes.Delete(w) })

	got, err := w.loadLatestFinishReport(report.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt != 2 || got.Detail != "boom" {
		t.Fatalf("report = %#v", got)
	}
	if len(got.Artifacts) != 1 || string(got.Artifacts[0].Data) != string(data) {
		t.Fatalf("artifacts = %#v", got.Artifacts)
	}
}
