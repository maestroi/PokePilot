package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunAttemptsAttachProblems(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
CREATE TABLE run_attempts (run_id TEXT, attempt INTEGER, reason TEXT, detail TEXT, runner_version TEXT);
CREATE TABLE objective_failures (run_id TEXT, attempt INTEGER, failure_key TEXT, fingerprint TEXT,
 family_key TEXT, family_fingerprint TEXT, blocking BOOLEAN, updated_at TEXT);
CREATE TABLE issue_links (failure_key TEXT, issue_id TEXT, status TEXT, payload_json TEXT);
INSERT INTO run_attempts VALUES ('r',1,'lost','',''),('r',2,'failed','boom','v1');
INSERT INTO objective_failures VALUES ('r',2,'k','fp','fam','famfp',1,'');
INSERT INTO issue_links VALUES ('fam','42','resolved','{"status":"resolved","fixed_revision":"v2"}');`); err != nil {
		t.Fatal(err)
	}
	got, err := (&controlPlane{db: db}).runAttempts("r")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[0].Problems) != 0 || len(got[1].Problems) != 1 {
		t.Fatalf("attempts = %+v", got)
	}
	if p := got[1].Problems[0]; p.TriageKey != "fam" || p.IssueID != "42" || !p.Blocking || !p.Fixed {
		t.Fatalf("problem = %+v", p)
	}
}

func TestBuildRunRecoveriesDeriveProblemAndFixed(t *testing.T) {
	attempts := []runAttemptView{
		{Attempt: 1, Problems: []attemptProblem{{TriageKey: "k", Fixed: true}}},
		{Attempt: 2, Problems: []attemptProblem{{TriageKey: "k2"}}},
		{Attempt: 3},
	}
	activity := []runActivityEvent{
		{Source: "system", Kind: "failure", Attempt: 1},
		{Source: "recovery", Kind: "retry", Attempt: 1},
		{Source: "recovery", Kind: "resume", Attempt: 2},
		{Source: "recovery", Kind: "retry", Attempt: 2},
		{Source: "recovery", Kind: "retry", Attempt: 3},
	}
	got := buildRunRecoveries(activity, attempts)
	if len(got) != 4 {
		t.Fatalf("recoveries = %+v", got)
	}
	want := []bool{true, true, false, false} // retry@1 fixed, resume@2 -> attempt 1 fixed, retry@2 open, retry@3 no problem
	for i, w := range want {
		if got[i].Fixed != w {
			t.Fatalf("recovery %d (%s@%d) fixed=%v, want %v", i, got[i].Kind, got[i].Attempt, got[i].Fixed, w)
		}
	}
}
