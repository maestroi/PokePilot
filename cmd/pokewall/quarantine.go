package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

type issueOccurrenceDisposition string

const (
	occurrenceReport     issueOccurrenceDisposition = "report"
	occurrenceQuarantine issueOccurrenceDisposition = "quarantine"
	occurrenceRegression issueOccurrenceDisposition = "regression"
)

// classifyIssueOccurrence decides whether another sighting of a canonical
// failure fingerprint should create new actionable work. Active issues own the
// defect already, so another equivalent occurrence is quarantined locally while
// its durable run dump remains evidence. A resolved/closed issue is different:
// unless it was explicitly ignored, a fresh sighting is a regression candidate
// and must be reported so Agent Orchestrator can reopen/reclassify it.
//
// Active status deliberately wins over a stale resolution field. Agent
// Orchestrator may reopen an issue before clearing its previous "fixed"
// resolution; treating that as resolved would hide the regression again.
func classifyIssueOccurrence(link IssueLink) issueOccurrenceDisposition {
	if link.IssueID == "" {
		return occurrenceReport
	}
	status := strings.ToLower(strings.TrimSpace(link.Status))
	resolution := strings.ToLower(strings.TrimSpace(link.Resolution))

	switch status {
	case "open", "actionable", "investigating", "reopened":
		return occurrenceQuarantine
	case "resolved", "closed", "fixed", "fixed-applied", "fixed_applied":
		if ignoredResolution(resolution) {
			return occurrenceQuarantine
		}
		return occurrenceRegression
	default:
		// Unknown states are never silently quarantined. Reporting is safer than
		// teaching the wall a new Agent Orchestrator lifecycle value by guess.
		return occurrenceReport
	}
}

func ignoredResolution(resolution string) bool {
	switch resolution {
	case "ignored", "ignore", "not_planned", "not-planned", "wontfix", "wont-fix", "won't_fix", "won't-fix":
		return true
	default:
		return false
	}
}

func (w *Wall) issueDispositionForKey(key string) (IssueLink, issueOccurrenceDisposition) {
	w.mu.Lock()
	link := w.issueLinks[key]
	w.mu.Unlock()
	return link, classifyIssueOccurrence(link)
}

// quarantineOccurrence settles an occurrence without sending it to Agent
// Orchestrator. The finish dump/checkpoint remain the immutable historical
// evidence; the persisted outbox marker makes restart rescans idempotent.
// Repeating the same external id never increments the quarantine counter.
func (w *Wall) quarantineOccurrence(e outboxEntry, fingerprint, build, note string) bool {
	return w.settleQuarantinedOutbox(e, fingerprint, build, note, true)
}

// deferOccurrence transfers ownership from the generic terminal-run reporter
// to the richer structured objective reporter. It uses the same durable outbox
// terminal state as quarantine so restarts cannot redispatch it, but it does
// NOT count as another failure occurrence: the objective reporter records that
// single sighting exactly once.
func (w *Wall) deferOccurrence(e outboxEntry, fingerprint, build, note string) bool {
	return w.settleQuarantinedOutbox(e, fingerprint, build, note, false)
}

func (w *Wall) settleQuarantinedOutbox(e outboxEntry, fingerprint, build, note string, countOccurrence bool) bool {
	now := time.Now().Unix()
	w.mu.Lock()
	if existing, ok := w.outbox[e.ExternalID]; ok {
		if existing.Status == outboxComplete || existing.Status == outboxQuarantined {
			w.mu.Unlock()
			return false
		}
		// Preserve the canonical fields already persisted by enqueueing while
		// allowing a direct objective-failure reporter to supply a fresh entry.
		if e.RunID == "" {
			e.RunID = existing.RunID
		}
		if e.Attempt == 0 {
			e.Attempt = existing.Attempt
		}
		if e.Key == "" {
			e.Key = existing.Key
		}
	}
	e.Status = outboxQuarantined
	e.Error = ""
	e.Note = strings.TrimSpace(note)
	e.NextAttempt = 0
	e.UpdatedAt = now
	w.outbox[e.ExternalID] = e

	if countOccurrence {
		if link, ok := w.issueLinks[e.Key]; ok && link.IssueID != "" {
			link.QuarantinedCount++
			link.LastObservedRun = e.RunID
			link.LastObservedRevision = strings.TrimSpace(build)
			link.LastDisposition = string(occurrenceQuarantine)
			link.UpdatedAt = now
			if link.Fingerprint == "" {
				link.Fingerprint = fingerprint
			}
			w.issueLinks[e.Key] = link
		}
	}
	w.mu.Unlock()
	w.saveState()
	return true
}

func hasObjectiveFailureArtifact(report farm.FinishReport) bool {
	for _, artifact := range report.Artifacts {
		if artifact.Name == farm.ObjectiveFailureArtifactName {
			return true
		}
	}
	return false
}

// objectiveFailureFingerprint prefers the versioned structured identity carried
// by v2 objective-failure telemetry. Historical v1/manual entries retain the
// old normalized-prose fingerprint so existing issue history stays reachable.
func objectiveFailureFingerprint(f farm.ObjectiveFailure) (key, fingerprint string, structured bool, err error) {
	if f.Identity == nil {
		pattern := objectiveFailurePattern(f)
		key, fingerprint = failureIdentity(pattern)
		return key, fingerprint, false, nil
	}

	key, fingerprint, err = farm.FingerprintFailureIdentity(*f.Identity)
	if err != nil {
		return "", "", true, err
	}
	if f.Key != "" && f.Key != key {
		return "", "", true, fmt.Errorf("objective failure key %q does not match structured identity %q", f.Key, key)
	}
	if f.Fingerprint != "" && f.Fingerprint != fingerprint {
		return "", "", true, fmt.Errorf("objective failure fingerprint %q does not match structured identity %q", f.Fingerprint, fingerprint)
	}
	return key, fingerprint, true, nil
}
