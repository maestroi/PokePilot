package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
)

func TestMaterializeFarmResumeKeepsStateAndKnowledgePaired(t *testing.T) {
	dir := t.TempDir()
	state := runnerResumeArtifact("round-007-frame-0000000700-goto.state", []byte("emulator-state"), "application/octet-stream")
	knowledge := runnerResumeArtifact("round-007-frame-0000000700-goto.knowledge-v4.json", []byte(`{"intent":"recover from pewter"}`), "application/json")
	cp := farm.ResumeCheckpoint{Attempt: 1, State: state, Knowledge: &knowledge}
	if err := materializeFarmResume(dir, cp); err != nil {
		t.Fatal(err)
	}
	if got := farmResumePath(dir); got != filepath.Join(dir, state.Name) {
		t.Fatalf("resume path = %q", got)
	}
	if got, err := os.ReadFile(filepath.Join(dir, knowledge.Name)); err != nil || string(got) != string(knowledge.Data) {
		t.Fatalf("knowledge = %q, %v", got, err)
	}
}

func TestMaterializeFarmResumeRejectsStateWithoutKnowledge(t *testing.T) {
	state := runnerResumeArtifact("round-001-frame-0000000100-goto.state", []byte("state"), "application/octet-stream")
	if err := materializeFarmResume(t.TempDir(), farm.ResumeCheckpoint{State: state}); err == nil {
		t.Fatal("missing knowledge should reject LLM resume")
	}
}

func TestRunFarmLLMWiresResumeIntoAgentBudget(t *testing.T) {
	src, err := os.ReadFile("farm.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "resumeFrom := farmResumePath(checkpointDir)") {
		t.Fatal("runFarmLLM does not resolve the durable resume marker")
	}
	if !strings.Contains(text, "ResumeFrom:    resumeFrom") {
		t.Fatal("runFarmLLM does not pass ResumeFrom into agent.Budget")
	}
	if !strings.Contains(text, "starter != \"\" && resumeFrom == \"\"") {
		t.Fatal("resumed LLM run would replay starter acquisition")
	}
}

func TestPrepareFarmAttemptLooksUpResumeOnFirstAttempt(t *testing.T) {
	src, err := os.ReadFile("farm_resume.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if strings.Contains(text, "spec.Attempt > 1") {
		t.Fatal("endless successors are attempt 1; resume lookup must not skip them")
	}
	if !strings.Contains(text, "planner == \"llm\" && dir != \"\"") {
		t.Fatal("LLM resume lookup is no longer wired")
	}
}

func TestResumeFallbackWarningOnlyFiresWhenResumeWasExpected(t *testing.T) {
	if warn, _ := resumeFallbackWarning(1, false, "no checkpoint found"); warn {
		t.Fatal("an ordinary first attempt with no expected resume must not warn")
	}
	warn, msg := resumeFallbackWarning(2, true, "resume lookup failed: dial tcp: timeout")
	if !warn {
		t.Fatal("a retry that fell back to a fresh cartridge must warn")
	}
	if !strings.Contains(msg, "attempt 2") || !strings.Contains(msg, "fresh cartridge") || !strings.Contains(msg, "resume lookup failed") {
		t.Fatalf("message missing expected detail: %q", msg)
	}
	if warn, msg := resumeFallbackWarning(1, true, ""); !warn || !strings.Contains(msg, "resume not attempted") {
		t.Fatalf("endless successor with no fallback reason should still warn with a placeholder: warn=%v msg=%q", warn, msg)
	}
}

func runnerResumeArtifact(name string, data []byte, mediaType string) farm.Artifact {
	sum := sha256.Sum256(data)
	return farm.Artifact{Name: name, MediaType: mediaType, SHA256: hex.EncodeToString(sum[:]), Data: data}
}

