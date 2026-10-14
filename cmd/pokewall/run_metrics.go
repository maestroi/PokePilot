package main

import (
	"log"
	"time"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/metrics"
)

// SetRunMetrics attaches the run-lifecycle series. Restored tiles rebuild
// the active gauge and the last-success timestamp; they do not increment
// started/completed/failed/aborted, so a restart cannot double-count a
// campaign that already ended.
func (w *Wall) SetRunMetrics(runs *metrics.Runs) {
	w.mu.Lock()
	w.runs = runs
	w.observeRestoredLocked()
	w.mu.Unlock()
	w.seedSuccessFromCatalog()
}

func (w *Wall) observeRestoredLocked() {
	if w.runs == nil {
		return
	}
	counts := map[string]int{}
	for _, t := range w.tiles {
		if t == nil {
			continue
		}
		active := !t.Finished && (t.Status == statusLeased || t.Status == statusRunning)
		t.runActive = active
		if active {
			counts[metrics.NormalizeGame(t.Game)]++
		}
		if t.Finished && t.Reason == "done" && !t.EndedAt.IsZero() {
			w.runs.NoteSuccess(t.Game, float64(t.EndedAt.Unix()))
		}
	}
	w.runs.ReplaceActive(counts)
}

func (w *Wall) seedSuccessFromCatalog() {
	if w == nil || w.runs == nil {
		return
	}
	catalog := catalogFor(w)
	if catalog == nil {
		return
	}
	latest, err := catalog.latestSuccessUnixByGame()
	if err != nil {
		log.Printf("pokewall: run metrics last success: %v", err)
		return
	}
	for game, unix := range latest {
		w.runs.NoteSuccess(game, unix)
	}
}

func (w *Wall) markRunActiveLocked(t *Tile) {
	if w.runs == nil || t == nil || t.runActive {
		return
	}
	t.runActive = true
	w.runs.MarkActive(t.Game)
}

func (w *Wall) clearRunActiveLocked(t *Tile) {
	if w.runs == nil || t == nil || !t.runActive {
		return
	}
	t.runActive = false
	w.runs.ClearActive(t.Game)
}

func (w *Wall) noteRunMilestonesLocked(t *Tile, player *farm.Player) {
	if t == nil || player == nil {
		return
	}
	if len(player.Party) > 0 {
		w.noteMilestoneIDLocked(t, "starter")
	}
	for n := 1; n <= len(player.Badges); n++ {
		id, ok := metrics.BadgeMilestone(n)
		if !ok {
			break
		}
		w.noteMilestoneIDLocked(t, id)
	}
	for _, label := range player.Milestones {
		if id, ok := metrics.StoryMilestone(label); ok {
			w.noteMilestoneIDLocked(t, id)
		}
	}
}

func (w *Wall) noteMilestoneIDLocked(t *Tile, id string) {
	if w.runs == nil || t == nil || !metrics.KnownMilestone(id) {
		return
	}
	if t.metricMilestones == nil {
		t.metricMilestones = map[string]struct{}{}
	}
	if _, seen := t.metricMilestones[id]; seen {
		return
	}
	t.metricMilestones[id] = struct{}{}
	w.runs.Milestone(t.Game, id)
}

func (t *Tile) milestoneIDs() []string {
	if t == nil || len(t.metricMilestones) == 0 {
		return nil
	}
	out := make([]string, 0, len(t.metricMilestones))
	for id := range t.metricMilestones {
		out = append(out, id)
	}
	return out
}

func (w *Wall) recordTerminalRunLocked(t *Tile, reason string, cancelled bool, now time.Time) {
	if w.runs == nil || t == nil {
		return
	}
	w.noteRunMilestonesLocked(t, t.Player)
	result := metrics.RunResult(reason, cancelled)
	if result == metrics.ResultCompleted {
		w.noteMilestoneIDLocked(t, "completed")
	}
	rounds := 0
	if t.Stats != nil {
		rounds = t.Stats.Rounds
		if t.Stats.Round > rounds {
			rounds = t.Stats.Round
		}
	}
	obs := metrics.RunFinish{
		Game:      t.Game,
		Mode:      t.Planner,
		Result:    result,
		Milestone: metrics.FurthestMilestone(t.milestoneIDs()),
		Rounds:    rounds,
		Frames:    t.Frame,
		At:        now,
	}
	if !t.QueuedAt.IsZero() {
		obs.Duration = now.Sub(t.QueuedAt)
		obs.HasDuration = obs.Duration >= 0
	}
	w.runs.Finished(obs)
}
