package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

// operatorHTTPHandler adds explicit debugging/repro endpoints around the
// existing operator inspector. Runner protocol routes still fall through to
// wallHTTPHandler -> Wall.Handler unchanged.
func operatorHTTPHandler(w *Wall) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/runs/{id}/repro", w.handleQueueRepro)
	mux.HandleFunc("GET /v1/runs/{id}/repro-source", w.handleReproSource)
	mux.HandleFunc("GET /v1/runs/{id}/repro-checkpoint", w.handleRunReproCheckpoint)
	mux.HandleFunc("GET /v1/runs/{id}/checkpoints", w.handleListCheckpoints)
	mux.HandleFunc("GET /v1/runs/{id}/checkpoint", w.handleSourceCheckpoint)
	mux.Handle("/", wallHTTPHandler(w))
	return mux
}

type checkpointView struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Frame        uint64 `json:"frame,omitempty"`
	Round        int    `json:"round,omitempty"`
	Replayable   bool   `json:"replayable"`
	HasKnowledge bool   `json:"has_knowledge,omitempty"`
	Map          uint8  `json:"map,omitempty"`
	X            uint8  `json:"x,omitempty"`
	Y            uint8  `json:"y,omitempty"`
	Question     string `json:"question,omitempty"`
	Decision     string `json:"decision,omitempty"`
}

type periodicCheckpointMeta struct {
	Frame    uint64 `json:"frame"`
	Map      uint8  `json:"map"`
	X        uint8  `json:"x"`
	Y        uint8  `json:"y"`
	Question string `json:"question,omitempty"`
	Decision string `json:"decision,omitempty"`
}

func (w *Wall) handleQueueRepro(res http.ResponseWriter, req *http.Request) {
	sourceID := strings.TrimSpace(req.PathValue("id"))
	var in farm.ReplayRequest
	if req.Body != nil {
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil && !errors.Is(err, fs.ErrNotExist) {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad repro request: " + err.Error()})
			return
		}
	}
	run, report, err := w.loadRunInspection(sourceID)
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	if run.Planner != "llm" {
		writeJSON(res, http.StatusConflict, map[string]string{"error": "checkpoint repro currently requires an llm run with paired agent knowledge"})
		return
	}
	attempt := in.Attempt
	if attempt <= 0 {
		attempt = run.Attempts
		if report != nil && report.Attempt > attempt {
			attempt = report.Attempt
		}
		if attempt <= 0 {
			attempt = 1
		}
	}
	cp, err := w.replayCheckpoint(sourceID, attempt, run.Planner, in.Checkpoint)
	if err != nil {
		writeCheckpointError(res, err)
		return
	}

	newID := "replay-" + strings.TrimPrefix(newRunID(), "run-")
	source := farm.ReplaySource{
		SourceRunID:   sourceID,
		SourceAttempt: attempt,
		Checkpoint:    cp.State.Name,
		CreatedAt:     time.Now().Unix(),
	}
	if err := w.writeReplaySource(newID, source); err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "persist repro source: " + err.Error()})
		return
	}

	spec := farm.Spec{
		RunID:      newID,
		Seed:       run.Seed,
		Planner:    run.Planner,
		Starter:    run.Starter,
		Dest:       run.Dest,
		Goal:       run.Goal,
		LLMProfile: run.LLMProfile,
		FPS:        run.FPS,
		MaxRounds:  run.MaxRounds,
		MaxFrames:  run.MaxFrames,
		// A repro is deliberately one verification run, never a new endless
		// chain and never a randomized successor.
		Endless:    false,
		RandomSeed: false,
	}
	w.mu.Lock()
	if _, exists := w.tiles[newID]; exists {
		w.mu.Unlock()
		_ = os.Remove(w.replaySourcePath(newID))
		writeJSON(res, http.StatusConflict, map[string]string{"error": "generated repro run already exists"})
		return
	}
	w.order = append(w.order, newID)
	w.tiles[newID] = &Tile{}
	w.queue = append(w.queue, newID)
	w.applySpec(newID, spec)
	delete(w.cancel, newID)
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, farm.ReplayQueued{RunID: newID, Source: source})
}

