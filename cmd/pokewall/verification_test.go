package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func TestIssueVerificationCountsOnlyRunsPastFailureProgress(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	baseTime := time.Unix(1_800_000_000, 0)
	detail := "still on map 0x0c at (10,35)"
	key, fp := failureIdentity(normalizeDetail(detail))

	baseline := &Tile{
		RunID: "run-baseline", Planner: "llm", Starter: "squirtle", Goal: "badges:3",
		QueuedAt: baseTime.Add(-2 * time.Hour), EndedAt: baseTime.Add(-time.Hour),
		Attempts: 1, Finished: true, Status: statusDone, Reason: "error", Detail: detail,
	}
	w.mu.Lock()
	w.order = append(w.order, baseline.RunID)
	w.tiles[baseline.RunID] = baseline
	w.issueLinks[key] = IssueLink{
		IssueID: "issue-1", IssueNumber: 1, Status: "resolved", Resolution: "fixed",
		OccurrenceCount: 3, FixedRevision: "build-fixed", LastObservedRun: baseline.RunID,
		Fingerprint: fp,
	}
	w.mu.Unlock()
	writeVerificationDump(t, dir, baseline.RunID, farm.FinishReport{
		RunID: baseline.RunID, Attempt: 1, Reason: "error", Detail: detail, RunnerVersion: "build-old",
		ProgressFinal: &farm.Progress{Badges: 2, Events: 20, Maps: 8},
	})

	w.refreshIssueVerifications(baseTime)
	link := verificationLink(t, w, key)
	if link.VerificationState != verificationVerifying || !link.VerificationHasBaseline {
		t.Fatalf("started verification = %+v", link)
	}
	if link.VerificationBaselineBadges != 2 || link.VerificationBaselineEvents != 20 || link.VerificationBaselineMaps != 8 {
		t.Fatalf("baseline = badges %d events %d maps %d", link.VerificationBaselineBadges, link.VerificationBaselineEvents, link.VerificationBaselineMaps)
	}

	irrelevant := &Tile{
		RunID: "run-irrelevant", Planner: "llm", Starter: "squirtle", Goal: "badges:3",
		QueuedAt: baseTime.Add(time.Minute), EndedAt: baseTime.Add(20 * time.Minute),
		Attempts: 1, Finished: true, Status: statusDone, Reason: "budget",
	}
	relevant := &Tile{
		RunID: "run-relevant", Planner: "llm", Starter: "squirtle", Goal: "badges:3",
		QueuedAt: baseTime.Add(2 * time.Minute), EndedAt: baseTime.Add(30 * time.Minute),
		Attempts: 1, Finished: true, Status: statusDone, Reason: "budget",
	}
	staleWorker := &Tile{
		RunID: "run-stale-worker", Planner: "llm", Starter: "squirtle", Goal: "badges:3",
		QueuedAt: baseTime.Add(3 * time.Minute), EndedAt: baseTime.Add(40 * time.Minute),
		Attempts: 1, Finished: true, Status: statusDone, Reason: "budget",
	}
	w.mu.Lock()
	w.order = append(w.order, irrelevant.RunID, relevant.RunID, staleWorker.RunID)
	w.tiles[irrelevant.RunID] = irrelevant
	w.tiles[relevant.RunID] = relevant
	w.tiles[staleWorker.RunID] = staleWorker
	w.mu.Unlock()
	writeVerificationDump(t, dir, irrelevant.RunID, farm.FinishReport{
		RunID: irrelevant.RunID, Attempt: 1, Reason: "budget", RunnerVersion: "build-fixed",
		ProgressFinal: &farm.Progress{Badges: 2, Events: 20, Maps: 8},
	})
	writeVerificationDump(t, dir, relevant.RunID, farm.FinishReport{
		RunID: relevant.RunID, Attempt: 1, Reason: "budget", RunnerVersion: "build-fixed",
		ProgressFinal: &farm.Progress{Badges: 2, Events: 21, Maps: 8},
	})
	writeVerificationDump(t, dir, staleWorker.RunID, farm.FinishReport{
		RunID: staleWorker.RunID, Attempt: 1, Reason: "budget", RunnerVersion: "build-old",
		ProgressFinal: &farm.Progress{Badges: 3, Events: 25, Maps: 10},
	})

	w.refreshIssueVerifications(baseTime.Add(time.Hour))
	link = verificationLink(t, w, key)
	if link.VerificationCleanRuns != 1 || link.VerificationLastCleanRun != relevant.RunID {
		t.Fatalf("clean verification runs = %+v", link)
	}
}

