package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestPauseRunningRunResumesSameIDFromLatestCheckpoint(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(pauseHTTPHandler(w, w.Handler()))
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()

	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "pause-me", Planner: "llm", Goal: farm.GoalFrom("beat the game")})
	first, err := client.Lease(ctx)
	if err != nil || first == nil || first.Attempt != 1 {
		t.Fatalf("lease 1 = %+v, %v", first, err)
	}

	state := wallResumeArtifact("round-007-frame-0000000700-goto.state", []byte("near-bug-state"), "application/octet-stream")
	knowledge := wallResumeArtifact("round-007-frame-0000000700-goto.knowledge-v4.json", []byte(`{"intent":"cross the blocker"}`), "application/json")
	if err := client.Checkpoint(ctx, farm.CheckpointReport{
		RunID: "pause-me", Attempt: 1, Artifacts: []farm.Artifact{state, knowledge},
	}); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	postPauseControl(t, srv.URL+"/v1/runs/pause-me/pause")
	reply, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: "pause-me", Frame: 700, Map: 5, X: 11, Y: 4})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !reply.Cancel {
		t.Fatal("pause request did not reuse the cooperative cancel signal")
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: "pause-me", Attempt: 1, Reason: "cancelled", Detail: "operator stop"}); err != nil {
		t.Fatalf("finish paused attempt: %v", err)
	}

	w.mu.Lock()
	paused := *w.tiles["pause-me"]
	w.mu.Unlock()
	if paused.Status != statusPaused || !paused.Finished || paused.Attempts != 1 {
		t.Fatalf("paused tile = status %q finished=%v attempts=%d", paused.Status, paused.Finished, paused.Attempts)
	}
	if paused.StopSoFar != "paused by operator" {
		t.Fatalf("pause note = %q", paused.StopSoFar)
	}

	postPauseControl(t, srv.URL+"/v1/runs/pause-me/resume")
	second, err := client.Lease(ctx)
	if err != nil || second == nil || second.RunID != "pause-me" || second.Attempt != 2 {
		t.Fatalf("lease 2 = %+v, %v", second, err)
	}
	cp, err := client.ResumeCheckpoint(ctx, "pause-me", second.Attempt)
	if err != nil {
		t.Fatalf("resume checkpoint: %v", err)
	}
	if cp == nil || cp.Attempt != 1 {
		t.Fatalf("resume checkpoint = %+v", cp)
	}
	if cp.State.Name != state.Name || string(cp.State.Data) != "near-bug-state" {
		t.Fatalf("resumed state = %+v, want newest pre-pause checkpoint", cp.State)
	}
	if cp.Knowledge == nil || cp.Knowledge.Name != knowledge.Name {
		t.Fatalf("resumed knowledge = %+v", cp.Knowledge)
	}
}

