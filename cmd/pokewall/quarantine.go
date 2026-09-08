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

// quarantineOccurrence settles one reporter record without creating new
// actionable work. The underlying run dump/checkpoint is the immutable
// occurrence evidence. Both the generic terminal-run reporter and the richer
// objective reporter can observe the same run/attempt/fingerprint, so the local
// quarantine counter is de-duplicated across their outbox records.
func (w *Wall) quarantineOccurrence(e outboxEntry, fingerprint, build, note string) bool {
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

	alreadyCounted := false
	for _, prior := range w.outbox {
		if prior.Status == outboxQuarantined && prior.RunID == e.RunID && prior.Attempt == e.Attempt && prior.Key == e.Key {
			alreadyCounted = true
			break
		}
	}

	e.Status = outboxQuarantined
	e.Error = ""
	e.Note = strings.TrimSpace(note)
	e.NextAttempt = 0
	e.UpdatedAt = now
	w.outbox[e.ExternalID] = e

	if !alreadyCounted {
		if link, ok := w.issueLinks[e.Key]; ok && link.IssueID != "" && classifyIssueOccurrence(link) == occurrenceQuarantine {
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
