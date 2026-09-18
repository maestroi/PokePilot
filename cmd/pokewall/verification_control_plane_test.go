package main

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestVerificationSnapshotIncludesDurableFinishedHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
CREATE TABLE runs (
 run_id TEXT PRIMARY KEY,
 status TEXT NOT NULL,
 ended_at INTEGER NOT NULL,
 row_json BLOB NOT NULL
);`); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	row := tileRow{
		RunID: "finished-from-db", Status: statusDone, Planner: "llm",
		Starter: "squirtle", Goal: "beat the game", Attempts: 2,
		QueuedAt: now.Add(-time.Hour).Unix(), EndedAt: now.Unix(),
		Reason: "done",
	}
	raw, _ := json.Marshal(row)
	if _, err := db.Exec(`INSERT INTO runs(run_id,status,ended_at,row_json) VALUES(?,?,?,?)`,
		row.RunID, row.Status, row.EndedAt, raw); err != nil {
		t.Fatal(err)
	}

	w := NewWall("")
	wallCatalogs.Store(w, &runCatalog{db: db})
	t.Cleanup(func() { wallCatalogs.Delete(w) })

	_, tiles := w.verificationSnapshot()
	tile, ok := findVerificationTile(tiles, row.RunID)
	if !ok {
		t.Fatalf("durable finished run missing from verification snapshot: %#v", tiles)
	}
	if !tile.Finished || tile.Attempts != 2 || tile.Goal != row.Goal {
		t.Fatalf("verification tile = %#v", tile)
	}
	if tile.QueuedAt.Unix() != row.QueuedAt || tile.EndedAt.Unix() != row.EndedAt {
		t.Fatalf("verification times = queued %v ended %v", tile.QueuedAt, tile.EndedAt)
	}
}
