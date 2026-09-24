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
	if err := farm.ValidateCheckpointReport(report); err != nil {
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

// handleCheckpointResume keeps retries close to safe progress without restoring
// directly into the state that just failed. Worker loss first resumes the exact
// previous attempt. Endless gameplay/code retries and failed successors prefer
// the newest consistent objective state+knowledge pair anywhere in the lineage,
// with durable badge checkpoints as their fallback. Ordinary LLM retries use
// only the newest durable post-gym checkpoint, so a stuck run skips completed
// gyms without being restored immediately beside the fault that triggered it.
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
	resilientRetry := previous > 0 && t.RecoveryProfile.Resilient() && planner == "llm" &&
		strings.HasPrefix(t.Detail, retryPrefix) && !lostRetry
	recoveryAttempts := t.RecoveryAttempts
	endlessRetry := previous > 0 && t.Endless && planner == "llm" && strings.HasPrefix(t.Detail, retryPrefix) && !lostRetry
	gymRetry := previous > 0 && !t.Endless && planner == "llm" && strings.HasPrefix(t.Detail, retryPrefix) && !lostRetry
	lineageRetry := previous == 0 && t.Endless && planner == "llm" && t.ResumeFromRunID != ""
	resumeParent := t.ResumeFromRunID
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
		} else if os.IsNotExist(err) && planner == "llm" {
			// The lost worker may have disappeared before writing a fresh
			// objective pair. Prefer the newest older ordinary checkpoint in
			// the lineage, and only then fall back to a durable badge snapshot.
			cp, err = w.latestLineageResumeCheckpoint(id, planner)
			if os.IsNotExist(err) {
				cp, err = w.latestLineageMajorCheckpoint(id)
			}
		}
	case resilientRetry:
		cp, err = w.resilientResumeCheckpoint(id, planner, recoveryAttempts)
	case endlessRetry:
		cp, err = w.latestLineageResumeCheckpoint(id, planner)
		if os.IsNotExist(err) {
			cp, err = w.latestLineageMajorCheckpoint(id)
		}
	case gymRetry:
		cp, err = w.latestLineageMajorCheckpoint(id)
	case lineageRetry:
		cp, err = w.latestLineageResumeCheckpoint(resumeParent, planner)
		if os.IsNotExist(err) {
			cp, err = w.latestLineageMajorCheckpoint(resumeParent)
		}
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
		w.mu.Lock()
		if current := w.tiles[id]; current != nil && !current.Finished {
			appendRunActivityLocked(current, runActivityEvent{
				Source: "recovery", Kind: "fresh_start", Attempt: attempt,
				RecoveryAttempt: recoveryAttempts,
				Summary:         "No usable checkpoint; starting fresh",
				Detail:          fmt.Sprintf("attempt %d recovery fallback", attempt),
			})
		}
		w.mu.Unlock()
		w.saveStateSoon()
		res.WriteHeader(http.StatusNoContent)
		return
	}
	kind := "resume"
	summary := "Resuming from checkpoint"
	if strings.HasPrefix(cp.State.Name, majorCheckpointPrefix) {
		kind = "rollback"
		summary = "Rolling back to major checkpoint"
	}
	w.mu.Lock()
	if current := w.tiles[id]; current != nil && !current.Finished {
		appendRunActivityLocked(current, runActivityEvent{
			Source: "recovery", Kind: kind, Attempt: attempt,
			RecoveryAttempt: recoveryAttempts,
			Summary:         summary,
			Detail:          cp.State.Name,
		})
	}
	w.mu.Unlock()
	w.saveStateSoon()
	writeJSON(res, http.StatusOK, cp)
}

// resilientResumeCheckpoint turns repeated no-progress failures into a
// deterministic rollback ladder. First retry stays near the fault so a changed
// planner decision/seed can recover cheaply. Further failures back up across
// major milestones one at a time; once no older retained milestone exists the
// 204 path deliberately falls back to a fresh cartridge.
func (w *Wall) resilientResumeCheckpoint(startID, planner string, recoveryAttempts int) (farm.ResumeCheckpoint, error) {
	if recoveryAttempts <= 1 {
		cp, err := w.latestLineageResumeCheckpoint(startID, planner)
		if err == nil {
			return cp, nil
		}
		if !os.IsNotExist(err) {
			return farm.ResumeCheckpoint{}, err
		}
		return w.latestLineageMajorCheckpoint(startID)
	}
	return w.latestLineageMajorCheckpointRollback(startID, recoveryAttempts-2)
}

func (w *Wall) latestLineageMajorCheckpointRollback(startID string, rollback int) (farm.ResumeCheckpoint, error) {
	latest, err := w.latestLineageMajorCheckpoint(startID)
	if err != nil {
		return farm.ResumeCheckpoint{}, err
	}
	if rollback <= 0 {
		return latest, nil
	}
	badge, ok := majorCheckpointBadge(latest.State.Name)
	if !ok || badge-rollback < 1 {
		return farm.ResumeCheckpoint{}, os.ErrNotExist
	}
	return w.latestLineageMajorCheckpointAtOrBelow(startID, badge-rollback)
}

// objectiveFrame is the cumulative emulator frame embedded in an objective
// checkpoint name. Round numbers restart on every resume, so the frame is the
// monotonic progress signal across carried-over and attempt-local checkpoints.
func objectiveFrame(name string) uint64 {
	_, frame := objectiveNumbers(name)
	return frame
}

