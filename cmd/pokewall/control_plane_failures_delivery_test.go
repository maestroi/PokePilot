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
 family_key TEXT NOT NULL DEFAULT '',
 family_fingerprint TEXT NOT NULL DEFAULT '',
 blocking BOOLEAN NOT NULL DEFAULT FALSE,
 terminal_count INTEGER NOT NULL DEFAULT 0,
 failure_json BLOB NOT NULL,
 report_json BLOB NOT NULL,
 delivery_status TEXT NOT NULL,
 delivery_error TEXT NOT NULL DEFAULT '',
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

func TestRequeueOrphanedObjectiveFailureErrorsOncePerRollout(t *testing.T) {
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
 family_key TEXT NOT NULL DEFAULT '',
 family_fingerprint TEXT NOT NULL DEFAULT '',
 blocking BOOLEAN NOT NULL DEFAULT FALSE,
 terminal_count INTEGER NOT NULL DEFAULT 0,
 delivery_status TEXT NOT NULL,
 delivery_error TEXT NOT NULL DEFAULT '',
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

	insert := func(runID, key string, blocking bool, terminal int, linked bool) {
		t.Helper()
		if _, err := db.Exec(`
INSERT INTO objective_failures(run_id,attempt,failure_key,blocking,terminal_count,delivery_status,delivery_error)
VALUES(?,?,?,?,?,'error','old sink rejected report')`, runID, 1, key, blocking, terminal); err != nil {
			t.Fatal(err)
		}
		if linked {
			if _, err := db.Exec(`INSERT INTO issue_links(failure_key,issue_id) VALUES(?,?)`, key, "123"); err != nil {
				t.Fatal(err)
			}
		}
	}

	insert("orphan-blocking", "orphan-blocking-key", true, 0, false)
	insert("orphan-terminal", "orphan-terminal-key", false, 1, false)
	insert("linked", "linked-key", true, 1, true)
	insert("nonblocking", "nonblocking-key", false, 0, false)

	cp := &controlPlane{db: db}
	n, err := cp.requeueOrphanedObjectiveFailureErrors()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("requeued = %d, want 2", n)
	}

	rows, err := db.Query(`SELECT failure_key,delivery_status,delivery_error FROM objective_failures ORDER BY failure_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string][2]string{}
	for rows.Next() {
		var key, status, deliveryErr string
		if err := rows.Scan(&key, &status, &deliveryErr); err != nil {
			t.Fatal(err)
		}
		got[key] = [2]string{status, deliveryErr}
	}
	if got["orphan-blocking-key"] != [2]string{"pending", ""} {
		t.Fatalf("orphan blocking = %v", got["orphan-blocking-key"])
	}
	if got["orphan-terminal-key"] != [2]string{"pending", ""} {
		t.Fatalf("orphan terminal = %v", got["orphan-terminal-key"])
	}
	if got["linked-key"][0] != "error" {
		t.Fatalf("linked error was requeued: %v", got["linked-key"])
	}
	if got["nonblocking-key"][0] != "error" {
		t.Fatalf("nonblocking error was requeued: %v", got["nonblocking-key"])
	}

	// Calling the recovery twice in one process is harmless: the first pass
	// changed only the eligible rows to pending, so there is nothing left to
	// requeue until a delivery attempt marks one error again.
	n, err = cp.requeueOrphanedObjectiveFailureErrors()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second requeue = %d, want 0", n)
	}
}
