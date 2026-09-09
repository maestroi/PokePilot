package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

const majorCheckpointPrefix = "major-badge-"

func (w *Wall) handleCheckpoint(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var incoming struct {
		farm.CheckpointReport
		Resume bool `json:"resume,omitempty"`
	}
	if err := json.NewDecoder(req.Body).Decode(&incoming); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad checkpoint: " + err.Error()})
		return
	}
	report := incoming.CheckpointReport
	if report.RunID != "" && report.RunID != id {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "run_id mismatch: path " + id + " body " + report.RunID})
		return
	}
	report.RunID = id
	if incoming.Resume {
		w.handleCheckpointResume(res, id, report.Attempt)
		return
	}
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

// handleCheckpointResume has two recovery tiers. A lease after worker loss
// resumes from the newest ordinary objective boundary, preserving the exact
// work that runner had completed. An endless run retrying a gameplay/code
// error instead rolls back to the newest durable major checkpoint, so a bad
// later decision does not erase badges already secured. Non-endless error
// retries intentionally retain their historical fresh-game semantics.
func (w *Wall) handleCheckpointResume(res http.ResponseWriter, id string, requestedAttempt int) {
	if w.dumpsDir == "" {
		res.WriteHeader(http.StatusNoContent)
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
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already finished: " + id})
		return
	}
	attempt := t.Attempts + 1
	if requestedAttempt != 0 && requestedAttempt != attempt {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("stale resume: run is on attempt %d, request claims %d", attempt, requestedAttempt),
		})
		return
	}
	previous := attempt - 1
	retryPrefix := fmt.Sprintf("attempt %d failed: ", previous)
	lostPrefix := retryPrefix + "no heartbeat for "
	planner := t.Planner
	lostRetry := previous > 0 && strings.HasPrefix(t.Detail, lostPrefix)
	majorRetry := previous > 0 && t.Endless && planner == "llm" && strings.HasPrefix(t.Detail, retryPrefix) && !lostRetry
	w.mu.Unlock()

	var (
		cp  farm.ResumeCheckpoint
		err error
	)
	switch {
	case lostRetry:
		cp, err = latestResumeCheckpoint(checkpointAttemptDir(w.dumpsDir, id, previous), planner)
		if err == nil {
			cp.Attempt = previous
		}
	case majorRetry:
		cp, err = latestMajorResumeCheckpoint(w.dumpsDir, id, previous)
	default:
		res.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("pokewall: %s attempt %d resume checkpoint: %v", id, previous, err)
		}
		// Resume is recovery, never a new reason for the run to fail. The
		// runner interprets 204 as a clean fresh-start fallback.
		res.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(res, http.StatusOK, cp)
}

func latestResumeCheckpoint(dir, planner string) (farm.ResumeCheckpoint, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return farm.ResumeCheckpoint{}, err
	}
	var objective, periodic []string
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		names = append(names, name)
		switch {
		case strings.HasPrefix(name, "round-") && strings.HasSuffix(name, ".state"):
			objective = append(objective, name)
		case strings.HasPrefix(name, "periodic-") && strings.HasSuffix(name, ".state"):
			periodic = append(periodic, name)
		}
	}
	sort.Strings(objective)
	sort.Strings(periodic)

	// LLM state and learned knowledge must come from one objective boundary.
	// A newer periodic emulator state without matching knowledge can make the
	// planner reason from a world it does not remember, so prefer consistency
	// over squeezing out the final partial objective.
	if planner == "llm" {
		return latestPairedCheckpoint(dir, objective, names)
	}

	// Scripted runs have no agent knowledge, so their latest periodic emulator
	// state is a complete resume point.
	if len(periodic) == 0 {
		return farm.ResumeCheckpoint{}, os.ErrNotExist
	}
	stateArt, err := checkpointArtifact(dir, periodic[len(periodic)-1], "application/octet-stream")
	if err != nil {
		return farm.ResumeCheckpoint{}, err
	}
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: []farm.Artifact{stateArt}}); err != nil {
		return farm.ResumeCheckpoint{}, err
	}
	return farm.ResumeCheckpoint{State: stateArt}, nil
}

