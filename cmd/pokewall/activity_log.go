package main

import (
	"log"
	"time"
)

const wallActivityLogInterval = 30 * time.Second

type wallActivitySnapshot struct {
	total        int
	queueDepth   int
	queued       int
	leased       int
	running      int
	done         int
	other        int
	workers      int
	busyWorkers  int
	issueOutbox  int
	activeRun    string
	activeFrame  uint64
	activeMap    uint8
	activeX      uint8
	activeY      uint8
	activeUpdate time.Time
}

func (w *Wall) activitySnapshot() wallActivitySnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()

	s := wallActivitySnapshot{
		total:       len(w.tiles),
		queueDepth:  len(w.queue),
		workers:     len(w.workers),
		issueOutbox: len(w.outbox),
	}
	for _, worker := range w.workers {
		if worker != nil && worker.RunID != "" {
			s.busyWorkers++
		}
	}
	for _, tile := range w.tiles {
		if tile == nil {
			continue
		}
		switch tile.Status {
		case statusQueued:
			s.queued++
		case statusLeased:
			s.leased++
		case statusRunning:
			s.running++
		case statusDone:
			s.done++
		default:
			s.other++
		}
		if tile.Finished || (tile.Status != statusRunning && tile.Status != statusLeased) {
			continue
		}
		if s.activeRun == "" || tile.lastUpdate.After(s.activeUpdate) {
			s.activeRun = tile.RunID
			s.activeFrame = tile.Frame
			s.activeMap = tile.Map
			s.activeX = tile.X
			s.activeY = tile.Y
			s.activeUpdate = tile.lastUpdate
		}
	}
	return s
}

// RunActivityLog emits a low-frequency operational heartbeat. It is meant for
// docker service logs: enough information to distinguish an idle farm from a
// healthy active one, without logging the one-second runner heartbeats.
func (w *Wall) RunActivityLog(interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for now := range ticker.C {
		s := w.activitySnapshot()
		active := "none"
		age := time.Duration(0)
		if s.activeRun != "" {
			active = s.activeRun
			if !s.activeUpdate.IsZero() {
				age = now.Sub(s.activeUpdate).Round(time.Second)
				if age < 0 {
					age = 0
				}
			}
		}
		log.Printf(
			"pokewall: activity total=%d queue_depth=%d queued=%d leased=%d running=%d done=%d other=%d workers=%d busy=%d issue_outbox=%d active_run=%s frame=%d map=0x%02x pos=%d,%d update_age=%s",
			s.total, s.queueDepth, s.queued, s.leased, s.running, s.done, s.other,
			s.workers, s.busyWorkers, s.issueOutbox,
			active, s.activeFrame, s.activeMap, s.activeX, s.activeY, age,
		)
	}
}
