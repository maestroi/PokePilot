package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	statusPaused             = "paused"
	autoPauseRepeatThreshold = 2
)

type pauseKey struct {
	wall  *Wall
	runID string
}

type pauseIntent struct {
	note string
}

type failureStreak struct {
	fingerprint string
	count       int
}

var pauseIntents sync.Map // pauseKey -> pauseIntent

var repeatedFailures = struct {
	sync.Mutex
	m map[pauseKey]failureStreak
}{m: make(map[pauseKey]failureStreak)}

type pauseFinishSnapshot struct {
	row       tileRow
	lastFrame []byte
	ok        bool
}

// pauseHTTPHandler layers pause/resume semantics around either the legacy
// runtime handler or the PostgreSQL-backed operator handler. Pause deliberately
// reuses the existing cooperative cancel signal, so old runners stop at the
// same safe boundary without a new wire-protocol field.
func pauseHTTPHandler(w *Wall, next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			if id, ok := runActionID(req.URL.Path, "pause"); ok {
				w.handlePause(res, req, id)
				return
			}
			if id, ok := runActionID(req.URL.Path, "resume"); ok {
				w.handleResume(res, req, id)
				return
			}
			if id, ok := runActionID(req.URL.Path, "cancel"); ok && w.cancelPausedRun(res, id) {
				return
			}
			if id, ok := runActionID(req.URL.Path, "finish"); ok {
				before := w.pauseFinishSnapshot(id)
				capture := &statusCaptureWriter{ResponseWriter: res}
				next.ServeHTTP(capture, req)
				if capture.status >= 200 && capture.status < 300 {
					if !w.finalizeRequestedPause(id) {
						circuitPaused := false
						if report, ok := finishReportFromRequest(req); ok {
							if circuitCanaryAdvanced(before.row, report) {
								w.releaseCircuitPeers(before.row.CircuitKey, id)
							}
							if cp := controlPlaneFor(w); cp != nil {
								attempt := report.Attempt
								if attempt <= 0 {
									attempt = before.row.Attempts + 1
								}
								decision, err := cp.failureCircuitDecision(before.row, report, attempt)
								if err != nil {
									log.Printf("pokewall: failure circuit %s/%d: %v", id, attempt, err)
								} else if decision.Open {
									circuitPaused = w.pauseForFailureCircuit(id, before, report, decision)
								}
							}
						}
						if !circuitPaused {
							w.maybeAutoPauseRepeatedFailure(id, before)
						}
					}
				}
				return
			}
		}
		next.ServeHTTP(res, req)
	})
}

func runActionID(path, action string) (string, bool) {
	const prefix = "/v1/runs/"
	suffix := "/" + action
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if raw == "" || strings.Contains(raw, "/") {
		return "", false
	}
	id, err := url.PathUnescape(raw)
	if err != nil || id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

func (w *Wall) handlePause(res http.ResponseWriter, _ *http.Request, id string) {
	now := time.Now()
	w.mu.Lock()
	t := w.tiles[id]
	if t == nil {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
		return
	}
	if t.Status == statusPaused {
		w.mu.Unlock()
		writeJSON(res, http.StatusOK, map[string]string{"status": statusPaused})
		return
	}
	if t.Finished || t.Status == statusDone {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already finished: " + id})
		return
	}

	if t.Status == statusQueued {
		w.queue = removeID(w.queue, id)
		t.Status = statusPaused
		t.Finished = true
		t.EndedAt = now
		t.StopSoFar = "paused by operator"
		t.lastUpdate = now
		delete(w.cancel, id)
		w.mu.Unlock()
		pauseIntents.Delete(pauseKey{wall: w, runID: id})
		w.saveState()
		writeJSON(res, http.StatusOK, map[string]string{"status": statusPaused})
		return
	}

	// Active runners already understand Cancel. Mark the semantic intent first,
	// then use that existing signal; the finish wrapper converts the resulting
	// settled cancellation into a resumable paused run.
	pauseIntents.Store(pauseKey{wall: w, runID: id}, pauseIntent{note: "paused by operator"})
	w.cancel[id] = true
	t.StopSoFar = "pause requested"
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, map[string]string{"status": "pausing"})
}

func (w *Wall) handleResume(res http.ResponseWriter, _ *http.Request, id string) {
	now := time.Now()
	w.mu.Lock()
	t := w.tiles[id]
	if t == nil {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
		return
	}
	if t.Status != statusPaused {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run is not paused: " + id})
		return
	}

	w.queue = removeID(w.queue, id)
	t.Status = statusQueued
	t.Finished = false
	t.EndedAt = time.Time{}
	t.Reason = ""
	t.StopSoFar = ""
	// Both checkpoint backends already define a lost-worker retry as an exact
	// resume of the previous attempt (newest consistent objective pair). A
	// paused run needs precisely that recovery policy, rather than the ordinary
	// error rollback to a major badge. This internal marker is only present
	// while queued/leased and is replaced by the next settlement.
	if t.Attempts > 0 {
		t.Detail = fmt.Sprintf("attempt %d failed: no heartbeat for paused resume", t.Attempts)
	} else {
		t.Detail = ""
	}
	// A human resume means a fix/new deployment gets a fresh retry budget while
	// Attempts remains monotonic for stale-generation protection and checkpoint
	// lineage.
	t.ErrorAttempts = 0
	t.LossRecoveries = 0
	clearTileCircuit(t)
	t.workerAddrs = nil
	t.lastUpdate = now
	delete(w.cancel, id)
	w.queue = append(w.queue, id)
	w.mu.Unlock()

	pauseIntents.Delete(pauseKey{wall: w, runID: id})
	clearFailureStreak(w, id)
	w.saveState()
	writeJSON(res, http.StatusOK, map[string]string{"status": statusQueued})
}

