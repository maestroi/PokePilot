package main

import (
	"context"
	"database/sql"
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
	// Repeated failure families are the strongest signal. Evaluate every
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
WHERE COALESCE(NULLIF(f.family_fingerprint,''), f.fingerprint)=? AND (f.blocking=TRUE OR f.terminal_count>0)
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
	if before.row.RecoveryProfile.Resilient() {
		// A circuit is still valuable evidence in resilient mode, but it is an
		// escalation signal rather than a stop signal. Keep the queued retry
		// alive, attach the fingerprint/progress metadata, and let a newly
		// deployed runner naturally pick up the next attempt.
		w.mu.Lock()
		if current := w.tiles[id]; current != nil && !current.Finished {
			setTileCircuit(current, decision)
			current.StopSoFar = fmt.Sprintf("goal recovery %d; %s", current.RecoveryAttempts, circuitPauseNote(decision))
			current.lastUpdate = time.Now()
			appendRunActivityLocked(current, runActivityEvent{
				Source: "recovery", Kind: "circuit", At: current.lastUpdate.Unix(),
				RecoveryAttempt: current.RecoveryAttempts,
				Summary:         "Repeated failure detected",
				Detail:          circuitPauseNote(decision),
			})
		}
		w.mu.Unlock()
		w.noteCircuitIssue(decision.Key, id, decision)
		w.saveState()
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
		t.RecoveryAttempts = 0
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
	target.RecoveryAttempts = 0
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
SELECT failure_key, fingerprint, family_key, family_fingerprint, run_id, failure_json
FROM objective_failures
WHERE (blocking=TRUE OR terminal_count>0)
  AND delivery_status<>'dismissed'
ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type acc struct {
		fingerprint   string
		pattern       string
		example       string
		count         int
		runIDs        []string
		seenRuns      map[string]bool
		issueKeys     []string
		seenIssueKeys map[string]bool
	}
	groups := map[string]*acc{}
	for rows.Next() {
		var occurrenceKey, occurrenceFingerprint, familyKey, familyFingerprint, runID string
		var raw []byte
		if err := rows.Scan(&occurrenceKey, &occurrenceFingerprint, &familyKey, &familyFingerprint, &runID, &raw); err != nil {
			return nil, err
		}
		var failure farm.ObjectiveFailure
		if json.Unmarshal(raw, &failure) != nil {
			continue
		}
		if familyKey == "" || familyFingerprint == "" {
			var err error
			familyKey, familyFingerprint, _, err = objectiveFailureFingerprint(failure)
			if err != nil {
				// Historical/corrupt rows remain visible under their exact key
				// rather than disappearing from the triage queue.
				familyKey, familyFingerprint = occurrenceKey, occurrenceFingerprint
			}
		}
		group := groups[familyKey]
		if group == nil {
			group = &acc{
				fingerprint:   familyFingerprint,
				pattern:       objectiveFailurePattern(failure),
				example:       strings.TrimSpace(failure.Objective + ": " + failure.Error),
				seenRuns:      map[string]bool{},
				seenIssueKeys: map[string]bool{},
			}
			groups[familyKey] = group
		}
		group.count++
		// Family fingerprints were introduced after many exact occurrence
		// fingerprints already owned GitHub issues. Keep those legacy keys with
		// the family so historical closed issues can still suppress stale work
		// without waiting for the same failure to recur after rollout.
		if occurrenceKey != "" && occurrenceKey != familyKey && !group.seenIssueKeys[occurrenceKey] {
			group.seenIssueKeys[occurrenceKey] = true
			group.issueKeys = append(group.issueKeys, occurrenceKey)
		}
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
		if link, ok := triageIssueLinkForFamily(w.issueLinks, key, group.issueKeys); ok {
			copy := link
			item.Issue = &copy
		} else {
			item.Dismissable = true
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

func triageIssueLinkForFamily(links map[string]IssueLink, familyKey string, occurrenceKeys []string) (IssueLink, bool) {
	if link, ok := links[familyKey]; ok && strings.TrimSpace(link.IssueID) != "" {
		return link, true
	}

	var best IssueLink
	bestPriority := -1
	for _, key := range occurrenceKeys {
		link, ok := links[key]
		if !ok || strings.TrimSpace(link.IssueID) == "" {
			continue
		}
		priority := triageIssueLinkPriority(link)
		if priority > bestPriority ||
			(priority == bestPriority && link.UpdatedAt > best.UpdatedAt) ||
			(priority == bestPriority && link.UpdatedAt == best.UpdatedAt && link.IssueNumber > best.IssueNumber) {
			best = link
			bestPriority = priority
		}
	}
	return best, bestPriority >= 0
}

// Active legacy links win over settled ones. A semantic family can contain
// several pre-family exact issues; choosing a closed sibling while another is
// still open would incorrectly hide actionable work.
func triageIssueLinkPriority(link IssueLink) int {
	status := strings.ToLower(strings.TrimSpace(link.Status))
	resolution := strings.ToLower(strings.TrimSpace(link.Resolution))
	switch status {
	case "open", "reopened", "investigating", "in_progress", "in-progress", "todo", "backlog":
		return 3
	}
	if resolution != "" {
		return 1
	}
	switch status {
	case "resolved", "closed", "fixed", "done", "completed":
		return 1
	default:
		return 2
	}
}

type dismissTriageRequest struct {
	Keys []string `json:"keys"`
}

type dismissTriageResult struct {
	Status        string   `json:"status"`
	Groups        int64    `json:"groups"`
	Occurrences   int64    `json:"occurrences"`
	SkippedLinked []string `json:"skipped_linked,omitempty"`
}

func normalizeTriageKeys(keys []string) []string {
	seen := make(map[string]bool, len(keys))
	out := make([]string, 0, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func (cp *controlPlane) dismissObjectiveFailureGroups(keys []string) (dismissTriageResult, error) {
	result := dismissTriageResult{Status: "dismissed"}
	keys = normalizeTriageKeys(keys)
	if len(keys) == 0 {
		return result, nil
	}

	tx, err := cp.db.Begin()
	if err != nil {
		return dismissTriageResult{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	selected := make(map[string]bool, len(keys))
	for _, key := range keys {
		var issueID string
		err := tx.QueryRow(`SELECT issue_id FROM issue_links WHERE failure_key=$1 AND issue_id<>''`, key).Scan(&issueID)
		if err == nil {
			result.SkippedLinked = append(result.SkippedLinked, key)
			continue
		}
		if err != sql.ErrNoRows {
			return dismissTriageResult{}, err
		}
		selected[key] = true
	}
	if len(selected) == 0 {
		if err := tx.Commit(); err != nil {
			return dismissTriageResult{}, err
		}
		return result, nil
	}

	// Historical rows predate family_key/family_fingerprint. Triage still groups
	// those rows by recomputing the canonical family from failure_json, so the
	// visible triage key is not necessarily stored in either key column. Resolve
	// the same effective identity here before updating, otherwise dismissing an
	// old group reports success with zero affected rows.
	type candidate struct {
		runID             string
		attempt           int
		occurrenceKey     string
		familyKey         string
		familyFingerprint string
		failureRaw        []byte
	}
	rows, err := tx.Query(`
SELECT run_id, attempt, failure_key, family_key, family_fingerprint, failure_json
FROM objective_failures
WHERE (blocking=TRUE OR terminal_count>0)
  AND delivery_status<>'dismissed'`)
	if err != nil {
		return dismissTriageResult{}, err
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.runID, &item.attempt, &item.occurrenceKey, &item.familyKey, &item.familyFingerprint, &item.failureRaw); err != nil {
			rows.Close()
			return dismissTriageResult{}, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return dismissTriageResult{}, err
	}
	if err := rows.Close(); err != nil {
		return dismissTriageResult{}, err
	}

	dismissedGroups := make(map[string]bool, len(selected))
	for _, item := range candidates {
		effectiveKey := strings.TrimSpace(item.familyKey)
		effectiveFingerprint := strings.TrimSpace(item.familyFingerprint)
		if effectiveKey == "" || effectiveFingerprint == "" {
			var failure farm.ObjectiveFailure
			if json.Unmarshal(item.failureRaw, &failure) == nil {
				if key, fingerprint, _, fingerprintErr := objectiveFailureFingerprint(failure); fingerprintErr == nil {
					effectiveKey = key
					effectiveFingerprint = fingerprint
				}
			}
		}
		if effectiveKey == "" {
			effectiveKey = item.occurrenceKey
		}
		if !selected[effectiveKey] {
			continue
		}

		updated, err := tx.Exec(`
UPDATE objective_failures
SET family_key=$1,
    family_fingerprint=$2,
    delivery_status='dismissed',
    delivery_error='',
    updated_at=CURRENT_TIMESTAMP
WHERE run_id=$3
  AND attempt=$4
  AND failure_key=$5
  AND delivery_status<>'dismissed'`,
			effectiveKey, effectiveFingerprint, item.runID, item.attempt, item.occurrenceKey)
		if err != nil {
			return dismissTriageResult{}, err
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return dismissTriageResult{}, err
		}
		if count > 0 {
			result.Occurrences += count
			dismissedGroups[effectiveKey] = true
		}
	}
	result.Groups = int64(len(dismissedGroups))

	if err := tx.Commit(); err != nil {
		return dismissTriageResult{}, err
	}
	return result, nil
}

func (cp *controlPlane) dismissObjectiveFailureGroup(key string) (int64, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, false, fmt.Errorf("failure key is required")
	}
	result, err := cp.dismissObjectiveFailureGroups([]string{key})
	if err != nil {
		return 0, false, err
	}
	return result.Occurrences, len(result.SkippedLinked) > 0, nil
}

func (w *Wall) handleDismissTriages(res http.ResponseWriter, req *http.Request) {
	req.Body = http.MaxBytesReader(res, req.Body, maxSmallControlBody)
	var body dismissTriageRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad dismiss request: " + err.Error()})
		return
	}
	keys := normalizeTriageKeys(body.Keys)
	if len(keys) == 0 {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "at least one failure key is required"})
		return
	}
	if len(keys) > 1000 {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "at most 1000 failure keys may be dismissed at once"})
		return
	}

	// The durable DB is authoritative, but keep in-memory issue links as an
	// additional guard for the small window before a freshly-created link has
	// been persisted.
	allowed := make([]string, 0, len(keys))
	skipped := make([]string, 0)
	w.mu.Lock()
	for _, key := range keys {
		if link := w.issueLinks[key]; link.IssueID != "" {
			skipped = append(skipped, key)
			continue
		}
		allowed = append(allowed, key)
	}
	w.mu.Unlock()

	cp := controlPlaneFor(w)
	if cp == nil {
		writeJSON(res, http.StatusServiceUnavailable, map[string]string{"error": "durable triage dismissal requires the control plane"})
		return
	}
	result, err := cp.dismissObjectiveFailureGroups(allowed)
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	result.SkippedLinked = append(skipped, result.SkippedLinked...)
	writeJSON(res, http.StatusOK, result)
}

func (w *Wall) handleDismissTriage(res http.ResponseWriter, req *http.Request) {
	key := strings.TrimSpace(req.PathValue("key"))
	if key == "" {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "failure key is required"})
		return
	}
	w.mu.Lock()
	link := w.issueLinks[key]
	w.mu.Unlock()
	if link.IssueID != "" {
		writeJSON(res, http.StatusConflict, map[string]string{"error": "failure group already has a linked issue"})
		return
	}
	cp := controlPlaneFor(w)
	if cp == nil {
		writeJSON(res, http.StatusServiceUnavailable, map[string]string{"error": "durable triage dismissal requires the control plane"})
		return
	}
	count, linked, err := cp.dismissObjectiveFailureGroup(key)
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if linked {
		writeJSON(res, http.StatusConflict, map[string]string{"error": "failure group already has a linked issue"})
		return
	}
	writeJSON(res, http.StatusOK, map[string]any{
		"status":      "dismissed",
		"key":         key,
		"occurrences": count,
	})
}
