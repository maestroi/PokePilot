package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

func (w *Wall) handleCheckpoint(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var report farm.CheckpointReport
	if err := json.NewDecoder(req.Body).Decode(&report); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad checkpoint: " + err.Error()})
		return
	}
	if report.RunID != "" && report.RunID != id {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "run_id mismatch: path " + id + " body " + report.RunID})
		return
	}
	report.RunID = id
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: report.Artifacts}); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if w.dumpsDir == "" {
		writeJSON(res, http.StatusServiceUnavailable, map[string]string{"error": "checkpoint storage is not configured"})
		return
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
		writeJSON(res, http.StatusConflict, map[string]string{"error": "late checkpoint: run already finished"})
		return
	}
	wantAttempt := t.Attempts + 1
	if report.Attempt != 0 && report.Attempt != wantAttempt {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("stale checkpoint: run is on attempt %d, report claims %d", wantAttempt, report.Attempt),
		})
		return
	}
	attempt := wantAttempt
	w.mu.Unlock()

	dir := checkpointAttemptDir(w.dumpsDir, id, attempt)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "checkpoint dir: " + err.Error()})
		return
	}

	type pending struct{ tmp, final string }
	var staged []pending
	cleanup := func() {
		for _, p := range staged {
			os.Remove(p.tmp)
		}
	}
	for _, a := range report.Artifacts {
		final := filepath.Join(dir, a.Name)
		tmp, err := os.CreateTemp(dir, ".ckpt-*")
		if err != nil {
			cleanup()
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "checkpoint temp: " + err.Error()})
			return
		}
		if _, err := tmp.Write(a.Data); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			cleanup()
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "checkpoint write: " + err.Error()})
			return
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmp.Name())
			cleanup()
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "checkpoint close: " + err.Error()})
			return
		}
		staged = append(staged, pending{tmp: tmp.Name(), final: final})
	}

	w.mu.Lock()
	t = w.tiles[id]
	if t == nil || t.Finished || t.Attempts+1 != attempt {
		w.mu.Unlock()
		cleanup()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "late checkpoint: run is no longer the active attempt"})
		return
	}
	w.mu.Unlock()

	for _, p := range staged {
		if err := os.Rename(p.tmp, p.final); err != nil {
			cleanup()
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "checkpoint rename: " + err.Error()})
			return
		}
	}
	if err := retainCheckpointWindow(dir); err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "checkpoint retain: " + err.Error()})
		return
	}
	writeJSON(res, http.StatusOK, map[string]string{"status": "ok"})
}

func checkpointAttemptDir(dumpsDir, runID string, attempt int) string {
	return filepath.Join(dumpsDir, "checkpoints", safeBase(runID), fmt.Sprintf("%d", attempt))
}

func retainCheckpointWindow(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var periodic, objective []string
	files := map[string]struct{}{}
	for _, e := range entries {
		name := e.Name()
		files[name] = struct{}{}
		switch {
		case strings.HasPrefix(name, "periodic-") && strings.HasSuffix(name, ".state"):
			periodic = append(periodic, name)
		case strings.HasPrefix(name, "round-") && strings.HasSuffix(name, ".state"):
			objective = append(objective, name)
		}
	}
	sort.Strings(periodic)
	sort.Strings(objective)
	drop := func(names []string, keep int, sidecar func(string) string) {
		if keep < 0 {
			keep = 0
		}
		n := len(names) - keep
		if n <= 0 {
			return
		}
		for _, name := range names[:n] {
			os.Remove(filepath.Join(dir, name))
			if side := sidecar(name); side != "" {
				os.Remove(filepath.Join(dir, side))
			}
		}
	}
	drop(periodic, checkpointPeriodicKeep, func(name string) string {
		return strings.TrimSuffix(name, ".state") + ".json"
	})
	drop(objective, checkpointObjectiveKeep, func(name string) string {
		base := strings.TrimSuffix(name, ".state")
		for f := range files {
			if strings.HasPrefix(f, base+".knowledge-v") && strings.HasSuffix(f, ".json") {
				return f
			}
		}
		return ""
	})
	return nil
}
