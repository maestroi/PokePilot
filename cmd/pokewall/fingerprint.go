package main

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	outboxPending  = "pending"
	outboxComplete = "complete"
	outboxError    = "error"

	// Keep enough flight-recorder history to replay a failure from before the
	// final bad decision. Objective checkpoints are the replay-safe LLM
	// boundaries because they carry paired agent knowledge.
	checkpointPeriodicKeep  = 6
	checkpointObjectiveKeep = 6
)

// IssueLink is the wall's copy of an Agent Orchestrator issue identity plus
// local disposition metadata for the failure fingerprint. The local fields
// deliberately live here because issueLinks is already durable wall state:
// a resolved failure survives wall restarts even when no external issue exists.
type IssueLink struct {
	IssueID         string `json:"issue_id"`
	IssueNumber     int64  `json:"issue_number"`
	IssueURL        string `json:"issue_url"`
	Status          string `json:"status,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
	OccurrenceCount int64  `json:"occurrence_count,omitempty"`
	FixedRevision   string `json:"fixed_revision,omitempty"`
	LastReportedRun string `json:"last_reported_run,omitempty"`
	UpdatedAt       int64  `json:"updated_at,omitempty"`
	Fingerprint     string `json:"fingerprint,omitempty"`
	Stale           bool   `json:"stale,omitempty"`

	// FailureResolution is PokePilot's local triage disposition. It is kept
	// separate from Agent Orchestrator's Resolution so either system can be
	// used independently. fixed means a code fix exists but may not be live;
	// fixed_applied records the occurrence baseline after the fix is live.
	FailureResolution      string `json:"failure_resolution,omitempty"`
	FailureResolutionCount int    `json:"failure_resolution_count,omitempty"`
	FailureRevision        string `json:"failure_revision,omitempty"`
	FailureNote            string `json:"failure_note,omitempty"`
	FailureUpdatedAt       int64  `json:"failure_updated_at,omitempty"`
}

type outboxEntry struct {
	ExternalID  string `json:"external_id"`
	RunID       string `json:"run_id"`
	Attempt     int    `json:"attempt"`
	Key         string `json:"key"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	NextAttempt int64  `json:"next_attempt,omitempty"`
	UpdatedAt   int64  `json:"updated_at"`
}

func failureIdentity(pattern string) (key, fingerprint string) {
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
