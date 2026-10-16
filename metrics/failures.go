package metrics

import (
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Failure outcome values describe individual objective-failure occurrences,
// not terminal run states. One objective group may contain multiple outcomes.
const (
	FailureRecovered  = "recovered"
	FailureTerminal   = "terminal"
	FailureUnresolved = "unresolved"
)

// NormalizeFailureType maps runner-supplied causes into a deliberately small
// vocabulary. Never use arbitrary cause/error text or fingerprints as labels.
func NormalizeFailureType(cause string) string {
	cause = strings.ToLower(strings.TrimSpace(cause))
	switch cause {
	case "stagnation", "failure-budget", "run-error", "inference-transport",
		"no_pokeball", "pc_box_full", "cant_afford", "not_in_stock":
		return strings.ReplaceAll(cause, "-", "_")
	case "blacked_out", "trainer_blacked_out", "combat_defeat":
		return "combat"
	case "route_prerequisite_missing", "field_move_prerequisite_missing":
		return "prerequisite"
	}
	switch {
	case strings.HasPrefix(cause, "field_"):
		return "field_move"
	case strings.HasPrefix(cause, "route_"), strings.HasPrefix(cause, "navigation_"):
		return "navigation"
	case strings.HasPrefix(cause, "trainer_"), strings.HasPrefix(cause, "combat_"), strings.HasPrefix(cause, "battle_"):
		return "combat"
	case strings.HasPrefix(cause, "catch_"), strings.HasPrefix(cause, "fishing_"):
		return "encounter"
	case strings.HasPrefix(cause, "train_"):
		return "training"
	case strings.HasPrefix(cause, "pc_"):
		return "pc"
	case strings.HasPrefix(cause, "shop_"), strings.HasPrefix(cause, "mart_"):
		return "shop"
	default:
		return "other"
	}
}

// NormalizeFailureReason keeps retry/terminal reason labels independent of
// arbitrary runner detail text and future stop-reason additions.
func NormalizeFailureReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "error", "failed", "stuck", "budget", "lost":
		return strings.ToLower(strings.TrimSpace(reason))
	default:
		return "other"
	}
}

// Failures exports objective-failure occurrence impacts and wall supervisor
// recovery attempts. Failure fingerprints and run identifiers stay in logs.
type Failures struct {
	observed           *prometheus.CounterVec
	recovered          *prometheus.CounterVec
	recoveryAttempts   *prometheus.CounterVec
	supervisorRecovery *prometheus.CounterVec
	timeToFailure      *prometheus.HistogramVec
}

// NewFailures registers wall failure/recovery collectors.
func NewFailures(reg *Registry) (*Failures, error) {
	if reg == nil {
		return nil, fmt.Errorf("metrics: nil registry")
	}
	s := &Failures{}
	var err error
	s.observed, err = reg.NewCounterVec("pokepilot_run_failures_total",
		"Objective failure occurrences by bounded category and outcome. A grouped report contributes its occurrence counts, not one sample per group.",
		[]string{"game", "failure_type", "outcome"})
	if err != nil {
		return nil, err
	}
	s.recovered, err = reg.NewCounterVec("pokepilot_failure_recoveries_total",
		"Objective failure occurrences recovered in the runner (subset of outcome=recovered).",
		[]string{"game", "failure_type"})
	if err != nil {
		return nil, err
	}
	s.recoveryAttempts, err = reg.NewCounterVec("pokepilot_failure_recovery_attempts_total",
		"Failed runner generations queued again by the wall supervisor. Deploy drains and user cancels are excluded.",
		[]string{"game", "reason"})
	if err != nil {
		return nil, err
	}
	s.supervisorRecovery, err = reg.NewCounterVec("pokepilot_failure_supervisor_recoveries_total",
		"Resilient supervisor recoveries confirmed by advancing the progression high-water mark.",
		[]string{"game"})
	if err != nil {
		return nil, err
	}
	s.timeToFailure, err = reg.NewHistogramVec("pokepilot_failure_time_to_failure_seconds",
		"Elapsed seconds from campaign queue to terminal unsuccessful run settlement, by bounded stop reason.",
		[]string{"game", "reason"}, RunDurationBuckets)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Objective records one decoded failure group; recovered and terminal are
// disjoint per-occurrence counts, with the remainder unresolved. A malformed
// legacy negative count is ignored. Call only for newly accepted finishes.
func (s *Failures) Objective(game, cause string, count, recovered, terminal int) {
	if s == nil || count <= 0 {
		return
	}
	if recovered < 0 {
		recovered = 0
	}
	if recovered > count {
		recovered = count
	}
	if terminal < 0 {
		terminal = 0
	}
	if terminal > count-recovered {
		terminal = count - recovered
	}
	game, kind := NormalizeGame(game), NormalizeFailureType(cause)
	if recovered > 0 {
		s.observed.WithLabelValues(game, kind, FailureRecovered).Add(float64(recovered))
		s.recovered.WithLabelValues(game, kind).Add(float64(recovered))
	}
	if terminal > 0 {
		s.observed.WithLabelValues(game, kind, FailureTerminal).Add(float64(terminal))
	}
	if rest := count - recovered - terminal; rest > 0 {
		s.observed.WithLabelValues(game, kind, FailureUnresolved).Add(float64(rest))
	}
}

// Retry records a newly queued recovery generation, excluding administrative
// drains and cancellations. Call once per actual transition into retry.
func (s *Failures) Retry(game, reason string) {
	if s != nil {
		s.recoveryAttempts.WithLabelValues(NormalizeGame(game), NormalizeFailureReason(reason)).Inc()
	}
}

// SupervisorRecovered counts one confirmed progression advancement following
// one or more resilient retries (not each retry).
func (s *Failures) SupervisorRecovered(game string) {
	if s != nil {
		s.supervisorRecovery.WithLabelValues(NormalizeGame(game)).Inc()
	}
}

// TerminalFailure records wall-clock duration of one unsuccessful terminal run.
// A missing/invalid queue timestamp must not become a zero-duration sample.
func (s *Failures) TerminalFailure(game, reason string, queuedAt, endedAt time.Time) {
	if s == nil || queuedAt.IsZero() || endedAt.Before(queuedAt) {
		return
	}
	s.timeToFailure.WithLabelValues(NormalizeGame(game), NormalizeFailureReason(reason)).Observe(endedAt.Sub(queuedAt).Seconds())
}
