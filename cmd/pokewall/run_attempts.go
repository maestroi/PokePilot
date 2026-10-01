package main

import (
	"net/http"
	"strings"
)

// attemptProblem is one failure group an attempt hit; the triage key is the
// same stable key the triage queue and issue links use.
type attemptProblem struct {
	TriageKey   string `json:"triage_key"`
	Fingerprint string `json:"fingerprint"`
	Blocking    bool   `json:"blocking,omitempty"`
	IssueID     string `json:"issue_id,omitempty"`
	IssueStatus string `json:"issue_status,omitempty"`
}

type runAttemptView struct {
	Attempt       int              `json:"attempt"`
	Reason        string           `json:"reason,omitempty"`
	Detail        string           `json:"detail,omitempty"`
	RunnerVersion string           `json:"runner_version,omitempty"`
	Problems      []attemptProblem `json:"problems"`
}

// runAttempts lists every finished attempt of a run with the problems
// (objective failures) attached to it.
func (cp *controlPlane) runAttempts(runID string) ([]runAttemptView, error) {
	rows, err := cp.db.Query(`SELECT attempt, reason, detail, runner_version FROM run_attempts WHERE run_id=? ORDER BY attempt`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []runAttemptView{}
	byAttempt := map[int]int{}
	for rows.Next() {
		v := runAttemptView{Problems: []attemptProblem{}}
		if err := rows.Scan(&v.Attempt, &v.Reason, &v.Detail, &v.RunnerVersion); err != nil {
			return nil, err
		}
		byAttempt[v.Attempt] = len(out)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	prob, err := cp.db.Query(`
SELECT f.attempt, COALESCE(NULLIF(f.family_key,''), f.failure_key), COALESCE(NULLIF(f.family_fingerprint,''), f.fingerprint),
       f.blocking, COALESCE(l.issue_id,''), COALESCE(l.status,'')
FROM objective_failures f
LEFT JOIN issue_links l ON l.failure_key=COALESCE(NULLIF(f.family_key,''), f.failure_key)
WHERE f.run_id=? ORDER BY f.attempt, f.updated_at`, runID)
	if err != nil {
		return nil, err
	}
	defer prob.Close()
	seen := map[[2]string]bool{}
	for prob.Next() {
		var attempt int
		var p attemptProblem
		if err := prob.Scan(&attempt, &p.TriageKey, &p.Fingerprint, &p.Blocking, &p.IssueID, &p.IssueStatus); err != nil {
			return nil, err
		}
		i, ok := byAttempt[attempt]
		id := [2]string{string(rune(attempt)), p.TriageKey}
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		out[i].Problems = append(out[i].Problems, p)
	}
	return out, prob.Err()
}

func (w *Wall) handleRunAttempts(res http.ResponseWriter, req *http.Request) {
	runID := strings.TrimSpace(req.PathValue("id"))
	cp := controlPlaneFor(w)
	if runID == "" || cp == nil {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	attempts, err := cp.runAttempts(runID)
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(res, http.StatusOK, attempts)
}
