package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const (
	defaultObjectiveFailureReportEvery    = 2 * time.Second
	agentOrchestratorMaxArtifactBytes      = 8 << 20
)

type objectiveDumpStamp struct {
	size    int64
	modNano int64
}

// RunObjectiveFailureReporter watches durable finish dumps for the structured
// objective-failure artifact emitted by new runners. Each run/attempt/failure
// group becomes one stable Agent Orchestrator occurrence. This deliberately
// happens after the run settles: recovered edge cases are preserved as data,
// while repeated unrecovered frontier failures can be escalated as critical
// progression blockers.
//
// The local seen cache avoids rereading multi-megabyte finish dumps every two
// seconds. Agent Orchestrator's external_id is the durable idempotency
// boundary: the runner-stamped ObservedAt and dump-derived evidence stay
// identical across retries, including a wall restart after a lost HTTP reply.
func (w *Wall) RunObjectiveFailureReporter(every time.Duration) {
	if every <= 0 {
		every = defaultObjectiveFailureReportEvery
	}
	seen := map[string]objectiveDumpStamp{}
	for {
		w.scanObjectiveFailureDumps(seen)
		time.Sleep(every)
	}
}

func (w *Wall) scanObjectiveFailureDumps(seen map[string]objectiveDumpStamp) {
	if w.dumpsDir == "" || w.issueClient() == nil {
		return
	}
	entries, err := os.ReadDir(w.dumpsDir)
	if err != nil {
		log.Printf("pokewall: scan objective failure dumps: %v", err)
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		stamp := objectiveDumpStamp{size: info.Size(), modNano: info.ModTime().UnixNano()}
		path := filepath.Join(w.dumpsDir, entry.Name())
		if seen[path] == stamp {
			continue
		}
		retry, err := w.reportObjectiveFailureDump(path)
		if err != nil {
			log.Printf("pokewall: objective failure report %s: %v", entry.Name(), err)
		}
		if !retry {
			seen[path] = stamp
		}
	}
}

// reportObjectiveFailureDump returns retry=true only for a transient report
// failure. Decode/schema failures are permanent for that immutable dump and
// are logged once per wall process rather than hot-looped forever.
func (w *Wall) reportObjectiveFailureDump(path string) (retry bool, retErr error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return true, err
	}
	var dump farm.FinishReport
	if err := json.Unmarshal(data, &dump); err != nil {
		return false, fmt.Errorf("decode finish dump: %w", err)
	}
	if dump.RunID == "" {
		return false, nil
	}
	failures, err := farm.DecodeObjectiveFailures(dump)
	if err != nil {
		return false, err
	}
	for _, failure := range failures {
		if err := w.reportObjectiveFailure(dump, failure); err != nil {
			if isRetryableIssueError(err) {
				return true, err
			}
			return false, err
		}
	}
	return false, nil
}

func objectiveFailurePattern(f farm.ObjectiveFailure) string {
	// Keep the existing triage normalizer for volatile coordinates/counts but
	// add the map AFTER normalization. Map-local navigation bugs are distinct
	// progression frontiers and should not be merged merely because their
	// error strings have the same shape.
	base := normalizeDetail(strings.TrimSpace(f.Objective) + " | " + strings.TrimSpace(f.Error))
	return fmt.Sprintf("%s | map=%02x", base, f.Map)
}

func objectiveFailureExternalID(runID string, attempt int, key string) string {
	if attempt <= 0 {
		attempt = 1
	}
	return fmt.Sprintf("%s-attempt-%d-objective-%s", runID, attempt, key)
}

