package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	maxSolverAttempts      = 32
	maxSolverAttemptField  = 256
	maxSolverAttemptNote   = 1024
	maxSolverAttemptBody   = 16 << 10
)

func cleanSolverAttemptField(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit > 0 && len(value) > limit {
		value = value[:limit]
	}
	return value
}

func mergeSolverAttempt(previous, next SolverAttempt) SolverAttempt {
	if next.Backend == "" {
		next.Backend = previous.Backend
	}
	if next.Model == "" {
		next.Model = previous.Model
	}
	if next.RunID == "" {
		next.RunID = previous.RunID
	}
	if next.Branch == "" {
		next.Branch = previous.Branch
	}
	if next.PRNumber == 0 {
		next.PRNumber = previous.PRNumber
	}
	if next.PRURL == "" {
		next.PRURL = previous.PRURL
	}
	if next.Note == "" {
		next.Note = previous.Note
	}
	if next.StartedAt == 0 {
		next.StartedAt = previous.StartedAt
	}
	return next
}

// handleSolverAttempt upserts one coding-agent attempt for a stable failure
// key. Issue resolution/verification remains authoritative for whether a fix
// actually worked; this endpoint only records who tried and what artifact
// (usually a PR) that attempt produced.
func (w *Wall) handleSolverAttempt(res http.ResponseWriter, req *http.Request) {
	key := strings.TrimSpace(req.PathValue("key"))
	if key == "" {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "failure key is required"})
		return
	}
	req.Body = http.MaxBytesReader(res, req.Body, maxSolverAttemptBody)
	var attempt SolverAttempt
	if err := json.NewDecoder(req.Body).Decode(&attempt); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad solver attempt: " + err.Error()})
		return
	}

	attempt.ID = cleanSolverAttemptField(attempt.ID, maxSolverAttemptField)
	attempt.Backend = cleanSolverAttemptField(attempt.Backend, maxSolverAttemptField)
	attempt.Model = cleanSolverAttemptField(attempt.Model, maxSolverAttemptField)
	attempt.State = cleanSolverAttemptField(attempt.State, maxSolverAttemptField)
	attempt.RunID = cleanSolverAttemptField(attempt.RunID, maxSolverAttemptField)
	attempt.Branch = cleanSolverAttemptField(attempt.Branch, maxSolverAttemptField)
	attempt.PRURL = cleanSolverAttemptField(attempt.PRURL, maxSolverAttemptField)
	attempt.Note = cleanSolverAttemptField(attempt.Note, maxSolverAttemptNote)
	if attempt.ID == "" || attempt.Backend == "" || attempt.State == "" {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "id, backend, and state are required"})
		return
	}
	if attempt.StartedAt < 0 || attempt.UpdatedAt < 0 || attempt.PRNumber < 0 || attempt.ExitCode < 0 {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "timestamps, pr_number, and exit_code must be non-negative"})
		return
	}

	now := time.Now().Unix()
	attempt.UpdatedAt = now

	w.mu.Lock()
	link, ok := w.issueLinks[key]
	if !ok || link.IssueID == "" {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown failure group " + key})
		return
	}

	found := false
	for i := range link.SolverAttempts {
		if link.SolverAttempts[i].ID != attempt.ID {
			continue
		}
		attempt = mergeSolverAttempt(link.SolverAttempts[i], attempt)
		link.SolverAttempts[i] = attempt
		found = true
		break
	}
	if !found {
		if attempt.StartedAt == 0 {
			attempt.StartedAt = now
		}
		link.SolverAttempts = append(link.SolverAttempts, attempt)
		if len(link.SolverAttempts) > maxSolverAttempts {
			link.SolverAttempts = append([]SolverAttempt(nil), link.SolverAttempts[len(link.SolverAttempts)-maxSolverAttempts:]...)
		}
	}
	w.issueLinks[key] = link
	count := len(link.SolverAttempts)
	w.mu.Unlock()

	w.saveState()
	writeJSON(res, http.StatusOK, map[string]any{
		"key":           key,
		"attempt":       attempt,
		"attempt_count": count,
	})
}
