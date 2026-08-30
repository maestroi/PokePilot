package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

// TestMainWiresFarmMode is the regression for 96eaf02: that merge kept
// runFarm but deleted the POKEPILOT_ORCH_URL gate in main(), so Swarm
// runners ran the one-shot CLI path and queued specs never leased.
func TestMainWiresFarmMode(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, `os.Getenv("POKEPILOT_ORCH_URL")`) {
		t.Fatal("main() does not read POKEPILOT_ORCH_URL; farm workers ignore the wall")
	}
	if !strings.Contains(text, "runFarm(") {
		t.Fatal("main() never calls runFarm")
	}
}

func TestWatchPort(t *testing.T) {
	if got := watchPort("[::]:8099"); got != 8099 {
		t.Fatalf("watchPort([::]:8099) = %d, want 8099", got)
	}
	if got := watchPort("localhost:18080"); got != 18080 {
		t.Fatalf("watchPort(localhost:18080) = %d, want 18080", got)
	}
	if got := watchPort("not-an-addr"); got != 0 {
		t.Fatalf("watchPort(bad) = %d, want 0", got)
	}
}

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

func TestFarmLLMAppliesSpecGoal(t *testing.T) {
	src, err := os.ReadFile("farm.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "planner.Goal = goal") {
		t.Fatal("runFarmLLM does not set planner.Goal from the leased spec")
	}
	if !strings.Contains(text, "spec.Goal") {
		t.Fatal("runFarm never passes spec.Goal into the llm run")
	}
	if !strings.Contains(text, "reportingPlanner{inner: planner, snap: snap}") {
		t.Fatal("runFarmLLM does not publish the latest plan onto the heartbeat snap")
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

// TestHeartbeatSnapKeepsPlan is the race the watch pane depends on:
// sampleHeartbeat rewrites Frame/Map/Trace every tick, and must not
// blank the question/decision the planner wrote while the model was
// thinking (when the stepper is blocked in HTTP and not sampling).
func TestHeartbeatSnapKeepsPlan(t *testing.T) {
	s := &heartbeatSnap{}
	s.store(farm.Heartbeat{RunID: "r1", Frame: 10, Trace: "control: control regained"})
	s.storePlan("1: go to pallet town\n2: talk at (5,3)", "")
	got := s.load()
	if got.Question != "1: go to pallet town\n2: talk at (5,3)" || got.Decision != "" || got.Frame != 10 {
		t.Fatalf("after storePlan: %+v", got)
	}

	s.storeStatus(farm.Heartbeat{RunID: "r1", Frame: 11, Map: 0x28, X: 5, Y: 6, Trace: "map: map 0x28 -> 0x00"})
	got = s.load()
	if got.Frame != 11 || got.Trace != "map: map 0x28 -> 0x00" {
		t.Fatalf("storeStatus dropped live fields: %+v", got)
	}
	if got.Question != "1: go to pallet town\n2: talk at (5,3)" || got.Decision != "" {
		t.Fatalf("storeStatus wiped the in-flight plan: %+v", got)
	}

	s.storePlan("1: go to pallet town\n2: talk at (5,3)", "go to pallet town")
	got = s.load()
	if got.Decision != "go to pallet town" || got.Frame != 11 {
		t.Fatalf("after decision: %+v", got)
	}
}

// blockingPlanner parks in Next until release is closed, so the test
// can observe the snap after the question is published and before the
// decision exists.
type blockingPlanner struct {
	entered chan struct{}
	release chan struct{}
	obj     agent.Objective
}

func (p blockingPlanner) Next(obs agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	close(p.entered)
	<-p.release
	return p.obj, nil
}

func TestReportingPlannerPublishesQuestionBeforeReply(t *testing.T) {
	snap := &heartbeatSnap{}
	entered := make(chan struct{})
	release := make(chan struct{})
	want := agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	p := reportingPlanner{
		inner: blockingPlanner{entered: entered, release: release, obj: want},
		snap:  snap,
	}
	offered := []agent.Objective{
		want,
		{Kind: agent.KindTalk, X: 5, Y: 3},
	}

	done := make(chan error, 1)
	go func() {
		got, err := p.Next(agent.Observation{}, offered)
		if err != nil {
			done <- err
			return
		}
		if got != want {
			done <- fmt.Errorf("Next = %s, want %s", got, want)
			return
		}
		done <- nil
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("planner never entered Next")
	}
	got := snap.load()
	if !strings.Contains(got.Question, "1: go to pallet town") || !strings.Contains(got.Question, "2: talk at (5,3)") {
		t.Fatalf("question while thinking = %q", got.Question)
	}
	if got.Decision != "" {
		t.Fatalf("decision set before the model replied: %q", got.Decision)
	}

	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("planner did not return after release")
	}
	got = snap.load()
	if got.Decision != "go to pallet town" {
		t.Fatalf("decision after reply = %q, want go to pallet town", got.Decision)
	}
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