func (w *Wall) handleReproSource(res http.ResponseWriter, req *http.Request) {
	source, err := w.readReplaySource(req.PathValue("id"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "run is not a checkpoint repro"})
			return
		}
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(res, http.StatusOK, source)
}

// handleRunReproCheckpoint is called by a leased runner. A normal run gets
// 204; a repro run gets the exact source checkpoint pinned when it was queued.
func (w *Wall) handleRunReproCheckpoint(res http.ResponseWriter, req *http.Request) {
	source, err := w.readReplaySource(req.PathValue("id"))
	if errors.Is(err, fs.ErrNotExist) {
		res.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	run, _, err := w.loadRunInspection(source.SourceRunID)
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	cp, err := w.replayCheckpoint(source.SourceRunID, source.SourceAttempt, run.Planner, source.Checkpoint)
	if err != nil {
		writeCheckpointError(res, err)
		return
	}
	writeJSON(res, http.StatusOK, cp)
}

// handleSourceCheckpoint lets local tooling materialize the same checkpoint a
// farm repro would use, without scheduling a run.
func (w *Wall) handleSourceCheckpoint(res http.ResponseWriter, req *http.Request) {
	run, report, err := w.loadRunInspection(req.PathValue("id"))
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	attempt := run.Attempts
	if report != nil && report.Attempt > attempt {
		attempt = report.Attempt
	}
	if raw := req.URL.Query().Get("attempt"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "attempt must be a positive integer"})
			return
		}
		attempt = parsed
	}
	if attempt <= 0 {
		attempt = 1
	}
	cp, err := w.replayCheckpoint(run.RunID, attempt, run.Planner, req.URL.Query().Get("name"))
	if err != nil {
		writeCheckpointError(res, err)
		return
	}
	writeJSON(res, http.StatusOK, cp)
}

func (w *Wall) handleListCheckpoints(res http.ResponseWriter, req *http.Request) {
	run, report, err := w.loadRunInspection(req.PathValue("id"))
	if err != nil {
		writeRunInspectError(res, err)
		return
	}
	attempt := run.Attempts
	if report != nil && report.Attempt > attempt {
		attempt = report.Attempt
	}
	if raw := req.URL.Query().Get("attempt"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "attempt must be a positive integer"})
			return
		}
		attempt = parsed
	}
	if attempt <= 0 {
		attempt = 1
	}
	views, err := w.checkpointViews(run.RunID, attempt, run.Planner)
	if err != nil {
		writeCheckpointError(res, err)
		return
	}
	writeJSON(res, http.StatusOK, map[string]any{
		"run_id":      run.RunID,
		"attempt":     attempt,
		"checkpoints": views,
	})
}

func (w *Wall) checkpointViews(runID string, attempt int, planner string) ([]checkpointView, error) {
	dir := checkpointAttemptDir(w.dumpsDir, runID, attempt)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".state") {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	out := make([]checkpointView, 0, len(names))
	for _, name := range names {
		view := checkpointView{Name: name}
		switch {
		case strings.HasPrefix(name, "round-"):
			view.Kind = "objective"
			view.Round, view.Frame = objectiveNumbers(name)
			view.HasKnowledge = matchingKnowledgeName(dir, name) != ""
			view.Replayable = planner != "llm" || view.HasKnowledge
		case strings.HasPrefix(name, "periodic-"):
			view.Kind = "periodic"
			view.Frame = periodicFrame(name)
			view.Replayable = planner != "llm"
			metaName := strings.TrimSuffix(name, ".state") + ".json"
			if data, err := os.ReadFile(filepath.Join(dir, metaName)); err == nil {
				var meta periodicCheckpointMeta
				if json.Unmarshal(data, &meta) == nil {
					view.Frame = meta.Frame
					view.Map, view.X, view.Y = meta.Map, meta.X, meta.Y
					view.Question, view.Decision = meta.Question, meta.Decision
				}
			}
		default:
			view.Kind = "state"
			view.Replayable = planner != "llm"
		}
		out = append(out, view)
	}
	return out, nil
}

