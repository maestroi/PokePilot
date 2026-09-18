package main

import (
	"testing"
	"time"
)

func TestActivitySnapshotSummarizesWallState(t *testing.T) {
	w := NewWall("")
	now := time.Now()
	w.mu.Lock()
	w.queue = []string{"queued"}
	w.tiles["queued"] = &Tile{RunID: "queued", Status: statusQueued, lastUpdate: now.Add(-2 * time.Second)}
	w.tiles["leased"] = &Tile{RunID: "leased", Status: statusLeased, lastUpdate: now.Add(-time.Second)}
	w.tiles["running"] = &Tile{RunID: "running", Status: statusRunning, Frame: 123, Map: 0x0c, X: 4, Y: 7, lastUpdate: now}
	w.tiles["done"] = &Tile{RunID: "done", Status: statusDone, Finished: true}
	w.workers["worker-a"] = &workerInfo{RunID: "running"}
	w.workers["worker-b"] = &workerInfo{}
	w.outbox["occurrence"] = outboxEntry{}
	w.mu.Unlock()

	got := w.activitySnapshot()
	if got.total != 4 || got.queueDepth != 1 || got.queued != 1 || got.leased != 1 || got.running != 1 || got.done != 1 {
		t.Fatalf("unexpected run counts: %+v", got)
	}
	if got.workers != 2 || got.busyWorkers != 1 || got.issueOutbox != 1 {
		t.Fatalf("unexpected worker/outbox counts: %+v", got)
	}
	if got.activeRun != "running" || got.activeFrame != 123 || got.activeMap != 0x0c || got.activeX != 4 || got.activeY != 7 {
		t.Fatalf("unexpected active run snapshot: %+v", got)
	}
}