func TestIssueVerificationBecomesVerifiedAfterTenRelevantRunsAndQuietWindow(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	start := time.Unix(1_800_100_000, 0)
	detail := "failed to leave route 25 at (8,4)"
	key, fp := failureIdentity(normalizeDetail(detail))
	baseline := &Tile{
		RunID: "run-base", Planner: "llm", Goal: "Beat the Elite Four and Champion.",
		QueuedAt: start.Add(-time.Hour), EndedAt: start.Add(-30 * time.Minute), Attempts: 1,
		Finished: true, Status: statusDone, Reason: "error", Detail: detail,
	}
	w.mu.Lock()
	w.order = append(w.order, baseline.RunID)
	w.tiles[baseline.RunID] = baseline
	w.issueLinks[key] = IssueLink{
		IssueID: "issue-2", Status: "resolved", Resolution: "fixed", FixedRevision: "fixed-2",
		OccurrenceCount: 9, LastObservedRun: baseline.RunID, Fingerprint: fp,
	}
	w.mu.Unlock()
	writeVerificationDump(t, dir, baseline.RunID, farm.FinishReport{
		RunID: baseline.RunID, Attempt: 1, Reason: "error", Detail: detail, RunnerVersion: "old",
		ProgressFinal: &farm.Progress{Badges: 3, Events: 30, Maps: 12},
	})
	w.refreshIssueVerifications(start)

	for i := 0; i < defaultIssueVerificationRuns; i++ {
		id := "run-clean-" + itoa(i+1)
		tile := &Tile{
			RunID: id, Planner: "llm", Goal: baseline.Goal,
			QueuedAt: start.Add(time.Duration(i+1) * time.Minute),
			EndedAt:  start.Add(time.Duration(i+1) * time.Minute).Add(20 * time.Minute),
			Attempts: 1, Finished: true, Status: statusDone, Reason: "budget",
		}
		w.mu.Lock()
		w.order = append(w.order, id)
		w.tiles[id] = tile
		w.mu.Unlock()
		writeVerificationDump(t, dir, id, farm.FinishReport{
			RunID: id, Attempt: 1, Reason: "budget", RunnerVersion: "fixed-2",
			ProgressFinal: &farm.Progress{Badges: 4, Events: 31 + i, Maps: 13},
		})
	}

	w.refreshIssueVerifications(start.Add(5 * time.Hour))
	link := verificationLink(t, w, key)
	if link.VerificationState != verificationVerifying || link.VerificationCleanRuns != defaultIssueVerificationRuns {
		t.Fatalf("before quiet window = %+v", link)
	}
	w.refreshIssueVerifications(start.Add(7 * time.Hour))
	link = verificationLink(t, w, key)
	if link.VerificationState != verificationVerified || link.VerificationVerifiedAt == 0 {
		t.Fatalf("verified = %+v", link)
	}
}

func TestIssueVerificationRegressionBlocksSameRevision(t *testing.T) {
	w := NewWall(t.TempDir())
	now := time.Unix(1_800_200_000, 0)
	key := "0123456789abcdef"
	w.mu.Lock()
	w.issueLinks[key] = IssueLink{
		IssueID: "issue-3", Status: "reopened", Resolution: "fixed", FixedRevision: "bad-fix",
		VerificationState: verificationVerified, VerificationRevision: "bad-fix",
		VerificationStartedAt: now.Add(-24 * time.Hour).Unix(), VerificationCleanRuns: 10,
	}
	w.mu.Unlock()

	w.refreshIssueVerifications(now)
	link := verificationLink(t, w, key)
	if link.VerificationState != verificationRegressed || link.VerificationCleanRuns != 0 || link.VerificationVerifiedAt != 0 {
		t.Fatalf("regressed = %+v", link)
	}

	// A stale fixed status with the same revision must not silently restart the
	// verification window. Only a new FixedRevision earns a new window.
	w.mu.Lock()
	link.Status = "resolved"
	w.issueLinks[key] = link
	w.mu.Unlock()
	w.refreshIssueVerifications(now.Add(time.Hour))
	if got := verificationLink(t, w, key); got.VerificationState != verificationRegressed {
		t.Fatalf("same revision restarted verification: %+v", got)
	}

	w.mu.Lock()
	link = w.issueLinks[key]
	link.FixedRevision = "good-fix"
	w.issueLinks[key] = link
	w.mu.Unlock()
	w.refreshIssueVerifications(now.Add(2 * time.Hour))
	if got := verificationLink(t, w, key); got.VerificationState != verificationVerifying || got.VerificationRevision != "good-fix" || got.VerificationCleanRuns != 0 {
		t.Fatalf("new revision did not restart verification: %+v", got)
	}
}

func TestIssueVerificationMatchingFailureRegressesBeforeStatusSync(t *testing.T) {
	dir := t.TempDir()
	w := NewWall(dir)
	start := time.Unix(1_800_300_000, 0)
	detail := "still on map 0x21 at (4,22)"
	key, fp := failureIdentity(normalizeDetail(detail))
	w.mu.Lock()
	w.issueLinks[key] = IssueLink{
		IssueID: "issue-4", Status: "resolved", Resolution: "fixed", FixedRevision: "fix-4",
		Fingerprint: fp, VerificationState: verificationVerifying, VerificationRevision: "fix-4",
		VerificationStartedAt: start.Unix(), VerificationRequiredRuns: 10,
		VerificationQuietSeconds: int64(defaultIssueVerificationQuiet / time.Second),
	}
	tile := &Tile{
		RunID: "run-regression", Planner: "llm", QueuedAt: start.Add(time.Minute), EndedAt: start.Add(time.Hour),
		Attempts: 1, Finished: true, Status: statusDone, Reason: "error", Detail: detail,
	}
	w.order = append(w.order, tile.RunID)
	w.tiles[tile.RunID] = tile
	w.mu.Unlock()
	writeVerificationDump(t, dir, tile.RunID, farm.FinishReport{
		RunID: tile.RunID, Attempt: 1, Reason: "error", Detail: detail, RunnerVersion: "fix-4",
	})

	w.refreshIssueVerifications(start.Add(2 * time.Hour))
	if got := verificationLink(t, w, key); got.VerificationState != verificationRegressed {
		t.Fatalf("matching recurrence = %+v", got)
	}
}

func writeVerificationDump(t *testing.T, dir, runID string, report farm.FinishReport) {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeDumpName(runID)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func verificationLink(t *testing.T, w *Wall, key string) IssueLink {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.issueLinks[key]
}
