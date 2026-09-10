package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestCheckpointReproQueuesFreshRunAndPinsSource(t *testing.T) {
	dumps := t.TempDir()
	w := NewWall(dumps)
	const sourceID = "failed-elite-four"
	sourceSpec := farm.Spec{
		RunID: sourceID, Planner: "llm", Starter: "squirtle", Goal: "elite-four",
		LLMProfile: "auto", Seed: 77, FPS: 0, MaxRounds: 400, MaxFrames: 900000,
		Endless: true, RandomSeed: true,
	}
	w.mu.Lock()
	w.order = append(w.order, sourceID)
	w.tiles[sourceID] = &Tile{}
	w.applySpec(sourceID, sourceSpec)
	w.tiles[sourceID].Status = statusDone
	w.tiles[sourceID].Finished = true
	w.tiles[sourceID].Attempts = 1
	w.mu.Unlock()

	checkpointDir := checkpointAttemptDir(dumps, sourceID, 1)
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := "round-012-frame-0000468000-goto"
	stateName := base + ".state"
	knowledgeName := base + ".knowledge-v4.json"
	stateBytes := []byte("emulator-state")
	knowledgeBytes := []byte(`{"knowledge":"before failure"}`)
	if err := os.WriteFile(filepath.Join(checkpointDir, stateName), stateBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkpointDir, knowledgeName), knowledgeBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(operatorHTTPHandler(w))
	defer srv.Close()

	body := bytes.NewBufferString(`{"checkpoint":"latest"}`)
	resp, err := http.Post(srv.URL+"/v1/runs/"+sourceID+"/repro", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("queue repro status = %d", resp.StatusCode)
	}
	var queued farm.ReplayQueued
	if err := json.NewDecoder(resp.Body).Decode(&queued); err != nil {
		t.Fatal(err)
	}
	if queued.RunID == "" || queued.RunID == sourceID || queued.Source.SourceRunID != sourceID || queued.Source.Checkpoint != stateName {
		t.Fatalf("queued = %+v", queued)
	}
	if queued.Source.SourceAttempt != 1 {
		t.Fatalf("source attempt = %d", queued.Source.SourceAttempt)
	}

	client := farm.NewClient(srv.URL)
	lease, err := client.Lease(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if lease == nil || lease.RunID != queued.RunID || lease.Goal != sourceSpec.Goal || lease.Seed != sourceSpec.Seed {
		t.Fatalf("lease = %+v", lease)
	}
	if lease.Endless || lease.RandomSeed {
		t.Fatalf("repro must be finite and deterministic: %+v", lease)
	}

	cp, err := client.ReplayCheckpoint(t.Context(), queued.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if cp == nil || cp.State.Name != stateName || string(cp.State.Data) != string(stateBytes) || cp.Knowledge == nil || cp.Knowledge.Name != knowledgeName {
		t.Fatalf("replay checkpoint = %+v", cp)
	}

	local, err := client.FetchCheckpoint(t.Context(), sourceID, 1, stateName)
	if err != nil {
		t.Fatal(err)
	}
	if local.State.Name != stateName || local.Knowledge == nil || local.Knowledge.Name != knowledgeName {
		t.Fatalf("local checkpoint = %+v", local)
	}

	listResp, err := http.Get(srv.URL + "/v1/runs/" + sourceID + "/checkpoints")
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list checkpoints status = %d", listResp.StatusCode)
	}
	var list struct {
		Checkpoints []checkpointView `json:"checkpoints"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Checkpoints) != 1 || !list.Checkpoints[0].Replayable || !list.Checkpoints[0].HasKnowledge || list.Checkpoints[0].Name != stateName {
		t.Fatalf("checkpoint list = %+v", list.Checkpoints)
	}

	// Repro provenance is a dumps sidecar, not volatile wall memory. A wall
	// restart can still resolve exactly which source state this run means.
	restarted := NewWall(dumps)
	source, err := restarted.readReplaySource(queued.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if source.SourceRunID != sourceID || source.Checkpoint != stateName || source.SourceAttempt != 1 {
		t.Fatalf("persisted source = %+v", source)
	}
}

func TestReplayCheckpointReturnsNoContentForOrdinaryRun(t *testing.T) {
	w := NewWall(t.TempDir())
	srv := httptest.NewServer(operatorHTTPHandler(w))
	defer srv.Close()

	cp, err := farm.NewClient(srv.URL).ReplayCheckpoint(t.Context(), "ordinary-run")
	if err != nil {
		t.Fatal(err)
	}
	if cp != nil {
		t.Fatalf("ordinary run replay checkpoint = %+v", cp)
	}
}

func TestMajorCheckpointWithPairedKnowledgeIsReplayable(t *testing.T) {
	dumps := t.TempDir()
	w := NewWall(dumps)
	const runID = "major-checkpoint-run"
	dir := checkpointAttemptDir(dumps, runID, 1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	state, knowledge := majorWallPair(2, 29, "major-state")
	for _, artifact := range []farm.Artifact{state, knowledge} {
		if err := os.WriteFile(filepath.Join(dir, artifact.Name), artifact.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	views, err := w.checkpointViews(runID, 1, "llm")
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("major checkpoint views = %+v", views)
	}
	view := views[0]
	if view.Kind != "major" || view.Round != 29 || view.Frame != 2900 || !view.HasKnowledge || !view.Replayable {
		t.Fatalf("major checkpoint view = %+v", view)
	}
	cp, err := w.replayCheckpoint(runID, 1, "llm", state.Name)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Knowledge == nil || cp.Knowledge.Name != knowledge.Name {
		t.Fatalf("major replay checkpoint = %+v", cp)
	}
}
