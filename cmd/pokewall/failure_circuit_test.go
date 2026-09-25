package main

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
	_ "modernc.org/sqlite"
)

func newFailureCircuitTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
CREATE TABLE objective_failures (
 run_id TEXT NOT NULL, attempt INTEGER NOT NULL, failure_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL, family_key TEXT NOT NULL DEFAULT '',
 family_fingerprint TEXT NOT NULL DEFAULT '', blocking BOOLEAN NOT NULL DEFAULT FALSE,
 terminal_count INTEGER NOT NULL DEFAULT 0, failure_json BLOB NOT NULL DEFAULT '{}',
 delivery_status TEXT NOT NULL DEFAULT 'pending', delivery_error TEXT NOT NULL DEFAULT '',
 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(run_id,attempt,failure_key)
);
CREATE TABLE issue_links (
 failure_key TEXT PRIMARY KEY,
 issue_id TEXT NOT NULL DEFAULT ''
);
CREATE TABLE run_attempts (
 run_id TEXT NOT NULL, attempt INTEGER NOT NULL, reason TEXT NOT NULL DEFAULT '',
 runner_version TEXT NOT NULL DEFAULT '', report_json BLOB NOT NULL DEFAULT '{}',
 finished_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(run_id,attempt)
);
CREATE TABLE runs (run_id TEXT PRIMARY KEY, row_json BLOB NOT NULL);
`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTerminalRunFailureSynthesizesError(t *testing.T) {
	report := farm.FinishReport{
		RunID: "run-error", Attempt: 1, Reason: "error", Detail: "planner exhausted",
		RunnerVersion: "build-a", ProgressFinal: &farm.Progress{Round: 7, Badges: 5, Map: 0x0f},
	}
	failure, ok := terminalRunFailure(report, nil)
	if !ok {
		t.Fatal("terminal error did not synthesize a failure")
	}
	if !failure.Blocking || failure.TerminalCount != 1 || failure.Cause != "run-error" || failure.Map != 0x0f {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestFailureCircuitOpensOnSecondCanonicalOccurrence(t *testing.T) {
	db := newFailureCircuitTestDB(t)
	cp := &controlPlane{db: db}
	report := farm.FinishReport{
		RunID: "run-current", Attempt: 1, Reason: "stuck", Detail: "same blocker",
		RunnerVersion: "build-a", ProgressFinal: &farm.Progress{Badges: 5, Events: 20, Maps: 70, Map: 0x0f},
	}
	failure, ok := terminalRunFailure(report, nil)
	if !ok {
		t.Fatal("missing synthetic failure")
	}
	key, fingerprint, _, err := objectiveFailureFingerprint(failure)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO objective_failures(run_id,attempt,failure_key,fingerprint,blocking,terminal_count) VALUES(?,?,?,?,TRUE,1)`,
		"run-prior", 1, key, fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO run_attempts(run_id,attempt,reason,runner_version) VALUES(?,?,?,?)`,
		"run-prior", 1, "stuck", "build-a"); err != nil {
		t.Fatal(err)
	}

	decision, err := cp.failureCircuitForOccurrence(tileRow{Game: "pokemon-red", Planner: "llm", Goal: "champion"}, report, 1, failure)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Open || decision.Kind != "fingerprint" || decision.Count != 2 || decision.Key != key {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.Badges != 5 {
		t.Fatalf("badges = %d, want 5", decision.Badges)
	}
}

func TestFailureCircuitOpensOnThirdFailureAtSameBadgeFrontier(t *testing.T) {
	db := newFailureCircuitTestDB(t)
	cp := &controlPlane{db: db}
	scope := tileRow{Game: "pokemon-red", Planner: "llm", Goal: "champion"}
	for i, id := range []string{"run-a", "run-b"} {
		previous := farm.FinishReport{
			RunID: id, Attempt: 1, Reason: "stuck", Detail: "different blocker",
			RunnerVersion: "build-a", ProgressFinal: &farm.Progress{Badges: 5, Events: 10 + i, Maps: 60 + i},
		}
		reportRaw, _ := json.Marshal(previous)
		rowRaw, _ := json.Marshal(scope)
		if _, err := db.Exec(`INSERT INTO run_attempts(run_id,attempt,reason,runner_version,report_json) VALUES(?,?,?,?,?)`,
			id, 1, previous.Reason, previous.RunnerVersion, reportRaw); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO runs(run_id,row_json) VALUES(?,?)`, id, rowRaw); err != nil {
			t.Fatal(err)
		}
	}

	current := farm.FinishReport{
		RunID: "run-c", Attempt: 1, Reason: "stuck", Detail: "third distinct blocker",
		RunnerVersion: "build-a", ProgressFinal: &farm.Progress{Badges: 5, Events: 12, Maps: 62, Map: 0x0f},
	}
	failure, ok := terminalRunFailure(current, nil)
	if !ok {
		t.Fatal("missing synthetic current failure")
	}
	decision, err := cp.failureCircuitForOccurrence(scope, current, 1, failure)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Open || decision.Kind != "progression-frontier" || decision.Count != 3 || decision.Badges != 5 {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestObjectiveFailureTriageUsesCanonicalFailureRows(t *testing.T) {
	db := newFailureCircuitTestDB(t)
	cp := &controlPlane{db: db}
	failure := farm.ObjectiveFailure{
		Objective: "make progress toward run goal", Error: "stagnation watchdog stopped the run",
		Count: 1, TerminalCount: 1, Blocking: true, Map: 0x0f,
	}
	raw, _ := json.Marshal(failure)
	for _, id := range []string{"run-a", "run-b"} {
		if _, err := db.Exec(`INSERT INTO objective_failures(run_id,attempt,failure_key,fingerprint,family_key,family_fingerprint,blocking,terminal_count,failure_json) VALUES(?,?,?,?,?,?,TRUE,1,?)`,
			id, 1, "occurrence-key", "sha256:occurrence", "canonical-key", "sha256:canonical", raw); err != nil {
			t.Fatal(err)
		}
	}
	w := NewWall("")
	w.issueLinks["canonical-key"] = IssueLink{IssueID: "42", Status: "open", CircuitOpen: true}

	groups, err := cp.objectiveFailureTriage(w)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Key != "canonical-key" || groups[0].Count != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	if groups[0].Issue == nil || !groups[0].Issue.CircuitOpen {
		t.Fatalf("issue = %+v, want circuit-open canonical link", groups[0].Issue)
	}
	if groups[0].Dismissable {
		t.Fatal("linked group must not be dismissable")
	}
}

func TestDismissObjectiveFailureGroupHidesCurrentEvidenceAndAllowsRecurrence(t *testing.T) {
	db := newFailureCircuitTestDB(t)
	cp := &controlPlane{db: db}
	failure := farm.ObjectiveFailure{
		Objective: "make progress toward run goal", Error: "stagnation watchdog stopped the run",
		Count: 1, TerminalCount: 1, Blocking: true, Map: 0x12,
	}
	raw, _ := json.Marshal(failure)
	for _, id := range []string{"run-a", "run-b"} {
		if _, err := db.Exec(`INSERT INTO objective_failures(run_id,attempt,failure_key,fingerprint,family_key,family_fingerprint,blocking,terminal_count,failure_json) VALUES(?,?,?,?,?,?,TRUE,1,?)`,
			id, 1, "occurrence-"+id, "sha256:occurrence", "canonical-key", "sha256:canonical", raw); err != nil {
			t.Fatal(err)
		}
	}

	count, linked, err := cp.dismissObjectiveFailureGroup("canonical-key")
	if err != nil {
		t.Fatal(err)
	}
	if linked || count != 2 {
		t.Fatalf("dismiss = count %d linked %v, want 2 false", count, linked)
	}

	w := NewWall("")
	groups, err := cp.objectiveFailureTriage(w)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups after dismiss = %+v, want none", groups)
	}

	if _, err := db.Exec(`INSERT INTO objective_failures(run_id,attempt,failure_key,fingerprint,family_key,family_fingerprint,blocking,terminal_count,failure_json) VALUES(?,?,?,?,?,?,TRUE,1,?)`,
		"run-c", 1, "occurrence-run-c", "sha256:occurrence", "canonical-key", "sha256:canonical", raw); err != nil {
		t.Fatal(err)
	}
	groups, err = cp.objectiveFailureTriage(w)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Count != 1 || groups[0].Key != "canonical-key" || !groups[0].Dismissable {
		t.Fatalf("groups after recurrence = %+v", groups)
	}
}