// TestMaterializeFarmResumeRejectsEmptyState keeps a zero-byte resume state out
// of the attempt's checkpoint ring entirely. If it were materialized it would
// fail LoadState and then be re-published by the uploader as this attempt's own
// checkpoint, which is how one mid-write read used to restart a run forever.
func TestMaterializeFarmResumeRejectsEmptyState(t *testing.T) {
	dir := t.TempDir()
	knowledge := runnerResumeArtifact("round-001-frame-0003842410-goto.knowledge-v6.json", []byte(`{"intent":"x"}`), "application/json")
	cp := farm.ResumeCheckpoint{
		Attempt:   1,
		State:     runnerResumeArtifact("round-001-frame-0003842410-goto.state", nil, "application/octet-stream"),
		Knowledge: &knowledge,
	}
	if err := materializeFarmResume(dir, cp); err == nil {
		t.Fatal("materialize accepted an empty resume state")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("rejected resume left %d file(s) in the checkpoint ring (err %v)", len(entries), err)
	}
}

func TestDiscardFarmResumeRemovesRejectedPair(t *testing.T) {
	dir := t.TempDir()
	state := runnerResumeArtifact("round-001-frame-0003842410-goto.state", []byte("state"), "application/octet-stream")
	knowledge := runnerResumeArtifact("round-001-frame-0003842410-goto.knowledge-v6.json", []byte(`{"intent":"x"}`), "application/json")
	cp := farm.ResumeCheckpoint{Attempt: 1, State: state, Knowledge: &knowledge}
	if err := materializeFarmResume(dir, cp); err != nil {
		t.Fatal(err)
	}
	discardFarmResume(dir, cp)
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("discarded resume left %d file(s) behind (err %v)", len(entries), err)
	}
	if got := farmResumePath(dir); got != "" {
		t.Fatalf("resume marker survived discard: %q", got)
	}
}

// TestMaterializeFarmResumeNeverExposesPartialState is the root-cause
// regression: the checkpoint uploader polls this directory on a timer, so the
// materialized state must appear whole or not at all. os.WriteFile truncates in
// place first, and a 321 KB state read inside that window is exactly what the
// wall once stored as a 0-byte checkpoint.
func TestMaterializeFarmResumeNeverExposesPartialState(t *testing.T) {
	dir := t.TempDir()
	name := "round-001-frame-0003842410-progress-secret-key-owned.state"
	payload := bytes.Repeat([]byte("emulator-state"), 321616/14)
	knowledge := runnerResumeArtifact("round-001-frame-0003842410-progress-secret-key-owned.knowledge-v6.json", []byte(`{"intent":"x"}`), "application/json")
	cp := farm.ResumeCheckpoint{
		Attempt:   1,
		State:     runnerResumeArtifact(name, payload, "application/octet-stream"),
		Knowledge: &knowledge,
	}

	var partial, complete int64
	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				continue // the rename has not landed yet: no state is visible
			}
			if len(b) != len(payload) {
				atomic.AddInt64(&partial, 1)
				continue
			}
			atomic.AddInt64(&complete, 1)
		}
	}()
	for i := 0; i < 250; i++ {
		if err := materializeFarmResume(dir, cp); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	<-readerDone
	if partial != 0 {
		t.Fatalf("reader observed %d partial writes (%d complete)", partial, complete)
	}
}

// A retry whose resume lookup fails has not been told "nothing to resume", so
// it must end the attempt rather than boot a fresh cartridge whose early
// checkpoints would replace the campaign's progress (run-s6v9q3t2w5rl).
func TestPrepareFarmAttemptRefusesFreshBootWhenExpectedResumeLookupFails(t *testing.T) {
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	m, err := emu.Open(rom)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	srv := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	boot, err := m.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	frame := m.FrameCount()
	spec := farm.Spec{RunID: "deep-campaign", Attempt: 358, Planner: "llm", Endless: true}
	_, _, err = prepareFarmAttempt(m, farm.NewClient(srv.URL), spec, "llm", boot, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "resume lookup failed") {
		t.Fatalf("prepare = %v, want resume lookup failure", err)
	}
	if m.FrameCount() != frame {
		t.Fatal("attempt booted a fresh cartridge after a failed resume lookup")
	}
}
