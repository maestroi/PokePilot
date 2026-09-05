package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill/fixture"
)

func TestCollectCheckpointArtifacts(t *testing.T) {
	dir := t.TempDir()
	writePair(t, dir, "round-002-frame-0000000200-goto.state", []byte("state-b"), []byte(`{"k":2}`))
	writePair(t, dir, "round-001-frame-0000000100-goto.state", []byte("state-a"), []byte(`{"k":1}`))
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := collectCheckpointArtifacts(dir)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("artifacts = %d, want 4 (two states + two knowledge): %v", len(got), namesOf(got))
	}
	wantOrder := []string{
		"round-001-frame-0000000100-goto.knowledge-v4.json",
		"round-001-frame-0000000100-goto.state",
		"round-002-frame-0000000200-goto.knowledge-v4.json",
		"round-002-frame-0000000200-goto.state",
	}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Fatalf("artifact %d name = %q, want %q (got %v)", i, got[i].Name, name, namesOf(got))
		}
		sum := sha256.Sum256(got[i].Data)
		if got[i].SHA256 != hex.EncodeToString(sum[:]) {
			t.Errorf("%s hash = %s, want %s", name, got[i].SHA256, hex.EncodeToString(sum[:]))
		}
	}
	if !bytes.Equal(got[1].Data, []byte("state-a")) || !bytes.Equal(got[3].Data, []byte("state-b")) {
		t.Fatal("state bytes not kept beside their knowledge files")
	}
}

func TestCollectCheckpointArtifactsPeriodicPairs(t *testing.T) {
	dir := t.TempDir()
	writePeriodic(t, dir, "periodic-00000018000.state", []byte("p-state"), []byte(`{"frame":18000}`))
	got, err := collectCheckpointArtifacts(dir)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 2 || got[0].Name != "periodic-00000018000.json" || got[1].Name != "periodic-00000018000.state" {
		t.Fatalf("periodic artifacts = %v", namesOf(got))
	}
}

func TestCollectCheckpointArtifactsRejects(t *testing.T) {
	t.Run("orphan knowledge", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "round-001-frame-0000000100-goto.knowledge-v4.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := collectCheckpointArtifacts(dir); err == nil {
			t.Fatal("orphan knowledge must be rejected")
		}
	})
	t.Run("missing pair", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "round-001-frame-0000000100-goto.state"), []byte("state"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := collectCheckpointArtifacts(dir); err == nil {
			t.Fatal("state without knowledge must be rejected")
		}
	})
	t.Run("unsafe name", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "bad name.state"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bad name.knowledge-v4.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := collectCheckpointArtifacts(dir); err == nil {
			t.Fatal("unsafe names must be rejected")
		}
	})
	t.Run("over budget", func(t *testing.T) {
		dir := t.TempDir()
		huge := bytes.Repeat([]byte("x"), farm.MaxFinishArtifactBytes+1)
		writePair(t, dir, "round-001-frame-0000000100-goto.state", huge, []byte("{}"))
		if _, err := collectCheckpointArtifacts(dir); err == nil {
			t.Fatal("over-budget content must be rejected")
		}
	})
}

func TestCollectCheckpointArtifactsEmptyDir(t *testing.T) {
	got, err := collectCheckpointArtifacts("")
	if err != nil || got != nil {
		t.Fatalf("empty dir = %v, %v", got, err)
	}
}

func TestEnqueuePeriodicDropsSuperseded(t *testing.T) {
	ch := make(chan periodicSample, 1)
	enqueuePeriodic(ch, periodicSample{Name: "a", State: []byte("1")})
	enqueuePeriodic(ch, periodicSample{Name: "b", State: []byte("2")})
	select {
	case got := <-ch:
		if got.Name != "b" {
			t.Fatalf("queued = %q, want the later sample b", got.Name)
		}
	default:
		t.Fatal("queue empty")
	}
	select {
	case extra := <-ch:
		t.Fatalf("queue held extra sample %q", extra.Name)
	default:
	}
}

func TestPeriodicCheckpointKeep(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 15; i++ {
		name := periodicStateName(uint64(i) * periodicCheckpointFrames)
		writePeriodic(t, dir, name, []byte("s"), []byte(`{}`))
	}
	if err := evictPeriodicCheckpoints(dir, periodicCheckpointKeep); err != nil {
		t.Fatalf("evict: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var states []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".state") {
			states = append(states, e.Name())
		}
	}
	if len(states) != periodicCheckpointKeep {
		t.Fatalf("kept %d periodic states, want %d: %v", len(states), periodicCheckpointKeep, states)
	}
	if states[0] != periodicStateName(4*periodicCheckpointFrames) {
		t.Fatalf("oldest kept = %s, want the 4th of 15", states[0])
	}
}

func TestCheckpointUploaderNeverTouchesEmu(t *testing.T) {
	src, err := os.ReadFile("farm_artifacts.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "func runCheckpointUploader") {
		t.Fatal("farm_artifacts.go has no runCheckpointUploader")
	}
	i := strings.Index(text, "func runCheckpointUploader")
	chunk := text[i:]
	if j := strings.Index(chunk, "\nfunc "); j > 0 {
		chunk = chunk[:j]
	}
	if strings.Contains(chunk, "*emu.Emu") {
		t.Fatal("runCheckpointUploader must not receive *emu.Emu")
	}
}

