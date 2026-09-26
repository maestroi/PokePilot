package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestRunOneSettlesLinkStallWithoutTouchingPoisonedEmulator(t *testing.T) {
	src := functionSource(t, "farm.go", "runOne")
	start := strings.Index(src, "if emulatorPoisoned {")
	if start < 0 {
		t.Fatal("runOne has no poisoned-emulator branch")
	}
	endRel := strings.Index(src[start:], "return true")
	if endRel < 0 {
		t.Fatal("poisoned-emulator branch does not recycle the worker")
	}
	block := src[start : start+endRel]
	if strings.Contains(block, "m.") {
		t.Fatalf("poisoned-emulator branch still touches m:\n%s", block)
	}
	if !strings.Contains(block, "finishRunWithRecording(nil, client, spec") {
		t.Fatal("poisoned-emulator branch does not settle the run without an emulator")
	}
}

func TestRunFarmLLMMarksLinkStallAsPoisoned(t *testing.T) {
	src := functionSource(t, "farm.go", "runFarmLLM")
	if !strings.Contains(src, "errors.Is(res.Err, skill.ErrLinkStalled)") {
		t.Fatal("runFarmLLM does not surface ErrLinkStalled as an emulator-poisoning fault")
	}
}

func TestFinishRunWithRecordingAllowsNilEmulator(t *testing.T) {
	var got farm.FinishReport
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs/poisoned/finish" {
			t.Errorf("finish request = %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode finish: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := farm.NewClient(srv.URL)
	client.Version = "test"
	finishRunWithRecording(nil, client, farm.Spec{
		RunID:   "poisoned",
		Attempt: 1,
		Planner: "llm",
		Game:    "pokemon_red",
	}, "error", "link stalled", 0, t.TempDir(), nil, nil, nil)

	if got.RunID != "poisoned" || got.Attempt != 1 || got.Reason != "error" {
		t.Fatalf("finish report = %+v", got)
	}
	if len(got.SaveState) != 0 || len(got.TraceTail) != 0 {
		t.Fatalf("poisoned finish touched emulator evidence: save=%d trace=%v", len(got.SaveState), got.TraceTail)
	}
}

func TestFarmModeHardExitsWhenWorkerNeedsRecycle(t *testing.T) {
	src := functionSource(t, "main.go", "main")
	if !strings.Contains(src, "if runFarm(m, client, library, watchPort(served), *checkpointDir, renderFeed, drainCtx.Done())") {
		t.Fatal("main does not inspect farm worker recycle result")
	}
	if !strings.Contains(src, "os.Exit(1)") {
		t.Fatal("main does not hard-exit before deferred m.Close on a poisoned emulator")
	}
}
