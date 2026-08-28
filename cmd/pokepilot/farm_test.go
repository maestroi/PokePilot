package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

// TestApplySpec pins the lease semantics: the spec is authoritative, so a
// zero Seed and a zero FPS must survive applySpec untouched (no CLI or
// default inheritance), while zero MaxRounds/MaxFrames fall back to the llm
// guardrails.
func TestApplySpec(t *testing.T) {
	s := farm.Spec{RunID: "r1", Seed: 0, Planner: "llm", FPS: 0}
	planner, starter, dest, fps, maxRounds, maxFrames := applySpec(s)
	if planner != "llm" || starter != "" || dest != "" {
		t.Fatalf("applySpec(%+v) = (%q,%q,%q), want (llm,\"\",\"\")", s, planner, starter, dest)
	}
	if fps != 0 {
		t.Fatalf("FPS 0 must stay 0 (flat-out), got %d", fps)
	}
	if maxRounds != llmMaxRounds {
		t.Fatalf("zero MaxRounds must default to llmMaxRounds (%d), got %d", llmMaxRounds, maxRounds)
	}
	if maxFrames != llmMaxFrames {
		t.Fatalf("zero MaxFrames must default to llmMaxFrames (%d), got %d", llmMaxFrames, maxFrames)
	}
	if burn := seedBurn(0); burn != 0 {
		t.Fatalf("seed 0 must burn nothing (replays bit-identically), burned %d", burn)
	}
	if burn := seedBurn(7); burn < 0 || burn >= 600 {
		t.Fatalf("seed 7 burn out of range [0,600): %d", burn)
	}

	s2 := farm.Spec{RunID: "r2", Seed: 42, Planner: "scripted", Starter: "squirtle", Dest: "pewter city", FPS: 30, MaxRounds: 5, MaxFrames: 1234}
	planner, starter, dest, fps, maxRounds, maxFrames = applySpec(s2)
	if planner != "scripted" || starter != "squirtle" || dest != "pewter city" || fps != 30 || maxRounds != 5 || maxFrames != 1234 {
		t.Fatalf("nonzero spec fields must pass through exactly: got (%q,%q,%q,%d,%d,%d)", planner, starter, dest, fps, maxRounds, maxFrames)
	}
}

// TestHeartbeatSnapshot hammers the plain snapshot from many goroutines so
// -race catches a missing lock on either store or load.
func TestHeartbeatSnapshot(t *testing.T) {
	s := &heartbeatSnap{}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				s.store(farm.Heartbeat{RunID: fmt.Sprintf("run-%d", n), Frame: uint64(i)})
				hb := s.load()
				if hb.Frame > 2000 || hb.RunID == "" {
					t.Errorf("load returned %+v", hb)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestHeartbeatLoopJoinsDespiteHungHandler proves the request deadline: the
// wall blocks until the client's context fires, and closing stop still joins
// the loop within the deadline instead of wedging behind the handler.
func TestHeartbeatLoopJoinsDespiteHungHandler(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()

	client := &farm.Client{BaseURL: srv.URL, HTTP: srv.Client()}
	snap := &heartbeatSnap{}
	cancel := make(chan struct{})
	stop := make(chan struct{})
	done := heartbeatLoop(client, "r1", snap.load, cancel, stop, time.Millisecond)

	time.Sleep(50 * time.Millisecond) // let the first heartbeat go in flight
	close(stop)
	select {
	case <-done:
	case <-time.After(heartbeatDeadline + 3*time.Second):
		t.Fatal("heartbeatLoop did not join after stop; a hung handler wedged it")
	}
	close(release) // wake any still-blocked handler so Close can drain
}

// TestHeartbeatCancelClosesOnce proves a cancel reply closes the cooperative
// channel exactly once (a second close would panic) and the loop still joins.
func TestHeartbeatCancelClosesOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"cancel":true}`)
	}))
	defer srv.Close()

	client := &farm.Client{BaseURL: srv.URL, HTTP: srv.Client()}
	snap := &heartbeatSnap{}
	cancel := make(chan struct{})
	stop := make(chan struct{})
	done := heartbeatLoop(client, "r1", snap.load, cancel, stop, 20*time.Millisecond)

	time.Sleep(300 * time.Millisecond) // several cancel=true replies land
	select {
	case <-cancel:
	default:
		t.Fatal("cancel was never closed despite cancel=true replies")
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("heartbeatLoop did not join after stop")
	}
}
