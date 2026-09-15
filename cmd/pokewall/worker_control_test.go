package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestForceEndWorkerTerminatesAndCancelsActiveRun(t *testing.T) {
	w := NewWall("")
	addr := "10.0.1.23:8099"
	altAddr := "10.0.2.23:8099"
	runID := "run-force-end"

	w.mu.Lock()
	w.order = append(w.order, runID)
	w.tiles[runID] = &Tile{
		RunID:      runID,
		Status:     statusRunning,
		Endless:    true,
		lastUpdate: time.Now(),
	}
	w.workers[addr] = &workerInfo{
		Addrs:    []string{addr, altAddr},
		RunID:    runID,
		LastSeen: time.Now(),
	}
	w.mu.Unlock()

	var terminated []string
	handler := workerControlHTTPHandlerWithTerminator(w, http.NotFoundHandler(), func(_ context.Context, got []string) error {
		terminated = append([]string(nil), got...)
		return nil
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/workers/"+url.PathEscape(addr)+"/force-end", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("force-end = %d: %s", res.Code, res.Body.String())
	}
	if len(terminated) != 2 || terminated[0] != addr || terminated[1] != altAddr {
		t.Fatalf("terminated addresses = %v, want [%s %s]", terminated, addr, altAddr)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.workers[addr]; ok {
		t.Fatalf("worker %q still present after force-end", addr)
	}
	tile := w.tiles[runID]
	if tile == nil || !tile.Finished || tile.Status != statusDone {
		t.Fatalf("run after force-end = %#v, want terminal done tile", tile)
	}
	if tile.Reason != "cancelled" {
		t.Fatalf("reason = %q, want cancelled", tile.Reason)
	}
	if tile.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", tile.Attempts)
	}
	if len(w.queue) != 0 {
		t.Fatalf("queue = %v, force-ended endless run must not enqueue a successor", w.queue)
	}
	if len(w.order) != 1 {
		t.Fatalf("order = %v, force-ended endless run unexpectedly created a successor", w.order)
	}
}

func TestForceEndWorkerFailureLeavesWorkerAndRunUntouched(t *testing.T) {
	w := NewWall("")
	addr := "10.0.1.24:8099"
	runID := "run-still-live"

	w.mu.Lock()
	w.order = append(w.order, runID)
	w.tiles[runID] = &Tile{RunID: runID, Status: statusRunning, lastUpdate: time.Now()}
	w.workers[addr] = &workerInfo{Addrs: []string{addr}, RunID: runID, LastSeen: time.Now()}
	w.mu.Unlock()

	handler := workerControlHTTPHandlerWithTerminator(w, http.NotFoundHandler(), func(context.Context, []string) error {
		return context.DeadlineExceeded
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/workers/"+url.PathEscape(addr)+"/force-end", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadGateway {
		t.Fatalf("force-end = %d, want %d", res.Code, http.StatusBadGateway)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.workers[addr] == nil {
		t.Fatal("worker disappeared after failed force-end")
	}
	if tile := w.tiles[runID]; tile == nil || tile.Finished || tile.Status != statusRunning {
		t.Fatalf("run changed after failed force-end: %#v", tile)
	}
}
