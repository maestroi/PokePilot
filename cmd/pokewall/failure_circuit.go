package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const (
	failureCircuitRepeatThreshold   = 2
	failureCircuitFrontierThreshold = 3
	failureCircuitFrontierLookback  = 64
)

type finishReportContextKey struct{}

type failureCircuitDecision struct {
	Open        bool   `json:"open"`
	Kind        string `json:"kind,omitempty"`
	Key         string `json:"key,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Count       int    `json:"count,omitempty"`
	Threshold   int    `json:"threshold,omitempty"`
	Badges      int    `json:"badges,omitempty"`
	Events      int    `json:"events,omitempty"`
	Maps        int    `json:"maps,omitempty"`
	Revision    string `json:"revision,omitempty"`
}

func withFinishReport(req *http.Request, report farm.FinishReport) *http.Request {
	copy := report
	return req.WithContext(context.WithValue(req.Context(), finishReportContextKey{}, &copy))
}

func finishReportFromRequest(req *http.Request) (farm.FinishReport, bool) {
	report, ok := req.Context().Value(finishReportContextKey{}).(*farm.FinishReport)
	if !ok || report == nil {
		return farm.FinishReport{}, false
	}
	return *report, true
}

func circuitObjectiveFailures(report farm.FinishReport) ([]farm.ObjectiveFailure, error) {
	failures, err := farm.DecodeObjectiveFailures(report)
	if err != nil {
		return nil, err
	}
	if synthetic, ok := terminalRunFailure(report, failures); ok {
		failures = append(failures, synthetic)
	}
	return failures, nil
}

func circuitFailureEligible(f farm.ObjectiveFailure) bool {
	return f.Blocking || f.TerminalCount > 0
}

func (cp *controlPlane) failureCircuitForOccurrence(scope tileRow, report farm.FinishReport, attempt int, failure farm.ObjectiveFailure) (failureCircuitDecision, error) {
	if cp == nil || !circuitFailureEligible(failure) {
		return failureCircuitDecision{}, nil
	}
	if attempt <= 0 {
		attempt = report.Attempt
	}
	if attempt <= 0 {
		attempt = 1
	}
	key, fingerprint, _, err := objectiveFailureFingerprint(failure)
	if err != nil {
		return failureCircuitDecision{}, err
	}
	revision := strings.TrimSpace(report.RunnerVersion)
	priorTotal, priorSameRevision, err := cp.failureFingerprintCounts(fingerprint, revision, report.RunID, attempt)
	if err != nil {
		return failureCircuitDecision{}, err
	}
	count := priorSameRevision + 1
	if priorTotal+1 > count {
		// A recurrence of a fingerprint already proven on older revisions is
		// still a repeated blocker. In particular, the first post-fix sighting
		// should stop the campaign immediately rather than burn another run.
		count = priorTotal + 1
	}
	if count >= failureCircuitRepeatThreshold {
		return circuitDecisionFromProgress(failureCircuitDecision{
			Open: true, Kind: "fingerprint", Key: key, Fingerprint: fingerprint,
			Count: count, Threshold: failureCircuitRepeatThreshold, Revision: revision,
		}, report.ProgressFinal), nil
	}

	frontierCount, err := cp.failureFrontierCount(scope, report, attempt)
	if err != nil {
		return failureCircuitDecision{}, err
	}
	if frontierCount >= failureCircuitFrontierThreshold {
		return circuitDecisionFromProgress(failureCircuitDecision{
			Open: true, Kind: "progression-frontier", Key: key, Fingerprint: fingerprint,
			Count: frontierCount, Threshold: failureCircuitFrontierThreshold, Revision: revision,
		}, report.ProgressFinal), nil
	}
	return failureCircuitDecision{}, nil
}

func (cp *controlPlane) failureCircuitDecision(scope tileRow, report farm.FinishReport, attempt int) (failureCircuitDecision, error) {
	failures, err := circuitObjectiveFailures(report)
	if err != nil {
		return failureCircuitDecision{}, err
	}
	// Exact repeated fingerprints are the strongest signal. Evaluate every
	// terminal/blocking failure before considering the broader badge frontier.
	for _, failure := range failures {
		if !circuitFailureEligible(failure) {
			continue
		}
		key, fingerprint, _, err := objectiveFailureFingerprint(failure)
		if err != nil {
			return failureCircuitDecision{}, err
		}
		revision := strings.TrimSpace(report.RunnerVersion)
		total, sameRevision, err := cp.failureFingerprintCounts(fingerprint, revision, report.RunID, max(1, attempt))
		if err != nil {
			return failureCircuitDecision{}, err
		}
		count := sameRevision + 1
		if total+1 > count {
			count = total + 1
		}
		if count >= failureCircuitRepeatThreshold {
			return circuitDecisionFromProgress(failureCircuitDecision{
				Open: true, Kind: "fingerprint", Key: key, Fingerprint: fingerprint,
				Count: count, Threshold: failureCircuitRepeatThreshold, Revision: revision,
			}, report.ProgressFinal), nil
		}
	}

	frontierCount, err := cp.failureFrontierCount(scope, report, attempt)
	if err != nil || frontierCount < failureCircuitFrontierThreshold {
		return failureCircuitDecision{}, err
	}
	for _, failure := range failures {
		if !circuitFailureEligible(failure) {
			continue
		}
		key, fingerprint, _, err := objectiveFailureFingerprint(failure)
		if err != nil {
			return failureCircuitDecision{}, err
		}
		return circuitDecisionFromProgress(failureCircuitDecision{
			Open: true, Kind: "progression-frontier", Key: key, Fingerprint: fingerprint,
			Count: frontierCount, Threshold: failureCircuitFrontierThreshold,
			Revision: strings.TrimSpace(report.RunnerVersion),
		}, report.ProgressFinal), nil
	}
	return failureCircuitDecision{}, nil
}

func circuitDecisionFromProgress(decision failureCircuitDecision, progress *farm.Progress) failureCircuitDecision {
	if progress != nil {
		decision.Badges = progress.Badges
		decision.Events = progress.Events
		decision.Maps = progress.Maps
	}
	return decision
}

func (cp *controlPlane) failureFingerprintCounts(fingerprint, revision, currentRun string, currentAttempt int) (total, sameRevision int, err error) {
	rows, err := cp.db.Query(`
SELECT f.run_id, f.attempt, COALESCE(a.runner_version, '')
FROM objective_failures f
LEFT JOIN run_attempts a ON a.run_id=f.run_id AND a.attempt=f.attempt
WHERE f.fingerprint=? AND (f.blocking=TRUE OR f.terminal_count>0)
ORDER BY f.updated_at DESC`, fingerprint)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var runID, observedRevision string
		var attempt int
		if err := rows.Scan(&runID, &attempt, &observedRevision); err != nil {
			return 0, 0, err
		}
		if runID == currentRun && attempt == currentAttempt {
			continue
		}
		id := fmt.Sprintf("%s/%d", runID, attempt)
		if seen[id] {
			continue
		}
		seen[id] = true
		total++
		if revision == "" || strings.TrimSpace(observedRevision) == revision {
			sameRevision++
		}
	}
	return total, sameRevision, rows.Err()
}

func sameFailureCircuitScope(a, b tileRow) bool {
	return strings.TrimSpace(a.Game) == strings.TrimSpace(b.Game) &&
		strings.TrimSpace(a.Planner) == strings.TrimSpace(b.Planner) &&
		strings.TrimSpace(a.Goal) == strings.TrimSpace(b.Goal)
}

func (cp *controlPlane) failureFrontierCount(scope tileRow, report farm.FinishReport, currentAttempt int) (int, error) {
	if report.ProgressFinal == nil {
		return 0, nil
	}
	revision := strings.TrimSpace(report.RunnerVersion)
	if revision == "" {
		return 0, nil
	}
	badges := report.ProgressFinal.Badges
	count := 1
	rows, err := cp.db.Query(`
SELECT a.run_id, a.attempt, a.report_json, r.row_json
FROM run_attempts a
JOIN runs r ON r.run_id=a.run_id
WHERE a.runner_version=? AND a.reason IN ('error','failed','stuck')
ORDER BY a.finished_at DESC
LIMIT ?`, revision, failureCircuitFrontierLookback)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var runID string
		var attempt int
		var reportRaw, rowRaw []byte
		if err := rows.Scan(&runID, &attempt, &reportRaw, &rowRaw); err != nil {
			return 0, err
		}
		if runID == report.RunID && attempt == currentAttempt {
			continue
		}
		id := fmt.Sprintf("%s/%d", runID, attempt)
		if seen[id] {
			continue
		}
		seen[id] = true
		var previous farm.FinishReport
		var previousScope tileRow
		if json.Unmarshal(reportRaw, &previous) != nil || json.Unmarshal(rowRaw, &previousScope) != nil {
			continue
		}
		if previous.ProgressFinal == nil || previous.ProgressFinal.Badges != badges || !sameFailureCircuitScope(scope, previousScope) {
			continue
		}
		count++
		if count >= failureCircuitFrontierThreshold {
			return count, nil
		}
	}
	return count, rows.Err()
}

func (w *Wall) circuitScopeForRun(runID string) (tileRow, bool) {
	if row, ok := w.ramRow(runID); ok {
		return row, true
	}
	if catalog := catalogFor(w); catalog != nil {
		row, ok, err := catalog.get(runID)
		if err == nil && ok {
			return row, true
		}
	}
	return tileRow{}, false
}

func circuitPauseNote(decision failureCircuitDecision) string {
	switch decision.Kind {
	case "progression-frontier":
		return fmt.Sprintf("circuit open after %d terminal failures at badge %d [triage:%s]", decision.Count, decision.Badges, decision.Key)
	default:
		return fmt.Sprintf("circuit open after %d matching failures [triage:%s]", decision.Count, decision.Key)
	}
}

func setTileCircuit(t *Tile, decision failureCircuitDecision) {
	t.CircuitKey = decision.Key
	t.CircuitFingerprint = decision.Fingerprint
	t.CircuitKind = decision.Kind
	t.CircuitCount = decision.Count
	t.CircuitBadges = decision.Badges
	t.CircuitEvents = decision.Events
	t.CircuitMaps = decision.Maps
	t.CircuitRevision = decision.Revision
}

func clearTileCircuit(t *Tile) {
	t.CircuitKey = ""
	t.CircuitFingerprint = ""
	t.CircuitKind = ""
	t.CircuitCount = 0
	t.CircuitBadges = 0
	t.CircuitEvents = 0
	t.CircuitMaps = 0
	t.CircuitRevision = ""
}

func (w *Wall) pauseForFailureCircuit(id string, before pauseFinishSnapshot, report farm.FinishReport, decision failureCircuitDecision) bool {
	if !decision.Open || !before.ok {
		return false
	}
	now := time.Now()
	w.mu.Lock()
	current := w.tiles[id]
	if current == nil {
		w.mu.Unlock()
		return false
	}
	// Endless is the resilient goal-supervisor mode. A repeated blocker is
	// still persisted, grouped, reported, and investigated, but it must not
	// quarantine the campaign: settleRun already bounds retries for one runner
	// generation and then enqueueNextLocked resumes a successor from the latest
	// major checkpoint. Keeping the circuit advisory here means weak/experimental
	// planners can need many recoveries without turning one bad objective into
	// a permanently stopped goal.
	//
	// Non-endless runs retain the strict circuit behavior used by qualification
	// and debugging: repeated deterministic blockers pause until a fixed-build
	// canary is available.
	if current.Endless {
		w.mu.Unlock()
		return false
	}

	target := current
	restoreCurrent := current.Status == statusQueued && !current.Finished
	if current.Finished && current.Status == statusDone && current.Endless {
		for _, queuedID := range w.queue {
			candidate := w.tiles[queuedID]
			if candidate != nil && candidate.Status == statusQueued && !candidate.Finished && candidate.ResumeFromRunID == id {
				target = candidate
				restoreCurrent = false
				break
			}
		}
	}
	if target.Status == statusPaused {
		w.mu.Unlock()
		return true
	}

	w.queue = removeID(w.queue, target.RunID)
	target.Status = statusPaused
	target.Finished = true
	target.EndedAt = now
	target.StopSoFar = circuitPauseNote(decision)
	target.lastUpdate = now
	setTileCircuit(target, decision)
	delete(w.cancel, target.RunID)

	if target == current {
		target.Reason = report.Reason
		target.Detail = report.Detail
	}
	if restoreCurrent && target == current {
		// settleRun clears live state before requeueing an error. Restore the
		// pre-finish snapshot so the paused card and resume checkpoint still
		// describe the actual blocker.
		target.Seed = before.row.Seed
		target.Frame = before.row.Frame
		target.Map = before.row.Map
		target.X = before.row.X
		target.Y = before.row.Y
		target.Trace = before.row.Trace
		target.Question = before.row.Question
		target.Decision = before.row.Decision
		target.Raw = before.row.Raw
		target.Sprites = append(target.Sprites[:0], before.row.Sprites...)
		target.Trail = append(target.Trail[:0], before.row.Trail...)
		target.Stats = before.row.Stats
		target.Player = before.row.Player
		target.lastFrame = append(target.lastFrame[:0], before.lastFrame...)
	}
	w.mu.Unlock()
	w.saveState()
	return true
}

func circuitCanaryAdvanced(before tileRow, report farm.FinishReport) bool {
	if before.CircuitKey == "" || before.CircuitKind != "canary" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(report.Reason), "done") {
		return true
	}
	p := report.ProgressFinal
	if p == nil {
		return false
	}
	if p.Badges > before.CircuitBadges {
		return true
	}
	if p.Badges < before.CircuitBadges {
		return false
	}
	if p.Events > before.CircuitEvents {
		return true
	}
	if p.Events < before.CircuitEvents {
		return false
	}
	return p.Maps > before.CircuitMaps
}

func (w *Wall) releaseCircuitPeers(key, completedCanary string) int {
	if key == "" {
		return 0
	}
	now := time.Now()
	released := 0
	w.mu.Lock()
	if current := w.tiles[completedCanary]; current != nil {
		clearTileCircuit(current)
	}
	for _, id := range w.order {
		t := w.tiles[id]
		if t == nil || t.RunID == completedCanary || t.Status != statusPaused || t.CircuitKey != key {
			continue
		}
		t.Status = statusQueued
		t.Finished = false
		t.EndedAt = time.Time{}
		t.Reason = ""
		t.StopSoFar = "circuit canary advanced; resumed"
		if t.Attempts > 0 {
			t.Detail = fmt.Sprintf("attempt %d failed: no heartbeat for circuit resume", t.Attempts)
		} else {
			t.Detail = ""
		}
		t.ErrorAttempts = 0
		t.LossRecoveries = 0
		t.workerAddrs = nil
		t.lastUpdate = now
		delete(w.cancel, id)
		w.queue = removeID(w.queue, id)
		w.queue = append(w.queue, id)
		clearTileCircuit(t)
		released++
	}
	w.mu.Unlock()
	if released > 0 {
		w.saveState()
	}
	return released
}

// circuitWorkersReadyLocked reports whether the live runner fleet has fully
// rolled away from the build that opened a circuit. Requiring every currently
// visible worker to report a version different from the broken revision keeps
// a released canary from being leased by an old worker during a rolling deploy.
// Caller holds w.mu.
func (w *Wall) circuitWorkersReadyLocked(brokenRevision string) bool {
	brokenRevision = strings.TrimSpace(brokenRevision)
	if brokenRevision == "" || len(w.workers) == 0 {
		return false
	}
	ready := false
	for _, worker := range w.workers {
		if worker == nil {
			continue
		}
		version := strings.TrimSpace(worker.Version)
		if version == "" || version == brokenRevision {
			return false
		}
		ready = true
	}
	return ready
}

func (w *Wall) maybeResumeCircuitCanary(key string, link IssueLink) bool {
	if key == "" || !issueFixedForVerification(link) {
		return false
	}
	now := time.Now()
	w.mu.Lock()
	for _, t := range w.tiles {
		if t != nil && t.CircuitKey == key && t.CircuitKind == "canary" && !t.Finished {
			w.mu.Unlock()
			return false
		}
	}
	var target *Tile
	foundPaused := false
	for _, id := range w.order {
		t := w.tiles[id]
		if t == nil || t.Status != statusPaused || t.CircuitKey != key {
			continue
		}
		foundPaused = true
		// CircuitRevision is the runner build that reproduced the blocker. Wait
		// until the whole visible runner fleet has rolled off that build; using
		// the wall server's own version here can release a canary too early.
		if !w.circuitWorkersReadyLocked(t.CircuitRevision) {
			continue
		}
		target = t
		break
	}
	if target == nil {
		// The issue reporter can mark CircuitOpen for triage priority (a repeat
		// fingerprint crossing threshold) without ever pausing a run: that
		// happens asynchronously from persisted failure counts, not from a live
		// tile it holds. If the fix landed and no tile is actually paused on
		// this key, there is nothing to release; clear the flag instead of
		// retrying forever against a paused tile that will never appear.
		if !foundPaused {
			if current, ok := w.issueLinks[key]; ok && current.CircuitOpen {
				current.CircuitOpen = false
				w.issueLinks[key] = current
				w.mu.Unlock()
				w.saveState()
				return false
			}
		}
		w.mu.Unlock()
		return false
	}
	target.Status = statusQueued
	target.Finished = false
	target.EndedAt = time.Time{}
	target.Reason = ""
	target.StopSoFar = "circuit fix deployed; canary resume"
	if target.Attempts > 0 {
		target.Detail = fmt.Sprintf("attempt %d failed: no heartbeat for circuit canary", target.Attempts)
	} else {
		target.Detail = ""
	}
	target.ErrorAttempts = 0
	target.LossRecoveries = 0
	target.workerAddrs = nil
	target.lastUpdate = now
	target.CircuitKind = "canary"
	delete(w.cancel, target.RunID)
	w.queue = removeID(w.queue, target.RunID)
	w.queue = append(w.queue, target.RunID)
	if current, ok := w.issueLinks[key]; ok {
		current.CircuitOpen = false
		w.issueLinks[key] = current
	}
	runID := target.RunID
	w.mu.Unlock()
	clearFailureStreak(w, runID)
	w.saveState()
	return true
}

func applyCircuitToIssueLink(link IssueLink, runID string, decision failureCircuitDecision) IssueLink {
	if !decision.Open {
		return link
	}
	link.CircuitOpen = true
	link.CircuitKind = decision.Kind
	link.CircuitCount = decision.Count
	link.CircuitRunID = runID
	link.CircuitOpenedAt = time.Now().Unix()
	return link
}

func (w *Wall) noteCircuitIssue(key, runID string, decision failureCircuitDecision) {
	if !decision.Open || key == "" {
		return
	}
	w.mu.Lock()
	// Keep circuit metadata even when the issue reporter has not created the
	// remote issue yet. Reporters merge into this placeholder later, so the
	// circuit can still be prioritized and auto-resumed once the issue is fixed.
	link := w.issueLinks[key]
	w.issueLinks[key] = applyCircuitToIssueLink(link, runID, decision)
	w.mu.Unlock()
}

func (w *Wall) requestCircuitInvestigation(c *issueClient, issueID string, decision failureCircuitDecision) error {
	if c == nil || strings.TrimSpace(issueID) == "" || !decision.Open {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultIssueTimeout)
	defer cancel()
	if err := c.Investigate(ctx, issueID); err != nil {
		if isRetryableIssueError(err) {
			return err
		}
		log.Printf("pokewall: circuit investigation issue %s: %v", issueID, err)
	}
	return nil
}

// objectiveFailureTriage exposes the same canonical identities used by issue
// reporting and the circuit breaker. This is the production queue consumed by
// unattended coding agents; unlike the legacy detail grouper it includes
// terminal stuck/failed failures and survives wall restarts.
func (cp *controlPlane) objectiveFailureTriage(w *Wall) ([]triageGroup, error) {
	rows, err := cp.db.Query(`
SELECT failure_key, fingerprint, run_id, failure_json
FROM objective_failures
WHERE blocking=TRUE OR terminal_count>0
ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type acc struct {
		fingerprint string
		pattern     string
		example     string
		count       int
		runIDs      []string
		seenRuns    map[string]bool
	}
	groups := map[string]*acc{}
	for rows.Next() {
		var key, fingerprint, runID string
		var raw []byte
		if err := rows.Scan(&key, &fingerprint, &runID, &raw); err != nil {
			return nil, err
		}
		var failure farm.ObjectiveFailure
		if json.Unmarshal(raw, &failure) != nil {
			continue
		}
		group := groups[key]
		if group == nil {
			group = &acc{
				fingerprint: fingerprint,
				pattern:     objectiveFailurePattern(failure),
				example:     strings.TrimSpace(failure.Objective + ": " + failure.Error),
				seenRuns:    map[string]bool{},
			}
			groups[key] = group
		}
		group.count++
		if !group.seenRuns[runID] && len(group.runIDs) < triageRunIDCap {
			group.seenRuns[runID] = true
			group.runIDs = append(group.runIDs, runID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]triageGroup, 0, len(groups))
	for key, group := range groups {
		item := triageGroup{
			Pattern: group.pattern, Key: key, Fingerprint: group.fingerprint,
			Count: group.count, Example: group.example, RunIDs: group.runIDs,
			Outbox: outboxStatusForKey(w.outbox, key),
		}
		if link, ok := w.issueLinks[key]; ok && link.IssueID != "" {
			copy := link
			item.Issue = &copy
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		leftCircuit := out[i].Issue != nil && out[i].Issue.CircuitOpen
		rightCircuit := out[j].Issue != nil && out[j].Issue.CircuitOpen
		if leftCircuit != rightCircuit {
			return leftCircuit
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}
