package main

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/farm"
	_ "modernc.org/sqlite"
)

func TestPendingObjectiveFailuresRedeliversOrphanedCompleteBlockingRows(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
CREATE TABLE objective_failures (
 run_id TEXT NOT NULL,
 attempt INTEGER NOT NULL,
 failure_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL DEFAULT '',
 blocking BOOLEAN NOT NULL DEFAULT FALSE,
 terminal_count INTEGER NOT NULL DEFAULT 0,
 failure_json BLOB NOT NULL,
 report_json BLOB NOT NULL,
 delivery_status TEXT NOT NULL,
 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(run_id,attempt,failure_key)
);
CREATE TABLE issue_links (
 failure_key TEXT PRIMARY KEY,
 issue_id TEXT NOT NULL DEFAULT ''
);
`); err != nil {
		t.Fatal(err)
	}

	insert := func(runID, key, status string, blocking bool, terminal int, linked bool) {
		t.Helper()
		failure := farm.ObjectiveFailure{
			Objective:     "advance story",
			Error:         "blocked",
			Count:         1,
			Blocking:      blocking,
			TerminalCount: terminal,
		}
		report := farm.FinishReport{RunID: runID, Attempt: 1, Reason: "failed"}
		failureRaw, _ := json.Marshal(failure)
		reportRaw, _ := json.Marshal(report)
		if _, err := db.Exec(`
INSERT INTO objective_failures(run_id,attempt,failure_key,blocking,terminal_count,failure_json,report_json,delivery_status)
VALUES(?,?,?,?,?,?,?,?)`, runID, 1, key, blocking, terminal, failureRaw, reportRaw, status); err != nil {
			t.Fatal(err)
		}
		if linked {
			if _, err := db.Exec(`INSERT INTO issue_links(failure_key,issue_id) VALUES(?,?)`, key, "123"); err != nil {
				t.Fatal(err)
			}
		}
	}

	insert("pending", "pending-key", "pending", false, 0, false)
	insert("orphan-blocking", "orphan-blocking-key", "complete", true, 0, false)
	insert("orphan-terminal", "orphan-terminal-key", "complete", false, 1, false)
	insert("linked", "linked-key", "complete", true, 1, true)
	insert("nonblocking", "nonblocking-key", "complete", false, 0, false)
	insert("error", "error-key", "error", true, 1, false)

	cp := &controlPlane{db: db}
	items, err := cp.pendingObjectiveFailures(20)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, item := range items {
		got[item.key] = true
	}
	for _, key := range []string{"pending-key", "orphan-blocking-key", "orphan-terminal-key"} {
		if !got[key] {
			t.Errorf("missing %s from pending objective failures", key)
		}
	}
	for _, key := range []string{"linked-key", "nonblocking-key", "error-key"} {
		if got[key] {
			t.Errorf("unexpected %s in pending objective failures", key)
		}
	}
}
