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
		Goal:            farm.GoalFrom(source.Goal),
		Dest:            source.Dest,
		PlayStyle:       source.PlayStyle,
		Purpose:         source.Purpose,
		RiskTolerance:   source.RiskTolerance,
		WildEncounters:  source.WildEncounters,
		LLMProfile:      source.LLMProfile,
		LLMDeployment:   source.LLMDeployment,
		ReasoningEffort: source.ReasoningEffort,
		FPS:             source.FPS,
		MaxRounds:       source.MaxRounds,
		MaxFrames:       source.MaxFrames,
		RecoveryProfile: source.RecoveryProfile,
		Endless:         source.Endless,
		RandomSeed:      source.RandomSeed,
	})
	// applySpec settles the destination goal state from the source. Passing the
	// source goal as provided keeps an explicit Free play goal (and any
	// already-resolved play-style default) from being re-derived in the clone.
	delete(w.cancel, cloneID)
	w.mu.Unlock()

	// The clone now owns every policy field on its own Tile, installed before
	// it becomes leasable so a fast worker cannot observe a partial policy.

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