func TestRepeatedIdenticalErrorsAutoPauseBeforeThirdAttempt(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(pauseHTTPHandler(w, w.Handler()))
	defer srv.Close()
	client := farm.NewClient(srv.URL)
	ctx := context.Background()

	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "looping", Planner: "llm", Goal: farm.GoalFrom("beat the game")})
	const detail = "unknown fishing rod old rod at map 05"
	expectedKey, expectedFingerprint := failureIdentity(normalizeDetail(detail))
	for attempt := 1; attempt <= autoPauseRepeatThreshold; attempt++ {
		spec, err := client.Lease(ctx)
		if err != nil || spec == nil || spec.Attempt != attempt {
			t.Fatalf("lease %d = %+v, %v", attempt, spec, err)
		}
		if _, err := client.Heartbeat(ctx, farm.Heartbeat{
			RunID: "looping", Frame: uint64(100 * attempt), Map: 5, X: 11, Y: 4,
		}); err != nil {
			t.Fatalf("heartbeat %d: %v", attempt, err)
		}
		if err := client.Finish(ctx, farm.FinishReport{
			RunID: "looping", Attempt: attempt, Reason: "error", Detail: detail,
			RunnerVersion: "build-broken",
			ProgressFinal: &farm.Progress{Badges: 2, Events: 20, Maps: 40},
		}); err != nil {
			t.Fatalf("finish %d: %v", attempt, err)
		}
	}

	w.mu.Lock()
	tile := *w.tiles["looping"]
	queued := append([]string(nil), w.queue...)
	link := w.issueLinks[expectedKey]
	w.mu.Unlock()
	if tile.Status != statusPaused || !tile.Finished {
		t.Fatalf("tile after repeated errors = status %q finished=%v", tile.Status, tile.Finished)
	}
	if tile.Attempts != autoPauseRepeatThreshold || tile.ErrorAttempts != autoPauseRepeatThreshold {
		t.Fatalf("attempt counters = attempts %d error %d", tile.Attempts, tile.ErrorAttempts)
	}
	if tile.CircuitKey != expectedKey || tile.CircuitFingerprint != expectedFingerprint || tile.CircuitKind != "repeat" {
		t.Fatalf("circuit identity = key %q fingerprint %q kind %q", tile.CircuitKey, tile.CircuitFingerprint, tile.CircuitKind)
	}
	if want := circuitPauseNote(failureCircuitDecision{Kind: "repeat", Count: autoPauseRepeatThreshold, Key: expectedKey}); tile.StopSoFar != want {
		t.Fatalf("auto-pause note = %q, want %q", tile.StopSoFar, want)
	}
	if !link.CircuitOpen || link.CircuitRunID != "looping" || link.CircuitKind != "repeat" {
		t.Fatalf("placeholder issue circuit = %+v", link)
	}
	for _, id := range queued {
		if id == "looping" {
			t.Fatalf("auto-paused run remained queued: %v", queued)
		}
	}
	third, err := client.Lease(ctx)
	if err != nil {
		t.Fatalf("lease after auto-pause: %v", err)
	}
	if third != nil {
		t.Fatalf("auto-pause still offered another attempt: %+v", third)
	}

	postPauseControl(t, srv.URL+"/v1/runs/looping/resume")
	third, err = client.Lease(ctx)
	if err != nil || third == nil || third.Attempt != autoPauseRepeatThreshold+1 {
		t.Fatalf("lease after resume = %+v, %v", third, err)
	}
	w.mu.Lock()
	if got := w.tiles["looping"].ErrorAttempts; got != 0 {
		w.mu.Unlock()
		t.Fatalf("error retry budget after human resume = %d, want 0", got)
	}
	w.mu.Unlock()
}

func TestRepeatedFailureCircuitCarriesFinishRevisionAndProgress(t *testing.T) {
	w := NewWall("")
	const detail = "unknown fishing rod old rod at map 05"
	// Prime the first identical occurrence; maybeAutoPauseRepeatedFailure records
	// the second one while observing the already-settled retry tile below.
	recordFailureStreak(w, "looping", detail)

	w.tiles["looping"] = &Tile{
		RunID: "looping", Status: statusQueued, Attempts: 2, ErrorAttempts: 2,
		Detail: "attempt 2 failed: " + detail,
	}
	w.queue = []string{"looping"}
	before := pauseFinishSnapshot{
		ok: true,
		row: tileRow{
			RunID: "looping", Attempts: 1, ErrorAttempts: 1,
			Seed: 42, Frame: 1234, Map: 5, X: 11, Y: 4,
		},
	}
	report := farm.FinishReport{
		RunID: "looping", Attempt: 2, Reason: "error", Detail: detail,
		RunnerVersion: "build-broken",
		ProgressFinal: &farm.Progress{Badges: 2, Events: 20, Maps: 40},
	}
	if !w.maybeAutoPauseRepeatedFailure("looping", before, report) {
		t.Fatal("second repeated failure did not open fallback circuit")
	}

	tile := w.tiles["looping"]
	if tile.CircuitRevision != "build-broken" || tile.CircuitBadges != 2 || tile.CircuitEvents != 20 || tile.CircuitMaps != 40 {
		t.Fatalf("circuit baseline = revision %q badges %d events %d maps %d", tile.CircuitRevision, tile.CircuitBadges, tile.CircuitEvents, tile.CircuitMaps)
	}
}

func TestCancelPausedRunMakesItTerminal(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(pauseHTTPHandler(w, w.Handler()))
	defer srv.Close()

	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "queued-pause", Planner: "llm"})
	postPauseControl(t, srv.URL+"/v1/runs/queued-pause/pause")
	postPauseControl(t, srv.URL+"/v1/runs/queued-pause/cancel")

	w.mu.Lock()
	tile := *w.tiles["queued-pause"]
	w.mu.Unlock()
	if tile.Status != statusDone || !tile.Finished || tile.Reason != "cancelled" {
		t.Fatalf("cancelled paused tile = status %q finished=%v reason=%q", tile.Status, tile.Finished, tile.Reason)
	}
}

func postPauseControl(t *testing.T, url string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("POST %s: status %d", url, res.StatusCode)
	}
}
