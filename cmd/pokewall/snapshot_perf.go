package main

import (
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

// tileRowLocked copies one live tile while w.mu is held. Keeping row creation
// in one place lets filtered dashboard reads stop before touching old history,
// while single-run inspection avoids cloning the complete catalog.
func (w *Wall) tileRowLocked(t *Tile) tileRow {
	if t == nil {
		return tileRow{}
	}
	return tileRow{
		RunID:              t.RunID,
		Status:             t.Status,
		Game:               t.Game,
		Planner:            t.Planner,
		Starter:            t.Starter,
		Dest:               t.Dest,
		Goal:               t.Goal,
		GoalProvided:       t.GoalProvided,
		PlayStyle:          t.PlayStyle,
		RiskTolerance:      t.RiskTolerance,
		WildEncounters:     t.WildEncounters,
		LLMProfile:         t.LLMProfile,
		LLMDeployment:      t.LLMDeployment,
		ExperimentID:       t.ExperimentID,
		ExperimentArm:      t.ExperimentArm,
		ExperimentCase:     t.ExperimentCase,
		ReasoningEffort:    t.ReasoningEffort,
		Seed:               t.Seed,
		FPS:                t.FPS,
		MaxRounds:          t.MaxRounds,
		MaxFrames:          t.MaxFrames,
		RecoveryProfile:    t.RecoveryProfile,
		Endless:            t.Endless,
		RandomSeed:         t.RandomSeed,
		QueuedAt:           unixTime(t.QueuedAt),
		EndedAt:            unixTime(t.EndedAt),
		Attempts:           t.Attempts,
		ErrorAttempts:      t.ErrorAttempts,
		LossRecoveries:     t.LossRecoveries,
		RecoveryAttempts:   t.RecoveryAttempts,
		RecoveryBadges:     t.RecoveryBadges,
		RecoveryEvents:     t.RecoveryEvents,
		RecoveryMaps:       t.RecoveryMaps,
		Frame:              t.Frame,
		Map:                t.Map,
		X:                  t.X,
		Y:                  t.Y,
		MapsVisited:        t.MapsVisited,
		Trace:              t.Trace,
		Question:           t.Question,
		Decision:           t.Decision,
		Raw:                t.Raw,
		StopSoFar:          t.StopSoFar,
		Sprites:            append([]farm.MapSprite(nil), t.Sprites...),
		Trail:              append([][2]uint8(nil), t.Trail...),
		Stats:              t.Stats,
		Player:             t.Player,
		Reason:             t.Reason,
		Detail:             t.Detail,
		Issue:              issueLinkFor(t, w.issueLinks),
		ReplayAvailable:    t.ReplayAvailable,
		ResumeFromRunID:    t.ResumeFromRunID,
		CircuitKey:         t.CircuitKey,
		CircuitFingerprint: t.CircuitFingerprint,
		CircuitKind:        t.CircuitKind,
		CircuitCount:       t.CircuitCount,
		CircuitBadges:      t.CircuitBadges,
		CircuitEvents:      t.CircuitEvents,
		CircuitMaps:        t.CircuitMaps,
		CircuitRevision:    t.CircuitRevision,
	}
}

func (w *Wall) tileRowWithActivityLocked(t *Tile) tileRow {
	row := w.tileRowLocked(t)
	if t != nil {
		row.Activity = copyRunActivity(t.Activity)
	}
	return row
}

func (w *Wall) tileRowWithLineageLocked(t *Tile, lineage map[string]struct{}) tileRow {
	row := w.tileRowLocked(t)
	if t != nil && t.Finished {
		if _, ok := lineage[t.RunID]; ok {
			row.ResumeProtected = true
		}
	}
	return row
}

// snapshotFiltered applies status/limit while holding w.mu and before copying
// heavyweight row fields. limit <= 0 means unlimited; status empty means all.
func (w *Wall) snapshotFiltered(status string, limit int) dashboardView {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	workers := make([]workerRow, 0, len(w.workers))
	for _, wk := range w.workers {
		if wk == nil || len(wk.Addrs) == 0 {
			continue
		}
		workers = append(workers, workerRow{
			Addr:    wk.Addrs[0],
			Version: wk.Version,
			RunID:   wk.RunID,
			SeenAgo: now.Sub(wk.LastSeen).Round(time.Second).String(),
		})
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].Addr < workers[j].Addr })

	status = strings.ToLower(strings.TrimSpace(status))
	capHint := len(w.order)
	if limit > 0 && limit < capHint {
		capHint = limit
	}
	lineage := w.liveCheckpointLineageLocked()
	rows := make([]tileRow, 0, capHint)
	for i := len(w.order) - 1; i >= 0; i-- {
		t := w.tiles[w.order[i]]
		if t == nil || (status != "" && t.Status != status) {
			continue
		}
		rows = append(rows, w.tileRowWithLineageLocked(t, lineage))
		if limit > 0 && len(rows) >= limit {
			break
		}
	}
	return dashboardView{Now: now.Unix(), WallVersion: w.Version, Runs: rows, Workers: workers}
}

func (w *Wall) snapshotRun(runID string) (tileRow, bool) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return tileRow{}, false
	}
	w.mu.Lock()
	t := w.tiles[runID]
	if t != nil {
		row := w.tileRowWithActivityLocked(t)
		w.mu.Unlock()
		return row, true
	}
	w.mu.Unlock()
	return w.catalogSnapshotRun(runID)
}