func TestDismissObjectiveFailureGroupsBatchesAndSkipsLinked(t *testing.T) {
	db := newFailureCircuitTestDB(t)
	cp := &controlPlane{db: db}
	failure := farm.ObjectiveFailure{
		Objective: "advance story", Error: "blocked", Count: 1, TerminalCount: 1, Blocking: true,
	}
	raw, _ := json.Marshal(failure)
	for _, tc := range []struct {
		runID string
		key   string
	}{
		{runID: "run-a1", key: "group-a"},
		{runID: "run-a2", key: "group-a"},
		{runID: "run-b", key: "group-b"},
		{runID: "run-linked", key: "group-linked"},
	} {
		if _, err := db.Exec(`INSERT INTO objective_failures(run_id,attempt,failure_key,fingerprint,family_key,family_fingerprint,blocking,terminal_count,failure_json) VALUES(?,?,?,?,?,?,TRUE,1,?)`,
			tc.runID, 1, "occurrence-"+tc.runID, "sha256:occurrence", tc.key, "sha256:"+tc.key, raw); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO issue_links(failure_key,issue_id) VALUES(?,?)`, "group-linked", "42"); err != nil {
		t.Fatal(err)
	}

	result, err := cp.dismissObjectiveFailureGroups([]string{"group-a", "group-b", "group-a", "group-linked"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups != 2 || result.Occurrences != 3 {
		t.Fatalf("result = %+v, want 2 groups / 3 occurrences", result)
	}
	if len(result.SkippedLinked) != 1 || result.SkippedLinked[0] != "group-linked" {
		t.Fatalf("skipped = %v, want group-linked", result.SkippedLinked)
	}

	var dismissed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM objective_failures WHERE delivery_status='dismissed'`).Scan(&dismissed); err != nil {
		t.Fatal(err)
	}
	if dismissed != 3 {
		t.Fatalf("dismissed rows = %d, want 3", dismissed)
	}
}

