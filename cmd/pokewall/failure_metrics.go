package main

import (
	"log/slog"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/metrics"
)

// SetFailureMetrics attaches the failure collectors. Counters are intentionally
// not reconstructed from historical rows: Prometheus owns counter persistence
// across process restarts, and replaying outbox deliveries would double-count.
func (w *Wall) SetFailureMetrics(failures *metrics.Failures) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failures = failures
}

// recordObjectiveFailuresLocked is called only for a newly accepted finish
// generation, before settlement clears retry state. A re-delivered issue
// outbox item or duplicate terminal finish must never re-record these events.
// Caller holds w.mu.
func (w *Wall) recordObjectiveFailuresLocked(game string, report farm.FinishReport) {
	if w.failures == nil {
		return
	}
	groups, err := circuitObjectiveFailures(report)
	if err != nil {
		slog.Warn("objective failure metrics: invalid telemetry",
			"service", "wall", "run_id", report.RunID, "error", err)
		return
	}
	for _, failure := range groups {
		cause := failure.Cause
		if cause == "" && failure.Identity != nil {
			cause = failure.Identity.Cause
		}
		w.failures.Objective(game, cause, failure.Count, failure.RecoveredCount, failure.TerminalCount)
		failureID, _, _, failureErr := objectiveFailureOccurrenceFingerprint(failure)
		familyID, _, _, familyErr := objectiveFailureFingerprint(failure)
		if failureErr != nil {
			failureID = ""
		}
		if familyErr != nil {
			familyID = ""
		}
		stage := "objective"
		if failure.Identity == nil {
			stage = "legacy"
		}
		// Loki can pivot from bounded Prometheus labels (game/failure_type)
		// into exact failures using the identifiers kept only in log fields.
		slog.Info("objective failure observed",
			"service", "wall",
			"event", "objective_failure",
			"run_id", report.RunID,
			"attempt", report.Attempt,
			"game", metrics.NormalizeGame(game),
			"stage", stage,
			"failure_type", metrics.NormalizeFailureType(cause),
			"failure_id", failureID,
			"family_id", familyID,
			"count", failure.Count,
			"recovered_count", failure.RecoveredCount,
			"terminal_count", failure.TerminalCount,
			"blocking", failure.Blocking,
		)
	}
}
