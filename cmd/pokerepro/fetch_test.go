package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeWall serves the three read-only routes pokerepro uses: two checkpoints
// (one of them unusable), the artifact index, and artifact content.
func fakeWall(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs/r1/checkpoints", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"attempt":2,"checkpoints":[
			{"name":"round-071-b.state","round":71,"replayable":true,"has_knowledge":false},
			{"name":"round-066-a.state","round":66,"replayable":true,"has_knowledge":true}]}`))
	})
	mux.HandleFunc("/v1/runs/r1/artifacts", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"artifacts":[
			{"name":"round-066-a.knowledge-v3.json"},
			{"name":"round-066-a.knowledge-v4.json"},
			{"name":"round-066-a.state"}]}`))
	})
	mux.HandleFunc("/v1/runs/r1/artifacts/{name}/content", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.PathValue("name")))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchCheckpointLatestSkipsUnpairedAndTakesNewestKnowledge(t *testing.T) {
	srv := fakeWall(t)
	cp, err := fetchCheckpoint(context.Background(), srv.Client(), srv.URL, "r1", 0, "latest")
	if err != nil {
		t.Fatalf("fetchCheckpoint: %v", err)
	}
	// round-071 is newer but has no paired knowledge, so it is not replayable.
	if cp.State.Name != "round-066-a.state" || cp.Attempt != 2 {
		t.Fatalf("state = %q attempt %d", cp.State.Name, cp.Attempt)
	}
	if cp.Knowledge.Name != "round-066-a.knowledge-v4.json" {
		t.Fatalf("knowledge = %q, want the highest version", cp.Knowledge.Name)
	}
	if string(cp.State.Data) != "round-066-a.state" {
		t.Fatalf("state data = %q", cp.State.Data)
	}
}

func TestFetchCheckpointRejectsUnusableName(t *testing.T) {
	srv := fakeWall(t)
	if _, err := fetchCheckpoint(context.Background(), srv.Client(), srv.URL, "r1", 0, "round-071-b.state"); err == nil {
		t.Fatal("want error for a checkpoint with no paired knowledge")
	}
}
