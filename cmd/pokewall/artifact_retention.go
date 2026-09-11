package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultArtifactRetention  = 24 * time.Hour
	defaultArtifactSweepEvery = 15 * time.Minute
)

// liveCheckpointLineageLocked returns every run whose checkpoint tree
// is still part of recovery for an active run: the active run itself and
// every ResumeFromRunID ancestor it can still fall back to. Caller holds
// w.mu.
func (w *Wall) liveCheckpointLineageLocked() map[string]struct{} {
	keep := make(map[string]struct{})
	for _, t := range w.tiles {
		if t == nil || t.Finished {
			continue
		}
		if t.RunID != "" {
			keep[t.RunID] = struct{}{}
		}
		seen := make(map[string]struct{})
		for id := t.ResumeFromRunID; id != ""; {
			if _, dup := seen[id]; dup {
				break
			}
			seen[id] = struct{}{}
			keep[id] = struct{}{}
			parent := w.tiles[id]
			if parent == nil {
				break
			}
			id = parent.ResumeFromRunID
		}
	}
	return keep
}

func (w *Wall) runArtifactsProtected(runID string) bool {
	w.mu.Lock()
	_, protected := w.liveCheckpointLineageLocked()[runID]
	w.mu.Unlock()
	return protected
}

func (w *Wall) deleteLocalCheckpointTree(runID string) error {
	if w.dumpsDir == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(w.dumpsDir, "checkpoints", safeBase(runID)))
}

func (w *Wall) deleteLocalRunArtifacts(runID string, attempts int) error {
	if err := w.deleteLocalFinishDumps(runID, attempts); err != nil {
		return err
	}
	return w.deleteLocalCheckpointTree(runID)
}

func localFinishDumpPaths(dumpsDir, runID string, attempts int) []string {
	if attempts < 1 {
		attempts = 1
	}
	paths := []string{filepath.Join(dumpsDir, safeDumpName(runID))}
	for attempt := 2; attempt <= attempts; attempt++ {
		paths = append(paths, filepath.Join(dumpsDir, fmt.Sprintf("%s-attempt-%d.json", safeBase(runID), attempt)))
	}
	return paths
}

// RunArtifactRetention bounds local wall storage without touching a
// checkpoint lineage that a live run may still resume from. It waits one
// interval before the first sweep so startup recovery/reporting gets the
// first chance to consume old finish dumps.
func (w *Wall) RunArtifactRetention(interval, maxAge time.Duration) {
	if w.dumpsDir == "" || maxAge <= 0 {
		return
	}
	if interval <= 0 {
		interval = defaultArtifactSweepEvery
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for now := range tick.C {
		if err := w.expireLocalArtifacts(now, maxAge); err != nil {
			log.Printf("pokewall: local artifact retention: %v", err)
		}
	}
}

type retentionRun struct {
	id              string
	attempts        int
	endedAt         time.Time
	replayAvailable bool
	pendingIssue    bool
	protected       bool
}

func (w *Wall) expireLocalArtifacts(now time.Time, maxAge time.Duration) error {
	if w.dumpsDir == "" || maxAge <= 0 {
		return nil
	}
	cutoff := now.Add(-maxAge)

	w.mu.Lock()
	lineage := w.liveCheckpointLineageLocked()
	pendingRuns := make(map[string]struct{})
	for _, entry := range w.outbox {
		if entry.Status == outboxPending {
			pendingRuns[entry.RunID] = struct{}{}
		}
	}
	runs := make([]retentionRun, 0, len(w.tiles))
	knownCheckpointBases := make(map[string]struct{}, len(w.tiles))
	for id, t := range w.tiles {
		if t == nil {
			continue
		}
		knownCheckpointBases[safeBase(id)] = struct{}{}
		if !t.Finished {
			continue
		}
		attempts := t.Attempts
		if attempts < 1 {
			attempts = 1
		}
		_, pending := pendingRuns[id]
		_, protected := lineage[id]
		runs = append(runs, retentionRun{
			id: id, attempts: attempts, endedAt: t.EndedAt,
			replayAvailable: t.ReplayAvailable,
			pendingIssue:    pending, protected: protected,
		})
	}
	w.mu.Unlock()

	coordinator := objectiveReporterFor(w)
	coordinator.mu.Lock()
	pendingPaths := make(map[string]struct{}, len(coordinator.pending))
	for path := range coordinator.pending {
		pendingPaths[path] = struct{}{}
	}
	coordinator.mu.Unlock()

	var errs []error
	stateChanged := false
	for _, run := range runs {
		if run.protected || run.pendingIssue || run.endedAt.IsZero() || run.endedAt.After(cutoff) {
			continue
		}
		pending := false
		for _, path := range localFinishDumpPaths(w.dumpsDir, run.id, run.attempts) {
			if _, ok := pendingPaths[path]; ok {
				pending = true
				break
			}
		}
		if pending {
			continue
		}

		// Recheck identity immediately before deletion. A reused run ID gets a
		// fresh active Tile; stale retention work must not delete its files.
		w.mu.Lock()
		cur := w.tiles[run.id]
		stillExpired := cur != nil && cur.Finished && cur.EndedAt.Equal(run.endedAt)
		w.mu.Unlock()
		if !stillExpired {
			continue
		}

		if err := w.deleteLocalFinishDumps(run.id, run.attempts); err != nil {
			errs = append(errs, fmt.Errorf("finish dumps %s: %w", run.id, err))
		} else if run.replayAvailable {
			w.mu.Lock()
			if cur := w.tiles[run.id]; cur != nil && cur.Finished && cur.EndedAt.Equal(run.endedAt) && cur.ReplayAvailable {
				cur.ReplayAvailable = false
				stateChanged = true
			}
			w.mu.Unlock()
		}
		if err := w.deleteLocalCheckpointTree(run.id); err != nil {
			errs = append(errs, fmt.Errorf("checkpoint tree %s: %w", run.id, err))
		}
	}

	// Old wall versions could leave checkpoint trees after a history row was
	// manually deleted. The checkpoints directory is dedicated storage, so
	// orphan run directories can safely age out by mtime. Top-level JSON is
	// intentionally not swept generically because deployments may place the
	// wall state file beside finish dumps.
	checkpointRoot := filepath.Join(w.dumpsDir, "checkpoints")
	entries, err := os.ReadDir(checkpointRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	} else if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if _, known := knownCheckpointBases[entry.Name()]; known {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if info.ModTime().After(cutoff) {
				continue
			}
			if err := os.RemoveAll(filepath.Join(checkpointRoot, entry.Name())); err != nil {
				errs = append(errs, fmt.Errorf("orphan checkpoint tree %s: %w", entry.Name(), err))
			}
		}
	}

	if stateChanged {
		w.saveState()
	}
	return errors.Join(errs...)
}
