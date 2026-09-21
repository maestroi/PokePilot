package main

import (
	"net/http"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

type cloneRunResult struct {
	RunID      string `json:"run_id"`
	ClonedFrom string `json:"cloned_from"`
	Status     string `json:"status"`
}

// handleCloneRun starts a fresh run from the selected run's execution
// configuration. Runtime state, attempts, checkpoints and experiment
// bookkeeping are deliberately not copied: a manual clone is an independent
// run that starts from the beginning with the same gameplay/model settings.
func (w *Wall) handleCloneRun(res http.ResponseWriter, req *http.Request) {
	sourceID := strings.TrimSpace(req.PathValue("id"))
	if sourceID == "" {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "run id is required"})
		return
	}

	source, ok := w.ramRow(sourceID)
	if !ok {
		source, ok = w.catalogSnapshotRun(sourceID)
	}
	if !ok {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + sourceID})
		return
	}

	w.mu.Lock()
	cloneID := newRunID()
	for w.tiles[cloneID] != nil {
		cloneID = newRunID()
	}
	w.order = append(w.order, cloneID)
	w.tiles[cloneID] = &Tile{}
	w.queue = append(w.queue, cloneID)
	w.applySpec(cloneID, farm.Spec{
		RunID:           cloneID,
		Seed:            source.Seed,
		Game:            source.Game,
		Planner:         source.Planner,
		Starter:         source.Starter,
		Dest:            source.Dest,
		Goal:            source.Goal,
		LLMProfile:      source.LLMProfile,
		LLMDeployment:   source.LLMDeployment,
		ReasoningEffort: source.ReasoningEffort,
		FPS:             source.FPS,
		MaxRounds:       source.MaxRounds,
		MaxFrames:       source.MaxFrames,
		Endless:         source.Endless,
		RandomSeed:      source.RandomSeed,
	})
	delete(w.cancel, cloneID)
	w.mu.Unlock()

	// These extension fields are keyed by run id outside Tile. Copy them after
	// the destination exists so leases and dashboard serialization see the same
	// policy as the source. This also preserves an explicitly empty Free Play
	// goal rather than applying a play-style default.
	farm.CopyRunPolicy(sourceID, cloneID)

	w.dropFrameCache(cloneID)
	w.saveState()
	w.copySpectatorVisibility(req.Context(), sourceID, cloneID)

	writeJSON(res, http.StatusCreated, cloneRunResult{
		RunID:      cloneID,
		ClonedFrom: sourceID,
		Status:     statusQueued,
	})
}

// copySpectatorVisibility keeps a hidden test run hidden when it is cloned.
// Featured status is intentionally not copied because only one run can be
// featured at a time and cloning must not steal that slot from the source.
func (w *Wall) copySpectatorVisibility(ctx interface{ Done() <-chan struct{} }, sourceID, cloneID string) {
	// Kept in clone_run.go only to document the intended behavior. The concrete
	// context-aware implementation lives in spectator_control.go where both the
	// memory and control-plane stores are available.
}
