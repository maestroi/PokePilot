package main

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

// One changed outbox entry must cost one row write, not a rewrite of the
// whole table (production rewrote ~16k rows several times a minute).
func TestPersistWallWritesOnlyChangedRows(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
CREATE TABLE control_plane_state (id INTEGER PRIMARY KEY, state_json BLOB NOT NULL, updated_at TIMESTAMP);
CREATE TABLE issue_links (failure_key TEXT PRIMARY KEY, issue_id TEXT, status TEXT, fingerprint TEXT, payload_json BLOB, updated_at TIMESTAMP);
CREATE TABLE issue_fingerprints (failure_key TEXT PRIMARY KEY, fingerprint TEXT, payload_json BLOB, updated_at TIMESTAMP);
CREATE TABLE issue_outbox (external_id TEXT PRIMARY KEY, run_id TEXT, attempt INTEGER, failure_key TEXT, status TEXT, next_attempt INTEGER, payload_json BLOB, updated_at TIMESTAMP);
CREATE TABLE writes (n INTEGER);
INSERT INTO writes VALUES (0);
CREATE TRIGGER o_ins AFTER INSERT ON issue_outbox BEGIN UPDATE writes SET n=n+1; END;
CREATE TRIGGER o_upd AFTER UPDATE ON issue_outbox BEGIN UPDATE writes SET n=n+1; END;
CREATE TRIGGER o_del AFTER DELETE ON issue_outbox BEGIN UPDATE writes SET n=n+1; END;
CREATE TRIGGER l_ins AFTER INSERT ON issue_links BEGIN UPDATE writes SET n=n+1; END;
CREATE TRIGGER l_upd AFTER UPDATE ON issue_links BEGIN UPDATE writes SET n=n+1; END;
CREATE TRIGGER l_del AFTER DELETE ON issue_links BEGIN UPDATE writes SET n=n+1; END;
`); err != nil {
		t.Fatal(err)
	}
	writes := func() int {
		var n int
		if err := db.QueryRow(`SELECT n FROM writes`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	count := func(table string) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	w := NewWall(t.TempDir())
	cp := &controlPlane{db: db}
	w.mu.Lock()
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("out-%02d", i)
		w.outbox[id] = outboxEntry{ExternalID: id, RunID: "run", Attempt: i, Status: outboxQuarantined}
		w.issueLinks[fmt.Sprintf("key-%02d", i)] = IssueLink{IssueID: "i", Status: "open"}
	}
	w.mu.Unlock()

	if err := cp.persistWall(w); err != nil {
		t.Fatal(err)
	}
	if count("issue_outbox") != 50 || count("issue_links") != 50 {
		t.Fatalf("seed rows = %d outbox, %d links", count("issue_outbox"), count("issue_links"))
	}

	before := writes()
	if err := cp.persistWall(w); err != nil {
		t.Fatal(err)
	}
	if got := writes() - before; got != 0 {
		t.Fatalf("unchanged persist wrote %d rows", got)
	}

	w.mu.Lock()
	e := w.outbox["out-07"]
	e.Status = outboxComplete
	w.outbox["out-07"] = e
	delete(w.outbox, "out-08")
	link := w.issueLinks["key-03"]
	link.UpdatedAt = 42
	w.issueLinks["key-03"] = link
	w.mu.Unlock()

	before = writes()
	if err := cp.persistWall(w); err != nil {
		t.Fatal(err)
	}
	if got := writes() - before; got != 3 {
		t.Fatalf("one update + one delete + one link change wrote %d rows, want 3", got)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM issue_outbox WHERE external_id='out-07'`).Scan(&status); err != nil || status != outboxComplete {
		t.Fatalf("out-07 status = %q, %v", status, err)
	}
	if count("issue_outbox") != 49 {
		t.Fatalf("outbox rows = %d, want 49", count("issue_outbox"))
	}

	// A fresh process (new coordinator) must still reconcile rows it never saw.
	controlPlaneWriteCoordinators.Delete(cp)
	w.mu.Lock()
	delete(w.outbox, "out-09")
	w.mu.Unlock()
	if err := cp.persistWall(w); err != nil {
		t.Fatal(err)
	}
	if count("issue_outbox") != 48 {
		t.Fatalf("outbox rows after fresh-process persist = %d, want 48", count("issue_outbox"))
	}
}