func TestFarmLLMBudgetReceivesCheckpointDir(t *testing.T) {
	src, err := os.ReadFile("farm.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "CheckpointDir: checkpointDir") {
		t.Fatal("runFarmLLM does not pass CheckpointDir into agent.Budget")
	}
	resumeSrc, err := os.ReadFile("farm_resume.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resumeSrc), "os.MkdirTemp(\"\", \"pokefarm-checkpoints-\")") {
		t.Fatal("attempt preparation does not create a per-lease checkpoint directory")
	}
	scripted := text[strings.Index(text, "func runFarmScripted"):]
	if i := strings.Index(scripted[1:], "\nfunc "); i > 0 {
		scripted = scripted[:i+1]
	}
	if strings.Contains(scripted, "CheckpointDir") || strings.Contains(scripted, "MkdirTemp") {
		t.Fatal("scripted farm runs must not manufacture checkpoints")
	}
}

func TestFinishRemovesCheckpointDirAfterReturn(t *testing.T) {
	dir, err := os.MkdirTemp("", "pokefarm-checkpoints-test-")
	if err != nil {
		t.Fatal(err)
	}
	writePair(t, dir, "round-001-frame-0000000100-goto.state", []byte("s"), []byte("{}"))

	var sawDir bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("checkpoint dir gone before Finish returned: %v", err)
		} else {
			sawDir = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := farm.NewClient(srv.URL)
	client.Version = "abc123"
	finishLeasedRun(nil, client, farm.Spec{RunID: "r1", Attempt: 1}, "error", "stuck", 7, dir)
	if !sawDir {
		t.Fatal("Finish handler never ran")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("checkpoint dir still present after Finish: %v", err)
	}
}

func TestBlockedCheckpointDoesNotBlockGameplay(t *testing.T) {
	var uploads atomic.Int32
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/runs/r1/checkpoint" {
			uploads.Add(1)
			select {
			case <-hang:
			case <-r.Context().Done():
			}
			return
		}
		json.NewEncoder(w).Encode(farm.HeartbeatReply{})
	}))
	defer srv.Close()
	defer close(hang)

	client := &farm.Client{BaseURL: srv.URL, HTTP: &http.Client{Timeout: 50 * time.Millisecond}}
	dir := t.TempDir()
	stop := make(chan struct{})
	samples := make(chan periodicSample, 1)
	done := runCheckpointUploader(client, "r1", 1, dir, samples, stop)

	started := time.Now()
	for i := 0; i < 20; i++ {
		enqueuePeriodic(samples, periodicSample{
			Name:  periodicStateName(uint64(i+1) * periodicCheckpointFrames),
			State: []byte("s"),
			Meta:  []byte(`{}`),
		})
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("uploader did not join; a blocked checkpoint wedged it")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("gameplay-side enqueue took %s", time.Since(started))
	}

	cancel := make(chan struct{})
	hbStop := make(chan struct{})
	hbDone := heartbeatLoop(client, "r1", (&heartbeatSnap{}).load, cancel, hbStop, time.Millisecond)
	close(hbStop)
	select {
	case <-hbDone:
	case <-time.After(heartbeatDeadline + 2*time.Second):
		t.Fatal("heartbeatLoop did not join while checkpoint endpoint was blocked")
	}
}

func TestFarmPeriodicCheckpointRoundTrip(t *testing.T) {
	if os.Getenv("POKEMON_RED_ROM") == "" {
		t.Skip("POKEMON_RED_ROM not set; cannot load checkpoints back")
	}
	m := fixture.Load(t, "reds_bedroom")
	var (
		captured []byte
		frame    uint64
		mapID    uint8
		x, y     uint8
		mem      state.Mem
		once     sync.Once
	)
	m.OnSample(func(em *emu.Emu) {
		once.Do(func() {
			g := state.Read(em, &mem)
			st, err := em.SaveState()
			if err != nil {
				t.Errorf("SaveState from OnSample: %v", err)
				return
			}
			captured = st
			frame = em.FrameCount()
			mapID, x, y = g.Player.MapID, g.Player.X, g.Player.Y
		})
	})
	m.StepFrames(8)
	if captured == nil {
		t.Fatal("OnSample never captured a save state")
	}

	m2, err := emu.Open(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Fatalf("open ROM: %v", err)
	}
	t.Cleanup(func() { m2.Close() })
	if err := m2.LoadState(captured); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	g := state.Read(m2, &mem)
	if m2.FrameCount() != frame || g.Player.MapID != mapID || g.Player.X != x || g.Player.Y != y {
		t.Fatalf("loaded checkpoint frame/map/tile = %d map=%#02x (%d,%d), want %d map=%#02x (%d,%d)",
			m2.FrameCount(), g.Player.MapID, g.Player.X, g.Player.Y, frame, mapID, x, y)
	}
}

func writePair(t *testing.T, dir, stateName string, state, knowledge []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, stateName), state, 0o644); err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(stateName, ".state")
	if err := os.WriteFile(filepath.Join(dir, base+".knowledge-v4.json"), knowledge, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePeriodic(t *testing.T, dir, stateName string, state, meta []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, stateName), state, 0o644); err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(stateName, ".state")
	if err := os.WriteFile(filepath.Join(dir, base+".json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
}

func namesOf(arts []farm.Artifact) []string {
	out := make([]string, len(arts))
	for i, a := range arts {
		out[i] = a.Name
	}
	return out
}