func TestDismissObjectiveFailureGroupRefusesLinkedIssue(t *testing.T) {
	db := newFailureCircuitTestDB(t)
	cp := &controlPlane{db: db}
	failure := farm.ObjectiveFailure{
		Objective: "advance story", Error: "blocked", Count: 1, TerminalCount: 1, Blocking: true,
	}
	raw, _ := json.Marshal(failure)
	if _, err := db.Exec(`INSERT INTO objective_failures(run_id,attempt,failure_key,fingerprint,family_key,family_fingerprint,blocking,terminal_count,failure_json) VALUES(?,?,?,?,?,?,TRUE,1,?)`,
		"run-a", 1, "occurrence-key", "sha256:occurrence", "canonical-key", "sha256:canonical", raw); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO issue_links(failure_key,issue_id) VALUES(?,?)`, "canonical-key", "42"); err != nil {
		t.Fatal(err)
	}

	count, linked, err := cp.dismissObjectiveFailureGroup("canonical-key")
	if err != nil {
		t.Fatal(err)
	}
	if !linked || count != 0 {
		t.Fatalf("dismiss = count %d linked %v, want 0 true", count, linked)
	}

	var status string
	if err := db.QueryRow(`SELECT delivery_status FROM objective_failures WHERE run_id=?`, "run-a").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("delivery_status = %q, want pending", status)
	}
}

func TestFailureCircuitPausesQueuedRetryAndPreservesFrame(t *testing.T) {
	w := NewWall("")
	w.tiles["run-1"] = &Tile{
		RunID: "run-1", Status: statusQueued, Attempts: 1, ErrorAttempts: 1,
		Seed: 999, Frame: 0, Finished: false,
	}
	w.order = []string{"run-1"}
	w.queue = []string{"run-1"}
	before := pauseFinishSnapshot{
		ok:        true,
		row:       tileRow{RunID: "run-1", Seed: 42, Frame: 1234, Map: 9, X: 3, Y: 4},
		lastFrame: []byte{1, 2, 3},
	}
	report := farm.FinishReport{RunID: "run-1", Attempt: 1, Reason: "error", Detail: "blocked"}
	decision := failureCircuitDecision{
		Open: true, Kind: "fingerprint", Key: "deadbeef", Fingerprint: "sha256:deadbeef",
		Count: 2, Threshold: 2, Badges: 5, Revision: "build-a",
	}
	if !w.pauseForFailureCircuit("run-1", before, report, decision) {
		t.Fatal("circuit did not pause retry")
	}
	tile := w.tiles["run-1"]
	if tile.Status != statusPaused || !tile.Finished || tile.Frame != 1234 || tile.Seed != 42 {
		t.Fatalf("tile = %+v", tile)
	}
	if tile.CircuitKey != "deadbeef" || tile.CircuitCount != 2 {
		t.Fatalf("circuit = key %q count %d", tile.CircuitKey, tile.CircuitCount)
	}
	if len(w.queue) != 0 {
		t.Fatalf("queue = %v, want empty", w.queue)
	}
}

func TestFailureCircuitDoesNotPauseEndlessGoalCampaign(t *testing.T) {
	w := NewWall("")
	w.tiles["run-1"] = &Tile{
		RunID: "run-1", Status: statusQueued, Attempts: 3, ErrorAttempts: 3,
		Seed: 999, Frame: 0, Finished: false, Endless: true,
	}
	w.order = []string{"run-1"}
	w.queue = []string{"run-1"}
	before := pauseFinishSnapshot{
		ok:        true,
		row:       tileRow{RunID: "run-1", Seed: 42, Frame: 1234, Map: 9, X: 3, Y: 4, Endless: true},
		lastFrame: []byte{1, 2, 3},
	}
	report := farm.FinishReport{RunID: "run-1", Attempt: 3, Reason: "error", Detail: "blocked"}
	decision := failureCircuitDecision{
		Open: true, Kind: "fingerprint", Key: "deadbeef", Fingerprint: "sha256:deadbeef",
		Count: 4, Threshold: 2, Badges: 5, Revision: "build-a",
	}

	if w.pauseForFailureCircuit("run-1", before, report, decision) {
		t.Fatal("endless goal campaign was quarantined by the failure circuit")
	}
	tile := w.tiles["run-1"]
	if tile.Status != statusQueued || tile.Finished {
		t.Fatalf("tile = %+v, want active queued endless campaign", tile)
	}
	if len(w.queue) != 1 || w.queue[0] != "run-1" {
		t.Fatalf("queue = %v, want run-1 to remain recoverable", w.queue)
	}
	if tile.CircuitKey != "" {
		t.Fatalf("circuit key = %q, advisory circuit must not mutate endless tile", tile.CircuitKey)
	}
}

func TestEndlessGoalCampaignEscalatesExhaustedErrorBudgetToSuccessor(t *testing.T) {
	w := NewWall("")
	current := &Tile{
		RunID: "run-parent", Status: statusRunning, Endless: true,
		Game: "pokemon-red", Planner: "llm", Goal: "elite-four", Seed: 42,
	}
	w.tiles[current.RunID] = current
	w.order = []string{current.RunID}

	// The first two genuine errors stay inside the same run id.
	for attempt := 1; attempt < maxAttempts; attempt++ {
		w.settleRun(current, "error", "recoverable blocker", time.Now())
		if current.Finished || current.Status != statusQueued {
			t.Fatalf("attempt %d settled campaign: %+v", attempt, current)
		}
		w.queue = removeID(w.queue, current.RunID)
		current.Status = statusRunning
	}

	// Exhausting the local retry budget ends only this generation. Endless
	// supervision must immediately enqueue a new run that resumes from it.
	w.settleRun(current, "error", "recovery budget exhausted", time.Now())
	if !current.Finished || current.Status != statusDone {
		t.Fatalf("parent = %+v, want settled generation", current)
	}
	if len(w.queue) != 1 {
		t.Fatalf("queue = %v, want one successor", w.queue)
	}
	successor := w.tiles[w.queue[0]]
	if successor == nil {
		t.Fatalf("missing successor %q", w.queue[0])
	}
	if !successor.Endless || successor.Finished || successor.Status != statusQueued {
		t.Fatalf("successor = %+v, want active endless queued run", successor)
	}
	if successor.ResumeFromRunID != current.RunID {
		t.Fatalf("resume_from = %q, want %q", successor.ResumeFromRunID, current.RunID)
	}
	if successor.Goal != current.Goal || successor.Game != current.Game || successor.Planner != current.Planner {
		t.Fatalf("successor did not preserve goal config: parent=%+v successor=%+v", current, successor)
	}
}

func TestFixedCircuitReleasesOneCanaryAfterRunnerRollout(t *testing.T) {
	w := NewWall("")
	// The wall build is deliberately still the broken revision. Canary gating
	// must use runner versions, not the control-plane server version.
	w.Version = "build-broken"
	w.workers["runner-fixed"] = &workerInfo{
		Addrs: []string{"10.0.0.2:8099"}, Version: "build-fixed", LastSeen: time.Now(),
	}
	for _, id := range []string{"run-a", "run-b"} {
		w.tiles[id] = &Tile{
			RunID: id, Status: statusPaused, Finished: true, EndedAt: time.Now(),
			CircuitKey: "deadbeef", CircuitKind: "fingerprint", CircuitRevision: "build-broken",
			CircuitBadges: 5, Attempts: 2,
		}
		w.order = append(w.order, id)
	}
	w.issueLinks["deadbeef"] = IssueLink{
		IssueID: "1", Status: "resolved", Resolution: "fixed", FixedRevision: "build-fixed",
		CircuitOpen: true,
	}
	if !w.maybeResumeCircuitCanary("deadbeef", w.issueLinks["deadbeef"]) {
		t.Fatal("fixed circuit did not release a canary after runner rollout")
	}
	queued := 0
	paused := 0
	for _, tile := range w.tiles {
		switch tile.Status {
		case statusQueued:
			queued++
			if tile.CircuitKind != "canary" {
				t.Fatalf("queued circuit kind = %q", tile.CircuitKind)
			}
		case statusPaused:
			paused++
		}
	}
	if queued != 1 || paused != 1 {
		t.Fatalf("queued=%d paused=%d, want 1/1", queued, paused)
	}
	if w.maybeResumeCircuitCanary("deadbeef", w.issueLinks["deadbeef"]) {
		t.Fatal("second canary released while first is active")
	}
}

func TestFixedCircuitWaitsUntilBrokenRunnerBuildDrains(t *testing.T) {
	w := NewWall("")
	w.workers["runner-old"] = &workerInfo{
		Addrs: []string{"10.0.0.1:8099"}, Version: "build-broken", LastSeen: time.Now(),
	}
	w.workers["runner-new"] = &workerInfo{
		Addrs: []string{"10.0.0.2:8099"}, Version: "build-fixed", LastSeen: time.Now(),
	}
	w.tiles["run-a"] = &Tile{
		RunID: "run-a", Status: statusPaused, Finished: true, EndedAt: time.Now(),
		CircuitKey: "deadbeef", CircuitKind: "fingerprint", CircuitRevision: "build-broken",
		Attempts: 2,
	}
	w.order = []string{"run-a"}
	w.issueLinks["deadbeef"] = IssueLink{
		IssueID: "1", Status: "resolved", Resolution: "fixed", FixedRevision: "build-fixed",
		CircuitOpen: true,
	}

	if w.maybeResumeCircuitCanary("deadbeef", w.issueLinks["deadbeef"]) {
		t.Fatal("canary released while a broken-build runner was still live")
	}
	if got := w.tiles["run-a"].Status; got != statusPaused {
		t.Fatalf("status with mixed runner fleet = %q, want paused", got)
	}

	delete(w.workers, "runner-old")
	if !w.maybeResumeCircuitCanary("deadbeef", w.issueLinks["deadbeef"]) {
		t.Fatal("canary did not release after broken-build runner drained")
	}
}

func TestFixedCircuitWaitsForVersionedRunner(t *testing.T) {
	w := NewWall("")
	w.workers["runner-unknown"] = &workerInfo{
		Addrs: []string{"10.0.0.3:8099"}, LastSeen: time.Now(),
	}
	w.tiles["run-a"] = &Tile{
		RunID: "run-a", Status: statusPaused, Finished: true,
		CircuitKey: "deadbeef", CircuitKind: "repeat", CircuitRevision: "build-broken",
	}
	w.order = []string{"run-a"}
	w.issueLinks["deadbeef"] = IssueLink{
		IssueID: "1", Status: "resolved", Resolution: "fixed", FixedRevision: "build-fixed",
	}

	if w.maybeResumeCircuitCanary("deadbeef", w.issueLinks["deadbeef"]) {
		t.Fatal("canary released onto an unversioned runner")
	}
}

// TestFixedCircuitWithNoPausedTileClearsOrphanedFlag covers the reporter path
// (objective_failures.go's quarantine branch): CircuitOpen can be set purely
// for triage priority, from persisted failure counts, without ever pausing a
// run. Once the issue is fixed, that flag must not wait forever for a paused
// tile that was never created.
func TestFixedCircuitWithNoPausedTileClearsOrphanedFlag(t *testing.T) {
	w := NewWall("")
	w.issueLinks["deadbeef"] = IssueLink{
		IssueID: "1", Status: "resolved", Resolution: "fixed", FixedRevision: "build-fixed",
		CircuitOpen: true, CircuitKind: "fingerprint", CircuitCount: 16,
	}

	if w.maybeResumeCircuitCanary("deadbeef", w.issueLinks["deadbeef"]) {
		t.Fatal("no paused tile exists; nothing should have been released")
	}
	if w.issueLinks["deadbeef"].CircuitOpen {
		t.Fatal("orphaned CircuitOpen flag was not cleared once fixed with no paused tile")
	}
}

// TestFixedCircuitStillWaitingKeepsFlagWhilePausedTileExists is the negative
// case: a real paused tile exists but the runner fleet has not rolled off the
// broken build yet. The orphan-clearing path must not fire here.
func TestFixedCircuitStillWaitingKeepsFlagWhilePausedTileExists(t *testing.T) {
	w := NewWall("")
	w.workers["runner-old"] = &workerInfo{
		Addrs: []string{"10.0.0.1:8099"}, Version: "build-broken", LastSeen: time.Now(),
	}
	w.tiles["run-a"] = &Tile{
		RunID: "run-a", Status: statusPaused, Finished: true,
		CircuitKey: "deadbeef", CircuitKind: "fingerprint", CircuitRevision: "build-broken",
	}
	w.order = []string{"run-a"}
	w.issueLinks["deadbeef"] = IssueLink{
		IssueID: "1", Status: "resolved", Resolution: "fixed", FixedRevision: "build-fixed",
		CircuitOpen: true,
	}

	if w.maybeResumeCircuitCanary("deadbeef", w.issueLinks["deadbeef"]) {
		t.Fatal("canary released while a broken-build runner was still live")
	}
	if !w.issueLinks["deadbeef"].CircuitOpen {
		t.Fatal("CircuitOpen was cleared while a real paused tile is still waiting on the fleet")
	}
}
