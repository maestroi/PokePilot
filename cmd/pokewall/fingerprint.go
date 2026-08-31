package main

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	outboxPending  = "pending"
	outboxComplete = "complete"
	outboxError    = "error"

	checkpointPeriodicKeep  = 3
	checkpointObjectiveKeep = 1
)

// IssueLink is the wall's copy of an Agent Orchestrator issue identity.
// Issue numbers are display-only; links always use the UUID URL.
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
