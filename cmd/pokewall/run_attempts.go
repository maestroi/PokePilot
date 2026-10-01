package main

import (
	"encoding/json"
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
	// Fixed is derived from the linked issue (status/resolution plus a
	// fixed revision), never set by hand.
	Fixed         bool   `json:"fixed,omitempty"`
	FixedRevision string `json:"fixed_revision,omitempty"`
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
       f.blocking, COALESCE(l.issue_id,''), COALESCE(l.status,''), COALESCE(l.payload_json,'{}')
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
		var payload []byte
		if err := prob.Scan(&attempt, &p.TriageKey, &p.Fingerprint, &p.Blocking, &p.IssueID, &p.IssueStatus, &payload); err != nil {
			return nil, err
		}
		var link IssueLink
		if json.Unmarshal(payload, &link) == nil {
			p.Fixed, p.FixedRevision = issueFixedForVerification(link), link.FixedRevision
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

type runRecoveryView struct {
	Attempt  int              `json:"attempt"`
	Kind     string           `json:"kind"`
	At       int64            `json:"at,omitempty"`
	Summary  string           `json:"summary"`
	Detail   string           `json:"detail,omitempty"`
	Problems []attemptProblem `json:"problems"`
	// Fixed: every attached problem has a fixed issue. A recovery with no
	// problem (e.g. a deploy drain) is never "fixed", it just has no problem.
	Fixed bool `json:"fixed"`
}

// recoveryProblemAttempt is the attempt whose problem a recovery responds to:
// retry/failure events are stamped with the failed attempt, the rest
// (resume, rollback, fresh_start, circuit, recovered) with the next one.
func recoveryProblemAttempt(e runActivityEvent) int {
	if e.Kind == "retry" || e.Kind == "failure" {
		return e.Attempt
	}
	return e.Attempt - 1
}

func buildRunRecoveries(activity []runActivityEvent, attempts []runAttemptView) []runRecoveryView {
	problems := map[int][]attemptProblem{}
	for _, a := range attempts {
		problems[a.Attempt] = a.Problems
	}
	out := []runRecoveryView{}
	for _, e := range activity {
		if e.Source != "recovery" {
			continue
		}
		p := problems[recoveryProblemAttempt(e)]
		if p == nil {
			p = []attemptProblem{}
		}
		fixed := len(p) > 0
		for _, x := range p {
			fixed = fixed && x.Fixed
		}
		out = append(out, runRecoveryView{Attempt: e.Attempt, Kind: e.Kind, At: e.At, Summary: e.Summary, Detail: e.Detail, Problems: p, Fixed: fixed})
	}
	return out
}

func (w *Wall) handleRunRecoveries(res http.ResponseWriter, req *http.Request) {
	runID := strings.TrimSpace(req.PathValue("id"))
	run, found := w.snapshotRun(runID)
	cp := controlPlaneFor(w)
	if !found || cp == nil {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	attempts, err := cp.runAttempts(runID)
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(res, http.StatusOK, buildRunRecoveries(run.Activity, attempts))
}
