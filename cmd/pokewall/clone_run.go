package main

import (
	"context"
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

	// These extension fields are keyed by run id outside Tile. Install them
	// before the clone becomes leasable so a fast worker cannot observe a
	// partially cloned policy. This also preserves an explicitly empty Free Play
	// goal rather than applying a play-style default.
	farm.CopyRunPolicy(sourceID, cloneID)

	// Do the same for spectator visibility. The clone exists in RAM but is not
	// in the lease queue yet, so a hidden source cannot briefly leak publicly.
	w.copySpectatorVisibility(req.Context(), sourceID, cloneID)

	w.mu.Lock()
	w.queue = append(w.queue, cloneID)
	w.mu.Unlock()
	w.dropFrameCache(cloneID)
	w.saveState()

	writeJSON(res, http.StatusCreated, cloneRunResult{
		RunID:      cloneID,
		ClonedFrom: sourceID,
		Status:     statusQueued,
	})
}

// copySpectatorVisibility keeps a hidden test run hidden when it is cloned.
// Featured status is intentionally not copied because only one run can be
// featured at a time and cloning must not steal that slot from the source.
func (w *Wall) copySpectatorVisibility(ctx context.Context, sourceID, cloneID string) {
	snapshot, err := w.spectatorControlSnapshot(ctx)
	if err != nil {
		return
	}
	setting, explicitlySet := snapshot.Runs[sourceID]
	if !explicitlySet {
		return // both source and clone use the default visible=true
	}
	visible := setting.Visible
	_, _ = w.patchSpectatorRunControl(ctx, cloneID, spectatorRunControlPatch{Visible: &visible})
}