func latestMajorResumeCheckpoint(dumpsDir, runID string, throughAttempt int) (farm.ResumeCheckpoint, error) {
	for attempt := throughAttempt; attempt >= 1; attempt-- {
		dir := checkpointAttemptDir(dumpsDir, runID, attempt)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return farm.ResumeCheckpoint{}, err
		}
		var states, names []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			names = append(names, name)
			if strings.HasPrefix(name, majorCheckpointPrefix) && strings.HasSuffix(name, ".state") {
				states = append(states, name)
			}
		}
		sort.Strings(states)
		cp, err := latestPairedCheckpoint(dir, states, names)
		if err == nil {
			cp.Attempt = attempt
			return cp, nil
		}
		if !os.IsNotExist(err) {
			return farm.ResumeCheckpoint{}, err
		}
	}
	return farm.ResumeCheckpoint{}, os.ErrNotExist
}

func latestPairedCheckpoint(dir string, states, names []string) (farm.ResumeCheckpoint, error) {
	for i := len(states) - 1; i >= 0; i-- {
		stateName := states[i]
		base := strings.TrimSuffix(stateName, ".state")
		knowledgeName := ""
		for _, name := range names {
			if strings.HasPrefix(name, base+".knowledge-v") && strings.HasSuffix(name, ".json") {
				if knowledgeName == "" || name > knowledgeName {
					knowledgeName = name
				}
			}
		}
		if knowledgeName == "" {
			continue
		}
		stateArt, err := checkpointArtifact(dir, stateName, "application/octet-stream")
		if err != nil {
			continue
		}
		knowledgeArt, err := checkpointArtifact(dir, knowledgeName, "application/json")
		if err != nil {
			continue
		}
		if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: []farm.Artifact{stateArt, knowledgeArt}}); err != nil {
			continue
		}
		return farm.ResumeCheckpoint{State: stateArt, Knowledge: &knowledgeArt}, nil
	}
	return farm.ResumeCheckpoint{}, os.ErrNotExist
}

func checkpointArtifact(dir, name, mediaType string) (farm.Artifact, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return farm.Artifact{}, err
	}
	sum := sha256.Sum256(data)
	return farm.Artifact{
		Name:      name,
		MediaType: mediaType,
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}, nil
}

func checkpointAttemptDir(dumpsDir, runID string, attempt int) string {
	return filepath.Join(dumpsDir, "checkpoints", safeBase(runID), fmt.Sprintf("%d", attempt))
}

func retainCheckpointWindow(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var periodic, objective, major []string
	files := map[string]struct{}{}
	for _, e := range entries {
		name := e.Name()
		files[name] = struct{}{}
		switch {
		case strings.HasPrefix(name, "periodic-") && strings.HasSuffix(name, ".state"):
			periodic = append(periodic, name)
		case strings.HasPrefix(name, "round-") && strings.HasSuffix(name, ".state"):
			objective = append(objective, name)
		case strings.HasPrefix(name, majorCheckpointPrefix) && strings.HasSuffix(name, ".state"):
			major = append(major, name)
		}
	}
	sort.Strings(periodic)
	sort.Strings(objective)
	sort.Strings(major)
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
	knowledgeSidecar := func(name string) string {
		base := strings.TrimSuffix(name, ".state")
		for f := range files {
			if strings.HasPrefix(f, base+".knowledge-v") && strings.HasSuffix(f, ".json") {
				return f
			}
		}
		return ""
	}
	drop(periodic, checkpointPeriodicKeep, func(name string) string {
		return strings.TrimSuffix(name, ".state") + ".json"
	})
	drop(objective, checkpointObjectiveKeep, knowledgeSidecar)
	// Major checkpoints never compete with the short objective flight
	// recorder. Keep a separate small ring so an hours-long campaign can roll
	// back across recent badges without unbounded storage growth.
	drop(major, checkpointMajorKeep, knowledgeSidecar)
	return nil
}