func (w *Wall) reportObjectiveFailure(dump farm.FinishReport, f farm.ObjectiveFailure) error {
	c := w.issueClient()
	if c == nil {
		return fmt.Errorf("issue integration is not configured")
	}
	pattern := objectiveFailurePattern(f)
	key, fp := failureIdentity(pattern)
	ext := objectiveFailureExternalID(dump.RunID, dump.Attempt, key)

	w.mu.Lock()
	if existing, ok := w.outbox[ext]; ok && existing.Status == outboxComplete {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()

	severity := "normal"
	classification := "observed"
	if f.Recovered {
		classification = "recovered"
	}
	if f.Blocking {
		severity = "critical"
		classification = "progression-blocker"
	}

	evidence, _ := json.Marshal(map[string]any{
		"classification":     classification,
		"run_id":             dump.RunID,
		"attempt":            max(1, dump.Attempt),
		"seed_burn":          dump.SeedBurn,
		"objective":          f.Objective,
		"error":              f.Error,
		"occurrences_in_run": f.Count,
		"first_round":        f.FirstRound,
		"last_round":         f.LastRound,
		"map":                fmt.Sprintf("0x%02x", f.Map),
		"x":                  f.X,
		"y":                  f.Y,
		"recovered":          f.Recovered,
		"blocking":           f.Blocking,
		"run_reason":         dump.Reason,
		"run_detail":         dump.Detail,
		"progress_early":     dump.ProgressEarly,
		"progress_final":     dump.ProgressFinal,
		"trace_tail":         dump.TraceTail,
		"runner_version":     dump.RunnerVersion,
	})

	titlePrefix := "[farm] objective failure: "
	summary := fmt.Sprintf("%s failed %d time(s) on map 0x%02x; recovered=%t; run ended %s.",
		f.Objective, f.Count, f.Map, f.Recovered, dump.Reason)
	if f.Blocking {
		titlePrefix = "[farm][progression-blocker] "
		summary = fmt.Sprintf("Progression blocker candidate: %s failed %d time(s) on map 0x%02x with no later major progress; run ended %s. Last error: %s",
			f.Objective, f.Count, f.Map, dump.Reason, f.Error)
	}
	observedAt := f.ObservedAt
	if observedAt.IsZero() {
		// Backward compatibility for hand-built/early telemetry artifacts.
		// New runners always stamp this in the finish artifact.
		observedAt = time.Now().UTC()
	}
	manifest := issueReportManifest{
		Source:           issueSource,
		Fingerprint:      fp,
		ExternalID:       ext,
		Title:            truncateBytes(titlePrefix+f.Objective+": "+f.Error, maxIssueTitleBytes),
		Summary:          summary,
		ObservedAt:       observedAt,
		ObservedRevision: dump.RunnerVersion,
		Severity:         severity,
		Evidence:         evidence,
	}
	artifacts := objectiveFailureEvidenceArtifacts(dump, f)
	ctx, cancel := context.WithTimeout(context.Background(), defaultIssueTimeout)
	defer cancel()
	result, err := c.Report(ctx, manifest, artifacts)
	if err != nil {
		w.mu.Lock()
		w.outbox[ext] = outboxEntry{
			ExternalID: ext,
			RunID:      dump.RunID,
			Attempt:    max(1, dump.Attempt),
			Key:        key,
			Status:     outboxError,
			Error:      err.Error(),
			UpdatedAt:  time.Now().Unix(),
		}
		w.mu.Unlock()
		w.saveState()
		return err
	}
	if result.Issue.ID == "" {
		return fmt.Errorf("agent orchestrator returned an empty issue id")
	}

	w.mu.Lock()
	link := w.issueLinks[key]
	link.IssueID = result.Issue.ID
	link.IssueNumber = result.Issue.IssueNumber
	link.IssueURL = c.issueURL(result.Issue.ID)
	link.Status = result.Issue.Status
	link.LastReportedRun = dump.RunID
	link.UpdatedAt = time.Now().Unix()
	link.Fingerprint = fp
	w.issueLinks[key] = link
	w.outbox[ext] = outboxEntry{
		ExternalID: ext,
		RunID:      dump.RunID,
		Attempt:    max(1, dump.Attempt),
		Key:        key,
		Status:     outboxComplete,
		UpdatedAt:  time.Now().Unix(),
	}
	w.mu.Unlock()
	w.saveState()
	return nil
}

func objectiveFailureEvidenceArtifacts(dump farm.FinishReport, f farm.ObjectiveFailure) []farm.Artifact {
	prefix := fmt.Sprintf("round-%03d-", f.LastRound)
	var essential []farm.Artifact
	var recording *farm.Artifact
	for _, a := range dump.Artifacts {
		switch {
		case a.Name == farm.ObjectiveFailureArtifactName:
			essential = append(essential, a)
		case strings.HasPrefix(a.Name, prefix):
			essential = append(essential, a)
		case f.Blocking && a.Name == "run.gbrun" && len(a.Data) <= agentOrchestratorMaxArtifactBytes:
			cp := a
			recording = &cp
		}
	}
	if f.Blocking && len(dump.SaveState) > 0 {
		essential = append(essential, hashedNamed("final.state", "application/octet-stream", dump.SaveState))
	}
	if f.Blocking && len(dump.FramePNG) > 0 {
		essential = append(essential, hashedNamed("final.png", "image/png", dump.FramePNG))
	}
	if recording != nil {
		candidate := append(append([]farm.Artifact(nil), essential...), *recording)
		if farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: candidate, SeedBurn: dump.SeedBurn}) == nil {
			return candidate
		}
	}
	if farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: essential, SeedBurn: dump.SeedBurn}) != nil {
		// The structured evidence in the manifest is more important than a
		// report being rejected because a diagnostic attachment is too large.
		return nil
	}
	return essential
}
