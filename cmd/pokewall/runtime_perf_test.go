package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func postJSONStatus(rawURL string, v any) (int, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return 0, err
	}
	resp, err := http.Post(rawURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func TestConcurrentFinishSettlesGenerationOnce(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	frameServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer frameServer.Close()

	wall := NewWall("")
	srv := httptest.NewServer(wall.Handler())
	defer srv.Close()

	s := spec("finish-race")
	s.Endless = true
	if status, err := postJSONStatus(srv.URL+"/v1/specs", s); err != nil || status != http.StatusOK {
		t.Fatalf("enqueue: status=%d err=%v", status, err)
	}
	if status, err := postJSONStatus(srv.URL+"/v1/lease", struct{}{}); err != nil || status != http.StatusOK {
		t.Fatalf("lease: status=%d err=%v", status, err)
	}
	addr := strings.TrimPrefix(frameServer.URL, "http://")
	hb := farm.Heartbeat{RunID: s.RunID, WorkerAddrs: []string{addr}}
	if status, err := postJSONStatus(srv.URL+"/v1/runs/"+s.RunID+"/heartbeat", hb); err != nil || status != http.StatusOK {
		t.Fatalf("heartbeat: status=%d err=%v", status, err)
	}

	report := farm.FinishReport{RunID: s.RunID, Attempt: 1, Reason: "done", Detail: "complete"}
	statuses := make(chan int, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, err := postJSONStatus(srv.URL+"/v1/runs/"+s.RunID+"/finish", report)
			if err != nil {
				errs <- err
				return
			}
			statuses <- status
		}()
	}

	// Both requests make it as far as ancillary frame capture. In the old
	// implementation that meant both had already passed the unsettled check
	// and would each call settleRun after this gate opened.
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("finish requests did not reach frame capture")
		}
	}
	close(release)
	wg.Wait()
	close(statuses)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("finish request: %v", err)
		}
	}
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("finish status=%d, want 200", status)
		}
	}

	wall.mu.Lock()
	attempts := wall.tiles[s.RunID].Attempts
	orderLen := len(wall.order)
	queueLen := len(wall.queue)
	wall.mu.Unlock()
	if attempts != 1 {
		t.Fatalf("attempts=%d, want exactly one settled generation", attempts)
	}
	if orderLen != 2 || queueLen != 1 {
		t.Fatalf("endless successors: order=%d queue=%d, want original + exactly one successor", orderLen, queueLen)
	}
}

func TestConcurrentFrameFetchesAreCoalesced(t *testing.T) {
	var requests atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	frameServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		_, _ = w.Write([]byte("frame"))
	}))
	defer frameServer.Close()

	wall := NewWall("")
	addr := strings.TrimPrefix(frameServer.URL, "http://")
	const callers = 12
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := wall.fetchRunnerFrameCached(context.Background(), "run-1", []string{addr})
			if err != nil {
				errs <- err
				return
			}
			if string(data) != "frame" {
				errs <- &unexpectedFrameError{got: string(data)}
			}
		}()
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream frame fetch did not start")
	}
	// Give the remaining callers a chance to join the in-flight request.
	time.Sleep(25 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("upstream frame requests=%d, want 1", got)
	}

	if _, err := wall.fetchRunnerFrameCached(context.Background(), "run-1", []string{addr}); err != nil {
		t.Fatalf("fresh cached frame: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("fresh cache triggered upstream request count=%d, want 1", got)
	}
}

type unexpectedFrameError struct{ got string }

func (e *unexpectedFrameError) Error() string { return "unexpected frame payload: " + e.got }

func TestSnapshotFilteredReturnsNewestMatchingRows(t *testing.T) {
	wall := NewWall("")
	wall.mu.Lock()
	for i := 0; i < 40; i++ {
		id := "run-" + string(rune('A'+i))
		status := statusDone
		if i%7 == 0 {
			status = statusRunning
		}
		wall.order = append(wall.order, id)
		wall.tiles[id] = &Tile{RunID: id, Status: status, Raw: strings.Repeat("x", 32<<10)}
	}
	wall.mu.Unlock()

	view := wall.snapshotFiltered(statusDone, 3)
	if len(view.Runs) != 3 {
		t.Fatalf("rows=%d, want 3", len(view.Runs))
	}
	for _, run := range view.Runs {
		if run.Status != statusDone {
			t.Fatalf("status=%q, want done", run.Status)
		}
	}
	// The result must be newest-first, preserving the dashboard contract.
	if view.Runs[0].RunID != "run-h" || view.Runs[1].RunID != "run-g" || view.Runs[2].RunID != "run-f" {
		t.Fatalf("newest rows=%q,%q,%q", view.Runs[0].RunID, view.Runs[1].RunID, view.Runs[2].RunID)
	}
}