func (w *Wall) cancelPausedRun(res http.ResponseWriter, id string) bool {
	w.mu.Lock()
	t := w.tiles[id]
	if t == nil || t.Status != statusPaused {
		w.mu.Unlock()
		return false
	}
	t.Status = statusDone
	t.Finished = true
	t.EndedAt = time.Now()
	t.Reason = "cancelled"
	t.Detail = "cancelled while paused"
	t.StopSoFar = ""
	delete(w.cancel, id)
	w.queue = removeID(w.queue, id)
	w.mu.Unlock()

	pauseIntents.Delete(pauseKey{wall: w, runID: id})
	clearFailureStreak(w, id)
	w.saveState()
	writeJSON(res, http.StatusOK, map[string]bool{"cancel": true})
	return true
}

func (w *Wall) pauseFinishSnapshot(id string) pauseFinishSnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.tiles[id]
	if t == nil {
		return pauseFinishSnapshot{}
	}
	return pauseFinishSnapshot{
		row:       w.tileRowLocked(t),
		lastFrame: append([]byte(nil), t.lastFrame...),
		ok:        true,
	}
}

func (w *Wall) finalizeRequestedPause(id string) bool {
	key := pauseKey{wall: w, runID: id}
	value, ok := pauseIntents.LoadAndDelete(key)
	if !ok {
		return false
	}
	intent := value.(pauseIntent)

	w.mu.Lock()
	t := w.tiles[id]
	if t == nil || !t.Finished {
		w.mu.Unlock()
		// A rejected/stale finish should not lose the operator's pause request.
		pauseIntents.Store(key, intent)
		return false
	}
	t.Status = statusPaused
	t.StopSoFar = intent.note
	w.mu.Unlock()
	w.saveState()
	return true
}

func retryFailureDetail(detail string) (string, bool) {
	if !strings.HasPrefix(detail, "attempt ") {
		return "", false
	}
	const marker = " failed: "
	i := strings.Index(detail, marker)
	if i < 0 || i+len(marker) >= len(detail) {
		return "", false
	}
	return detail[i+len(marker):], true
}

func recordFailureStreak(w *Wall, id, detail string) int {
	key := pauseKey{wall: w, runID: id}
	fingerprint := normalizeDetail(detail)
	repeatedFailures.Lock()
	defer repeatedFailures.Unlock()
	streak := repeatedFailures.m[key]
	if streak.fingerprint == fingerprint && fingerprint != "" {
		streak.count++
	} else {
		streak = failureStreak{fingerprint: fingerprint, count: 1}
	}
	repeatedFailures.m[key] = streak
	return streak.count
}

func clearFailureStreak(w *Wall, id string) {
	repeatedFailures.Lock()
	delete(repeatedFailures.m, pauseKey{wall: w, runID: id})
	repeatedFailures.Unlock()
}

func (w *Wall) maybeAutoPauseRepeatedFailure(id string, before pauseFinishSnapshot) bool {
	if !before.ok {
		return false
	}

	w.mu.Lock()
	t := w.tiles[id]
	if t == nil {
		w.mu.Unlock()
		return false
	}
	// settleRun increments only ErrorAttempts for genuine run errors. Comparing
	// the pre-finish snapshot avoids treating lost-worker retries as gameplay
	// failures even though both use the same "attempt N failed" detail shape.
	isErrorRetry := t.ErrorAttempts > before.row.ErrorAttempts && t.Status == statusQueued && !t.Finished
	detail, ok := retryFailureDetail(t.Detail)
	attempts := t.Attempts
	terminalDone := t.Finished && t.Status == statusDone
	w.mu.Unlock()
	if !isErrorRetry || !ok {
		if terminalDone {
			clearFailureStreak(w, id)
		}
		return false
	}

	if recordFailureStreak(w, id, detail) < autoPauseRepeatThreshold {
		return false
	}

	now := time.Now()
	w.mu.Lock()
	t = w.tiles[id]
	if t == nil || t.Status != statusQueued || t.Finished || t.Attempts != attempts {
		w.mu.Unlock()
		return false
	}
	w.queue = removeID(w.queue, id)
	t.Status = statusPaused
	// Paused is a settled generation, so Finished becomes true until Resume.
	// This keeps the stale-run reaper from converting an intentionally paused
	// run into a worker-loss retry while the public status remains "paused".
	t.Finished = true
	t.EndedAt = now
	t.Reason = "error"
	t.Detail = detail
	t.StopSoFar = fmt.Sprintf("auto-paused after %d repeated failures", autoPauseRepeatThreshold)
	t.lastUpdate = now
	// The ordinary retry path clears live state before requeueing. Put the last
	// pre-finish snapshot back so the paused card still shows where the bug was.
	t.Seed = before.row.Seed
	t.Frame = before.row.Frame
	t.Map = before.row.Map
	t.X = before.row.X
	t.Y = before.row.Y
	t.Trace = before.row.Trace
	t.Question = before.row.Question
	t.Decision = before.row.Decision
	t.Raw = before.row.Raw
	t.Sprites = append(t.Sprites[:0], before.row.Sprites...)
	t.Trail = append(t.Trail[:0], before.row.Trail...)
	t.Stats = before.row.Stats
	t.Player = before.row.Player
	t.lastFrame = append(t.lastFrame[:0], before.lastFrame...)
	w.mu.Unlock()
	w.saveState()
	return true
}
