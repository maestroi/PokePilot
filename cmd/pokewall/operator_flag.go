package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

// operatorFlagGrace bounds how long a flagged run may keep heartbeating
// without finishing before the reaper settles it as stuck itself.
const operatorFlagGrace = 10 * time.Minute

// handleFlagStuck lets an operator declare an active run stuck. The runner is
// stopped through the cooperative cancel flag; operatorFlagFinish then turns
// that stop into the ordinary stuck outcome, so the failure reaches triage
// and the fixer exactly like a stagnation-watchdog stop.
func (w *Wall) handleFlagStuck(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var body struct {
		Note string `json:"note"`
	}
	req.Body = http.MaxBytesReader(res, req.Body, 4<<10)
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad flag body: " + err.Error()})
		return
	}
	note := strings.TrimSpace(body.Note)
	if note == "" {
		note = "no note"
	}
	if len(note) > 500 {
		note = note[:500]
	}
	w.mu.Lock()
	t, ok := w.tiles[id]
	if !ok {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
		return
	}
	if t.Finished {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already finished: " + id})
		return
	}
	now := time.Now()
	t.OperatorFlag = note
	t.OperatorFlagAttempt = t.Attempts + 1
	t.OperatorFlaggedAt = now.Unix()
	w.cancel[id] = true
	appendRunActivityLocked(t, runActivityEvent{
		Source: "operator", Kind: "flag-stuck", At: now.Unix(), Frame: t.Frame, Attempt: t.OperatorFlagAttempt,
		Summary: "Operator flagged run as stuck", Detail: note,
	})
	attempt := t.OperatorFlagAttempt
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, map[string]any{"flagged": true, "attempt": attempt})
}

// operatorFlagFinish rewrites the cooperative-cancel stop of a flagged
// attempt into the stuck outcome. Only the stop the flag caused is
// rewritten: the LLM path reports a cancel as budget with no detail, the
// policy paths report cancelled. Any other outcome stands.
func (w *Wall) operatorFlagFinish(report *farm.FinishReport) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tiles[report.RunID]
	if !ok || t.OperatorFlag == "" {
		return
	}
	attempt := report.Attempt
	if attempt == 0 {
		attempt = t.Attempts + 1
		if t.Finished {
			attempt = t.Attempts
		}
	}
	if attempt != t.OperatorFlagAttempt {
		return
	}
	cancelStop := (report.Reason == "budget" && strings.TrimSpace(report.Detail) == "") || report.Reason == "cancelled"
	if !cancelStop {
		return
	}
	report.Reason = "stuck"
	report.Detail = "operator flagged: " + t.OperatorFlag
}

// reapFlaggedLocked settles a flagged attempt whose runner keeps
// heartbeating but never finishes (a wedged loop cannot honor the cancel).
// There is no finish dump in that case, so no issue is filed; the activity
// entry says so. Caller holds w.mu.
func (w *Wall) reapFlaggedLocked(t *Tile, now time.Time) bool {
	if t.Finished || t.Status == statusQueued || t.OperatorFlag == "" || t.OperatorFlagAttempt != t.Attempts+1 {
		return false
	}
	if now.Sub(time.Unix(t.OperatorFlaggedAt, 0)) < operatorFlagGrace {
		return false
	}
	w.settleRun(t, "stuck", fmt.Sprintf("operator flagged: %s (runner did not stop within %s; no finish dump)", t.OperatorFlag, operatorFlagGrace), now)
	return true
}
