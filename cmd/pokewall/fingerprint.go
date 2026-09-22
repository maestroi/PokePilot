package main

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/maestroi/pokepilot/farm"
)

const (
	outboxPending     = "pending"
	outboxComplete    = "complete"
	outboxError       = "error"
	outboxQuarantined = "quarantined"

	// Keep enough flight-recorder history to replay a failure from before the
	// final bad decision. Objective checkpoints are the replay-safe LLM
	// boundaries because they carry paired agent knowledge. Major checkpoints
	// are progression milestones and get their own ring so ordinary churn can
	// never evict the last few badges.
	checkpointPeriodicKeep  = 6
	checkpointObjectiveKeep = 6
	checkpointMajorKeep     = 3
)

// SolverAttempt records one coding-agent attempt against a stable failure key.
// The wall stores this beside the issue link so model attribution survives PR
// merges and issue resolution without depending on mutable GitHub labels.
type SolverAttempt struct {
	ID        string `json:"id"`
	Backend   string `json:"backend"`
	Model     string `json:"model,omitempty"`
	State     string `json:"state"`
	RunID     string `json:"run_id,omitempty"`
	Branch    string `json:"branch,omitempty"`
	PRNumber  int64  `json:"pr_number,omitempty"`
	PRURL     string `json:"pr_url,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
	Note      string `json:"note,omitempty"`
	StartedAt int64  `json:"started_at,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

// IssueLink is the wall's copy of an Agent Orchestrator issue identity.
// Issue numbers are display-only; links always use the UUID URL.
type IssueLink struct {
	IssueID              string `json:"issue_id"`
	IssueNumber          int64  `json:"issue_number"`
	IssueURL             string `json:"issue_url"`
	Status               string `json:"status,omitempty"`
	Resolution           string `json:"resolution,omitempty"`
	OccurrenceCount      int64  `json:"occurrence_count,omitempty"`
	QuarantinedCount     int64  `json:"quarantined_count,omitempty"`
	FixedRevision        string `json:"fixed_revision,omitempty"`
	LastReportedRun      string `json:"last_reported_run,omitempty"`
	LastObservedRun      string `json:"last_observed_run,omitempty"`
	LastObservedRevision string `json:"last_observed_revision,omitempty"`
	LastDisposition      string `json:"last_disposition,omitempty"`
	UpdatedAt            int64  `json:"updated_at,omitempty"`
	Fingerprint          string `json:"fingerprint,omitempty"`
	Stale                bool            `json:"stale,omitempty"`
	SolverAttempts       []SolverAttempt `json:"solver_attempts,omitempty"`

	// Circuit* is local automation state. It marks a failure group that has
	// crossed the repeated-failure/progression-frontier threshold and should be
	// claimed by an unattended repair agent.
	CircuitOpen     bool   `json:"circuit_open,omitempty"`
	CircuitKind     string `json:"circuit_kind,omitempty"`
	CircuitCount    int    `json:"circuit_count,omitempty"`
	CircuitRunID    string `json:"circuit_run_id,omitempty"`
	CircuitOpenedAt int64  `json:"circuit_opened_at,omitempty"`

	// Verification is PokePilot-local evidence that a remotely fixed issue
	// stayed fixed. Agent Orchestrator remains the source of truth for Status,
	// Resolution and FixedRevision; these fields never mutate its lifecycle.
	VerificationState           string `json:"verification_state,omitempty"`
	VerificationRevision        string `json:"verification_revision,omitempty"`
	VerificationStartedAt       int64  `json:"verification_started_at,omitempty"`
	VerificationOccurrenceCount int64  `json:"verification_occurrence_count,omitempty"`
	VerificationRequiredRuns    int    `json:"verification_required_runs,omitempty"`
	VerificationCleanRuns       int    `json:"verification_clean_runs,omitempty"`
	VerificationQuietSeconds    int64  `json:"verification_quiet_seconds,omitempty"`
	VerificationVerifiedAt      int64  `json:"verification_verified_at,omitempty"`
	VerificationLastCleanRun    string `json:"verification_last_clean_run,omitempty"`
	VerificationLastCleanAt     int64  `json:"verification_last_clean_at,omitempty"`
	VerificationPlanner         string `json:"verification_planner,omitempty"`
	VerificationStarter         string `json:"verification_starter,omitempty"`
	VerificationGoal            string `json:"verification_goal,omitempty"`
	VerificationHasBaseline     bool   `json:"verification_has_baseline,omitempty"`
	VerificationBaselineBadges  int    `json:"verification_baseline_badges,omitempty"`
	VerificationBaselineEvents  int    `json:"verification_baseline_events,omitempty"`
	VerificationBaselineMaps    int    `json:"verification_baseline_maps,omitempty"`
}

type outboxEntry struct {
	ExternalID  string `json:"external_id"`
	RunID       string `json:"run_id"`
	Attempt     int    `json:"attempt"`
	Key         string `json:"key"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	Note        string `json:"note,omitempty"`
	NextAttempt int64  `json:"next_attempt,omitempty"`
	UpdatedAt   int64  `json:"updated_at"`
}

func failureIdentity(pattern string) (key, fingerprint string) {
	// New farm runs put a prose-free canonical fingerprint marker in Detail.
	// Recover it directly so the wall/MCP/issue handoff all share the exact
	// identity persisted in objective-failures.json. Historical dumps keep the
	// old normalized-prose SHA fallback below.
	if key, fingerprint, ok := farm.ParseFailureDetailMarker(pattern); ok {
		return key, fingerprint
	}
	sum := sha256.Sum256([]byte(pattern))
	h := hex.EncodeToString(sum[:])
	return h[:16], "sha256:" + h
}

func copyIssueLink(src map[string]IssueLink) map[string]IssueLink {
	if len(src) == 0 {
		return map[string]IssueLink{}
	}
	out := make(map[string]IssueLink, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func copyOutbox(src map[string]outboxEntry) map[string]outboxEntry {
	if len(src) == 0 {
		return map[string]outboxEntry{}
	}
	out := make(map[string]outboxEntry, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