// sortByFrame orders objective checkpoints by progress. Stable ordering keeps
// filename order for equal frames, matching the previous behavior.
func sortByFrame(names []string) {
	sort.SliceStable(names, func(i, j int) bool {
		return objectiveFrame(names[i]) < objectiveFrame(names[j])
	})
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
	sortByFrame(objective)
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

// latestLineageResumeCheckpoint returns the deepest usable ordinary checkpoint
// in a run's lineage. The frame embedded in the checkpoint name is cumulative
// emulator progress, so it is the only comparable ordering across attempts:
// picking the newest attempt instead would let an attempt that booted fresh —
// and therefore wrote only early checkpoints before dying — outrank the deep
// progress an earlier attempt actually reached, and the recovery would quietly
// replay hours of gameplay. Attempts are searched newest-first so an equally
// deep candidate still prefers the most recent one, and a candidate that cannot
// be read is skipped rather than ending the search.
func (w *Wall) latestLineageResumeCheckpoint(startID, planner string) (farm.ResumeCheckpoint, error) {
	seen := map[string]struct{}{}
	var best farm.ResumeCheckpoint
	bestFrame := uint64(0)
	found := false
	id := startID
	for id != "" {
		if _, dup := seen[id]; dup {
			break
		}
		seen[id] = struct{}{}
		w.mu.Lock()
		t := w.tiles[id]
		through := 0
		parent := ""
		if t != nil {
			through = t.Attempts
			parent = t.ResumeFromRunID
		}
		w.mu.Unlock()
		for attempt := through; attempt >= 1; attempt-- {
			cp, err := latestResumeCheckpoint(checkpointAttemptDir(w.dumpsDir, id, attempt), planner)
			if err == nil {
				cp.Attempt = attempt
				if frame := objectiveFrame(cp.State.Name); !found || frame > bestFrame {
					best, bestFrame, found = cp, frame, true
				}
				continue
			}
			if !os.IsNotExist(err) {
				return farm.ResumeCheckpoint{}, err
			}
		}
		id = parent
	}
	if found {
		return best, nil
	}
	return farm.ResumeCheckpoint{}, os.ErrNotExist
}

func majorCheckpointBadge(name string) (int, bool) {
	if !strings.HasPrefix(name, majorCheckpointPrefix) || !strings.HasSuffix(name, ".state") {
		return 0, false
	}
	var badge int
	if _, err := fmt.Sscanf(name, "major-badge-%d-", &badge); err != nil || badge < 1 || badge > 8 {
		return 0, false
	}
	return badge, true
}

func (w *Wall) latestLineageMajorCheckpoint(startID string) (farm.ResumeCheckpoint, error) {
	return w.latestLineageMajorCheckpointAtOrBelow(startID, 8)
}

func (w *Wall) latestLineageMajorCheckpointAtOrBelow(startID string, maxBadge int) (farm.ResumeCheckpoint, error) {
	seen := map[string]struct{}{}
	id := startID
	bestBadge := 0
	var best farm.ResumeCheckpoint
	found := false
	for id != "" {
		if _, dup := seen[id]; dup {
			break
		}
		seen[id] = struct{}{}
		w.mu.Lock()
		t := w.tiles[id]
		through := 0
		parent := ""
		if t != nil {
			through = t.Attempts
			parent = t.ResumeFromRunID
		}
		w.mu.Unlock()
		if through > 0 {
			cp, err := latestMajorResumeCheckpointAtOrBelow(w.dumpsDir, id, through, maxBadge)
			if err == nil {
				badge, ok := majorCheckpointBadge(cp.State.Name)
				if ok && (!found || badge > bestBadge) {
					best = cp
					bestBadge = badge
					found = true
				}
			} else if !os.IsNotExist(err) {
				return farm.ResumeCheckpoint{}, err
			}
		}
		id = parent
	}
	if found {
		return best, nil
	}
	return farm.ResumeCheckpoint{}, os.ErrNotExist
}

func latestMajorResumeCheckpoint(dumpsDir, runID string, throughAttempt int) (farm.ResumeCheckpoint, error) {
	return latestMajorResumeCheckpointAtOrBelow(dumpsDir, runID, throughAttempt, 8)
}

func latestMajorResumeCheckpointAtOrBelow(dumpsDir, runID string, throughAttempt, maxBadge int) (farm.ResumeCheckpoint, error) {
	bestBadge := 0
	var best farm.ResumeCheckpoint
	found := false
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
			if badge, ok := majorCheckpointBadge(name); ok && badge <= maxBadge {
				states = append(states, name)
			}
		}
		sort.Strings(states)
		cp, err := latestPairedCheckpoint(dir, states, names)
		if err == nil {
			badge, ok := majorCheckpointBadge(cp.State.Name)
			if ok && (!found || badge > bestBadge) {
				cp.Attempt = attempt
				best = cp
				bestBadge = badge
				found = true
			}
			continue
		}
		if !os.IsNotExist(err) {
			return farm.ResumeCheckpoint{}, err
		}
	}
	if found {
		return best, nil
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
	art := farm.Artifact{
		Name:      name,
		MediaType: mediaType,
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}
	// A state that is no longer a complete save must never be served as a
	// resume candidate: the caller skips it and keeps looking for an older
	// usable pair, so a truncated file cannot restart a run from scratch.
	if err := farm.ValidateCheckpointState(art); err != nil {
		return farm.Artifact{}, err
	}
	return art, nil
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
	sortByFrame(objective)
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