func (w *Wall) replayCheckpoint(runID string, attempt int, planner, requested string) (*farm.ResumeCheckpoint, error) {
	if w.dumpsDir == "" {
		return nil, fmt.Errorf("checkpoint storage is not configured")
	}
	dir := checkpointAttemptDir(w.dumpsDir, runID, attempt)
	requested = strings.TrimSpace(requested)
	if requested == "" || requested == "latest" {
		cp, err := latestResumeCheckpoint(dir, planner)
		if err != nil {
			return nil, err
		}
		cp.Attempt = attempt
		return &cp, nil
	}
	if filepath.Base(requested) != requested || !strings.HasSuffix(requested, ".state") {
		return nil, fmt.Errorf("invalid checkpoint name %q", requested)
	}
	state, err := checkpointArtifact(dir, requested, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	cp := &farm.ResumeCheckpoint{Attempt: attempt, State: state}
	if planner == "llm" {
		knowledgeName := matchingKnowledgeName(dir, requested)
		if knowledgeName == "" {
			return nil, fmt.Errorf("checkpoint %s has no paired agent knowledge", requested)
		}
		knowledge, err := checkpointArtifact(dir, knowledgeName, "application/json")
		if err != nil {
			return nil, err
		}
		cp.Knowledge = &knowledge
	}
	arts := []farm.Artifact{cp.State}
	if cp.Knowledge != nil {
		arts = append(arts, *cp.Knowledge)
	}
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: arts}); err != nil {
		return nil, err
	}
	return cp, nil
}

func matchingKnowledgeName(dir, stateName string) string {
	base := strings.TrimSuffix(stateName, ".state")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best := ""
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, base+".knowledge-v") && strings.HasSuffix(name, ".json") && name > best {
			best = name
		}
	}
	return best
}

func objectiveNumbers(name string) (round int, frame uint64) {
	_, _ = fmt.Sscanf(name, "round-%d-frame-%d-", &round, &frame)
	return round, frame
}

func periodicFrame(name string) uint64 {
	var frame uint64
	_, _ = fmt.Sscanf(name, "periodic-%d.state", &frame)
	return frame
}

func (w *Wall) replaySourcePath(runID string) string {
	return filepath.Join(w.dumpsDir, "repros", safeBase(runID)+".json")
}

func (w *Wall) writeReplaySource(runID string, source farm.ReplaySource) error {
	if w.dumpsDir == "" {
		return fmt.Errorf("checkpoint storage is not configured")
	}
	dir := filepath.Join(w.dumpsDir, "repros")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(source)
	if err != nil {
		return err
	}
	return writeAtomic(w.replaySourcePath(runID), data, 0o644)
}

func (w *Wall) readReplaySource(runID string) (farm.ReplaySource, error) {
	var source farm.ReplaySource
	if w.dumpsDir == "" {
		return source, fs.ErrNotExist
	}
	data, err := os.ReadFile(w.replaySourcePath(runID))
	if err != nil {
		return source, err
	}
	if err := json.Unmarshal(data, &source); err != nil {
		return source, fmt.Errorf("decode repro source: %w", err)
	}
	if source.SourceRunID == "" || source.SourceAttempt <= 0 || source.Checkpoint == "" {
		return source, fmt.Errorf("invalid repro source metadata")
	}
	return source, nil
}

func writeCheckpointError(res http.ResponseWriter, err error) {
	if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "checkpoint not found"})
		return
	}
	writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
}
