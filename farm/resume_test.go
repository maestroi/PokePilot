package farm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResumeCheckpointClient(t *testing.T) {
	state := resumeArtifact("round-004-frame-0000000400-goto.state", []byte("state"), "application/octet-stream")
	knowledge := resumeArtifact("round-004-frame-0000000400-goto.knowledge-v4.json", []byte(`{"intent":"leave pewter"}`), "application/json")
	var got struct {
		RunID   string `json:"run_id"`
		Attempt int    `json:"attempt"`
		Resume  bool   `json:"resume"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/long-run/checkpoint" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		writeResumeJSON(t, w, ResumeCheckpoint{Attempt: 3, State: state, Knowledge: &knowledge})
	}))
	defer srv.Close()

	cp, err := NewClient(srv.URL).ResumeCheckpoint(context.Background(), "long-run", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Resume || got.RunID != "long-run" || got.Attempt != 4 {
		t.Fatalf("request = %+v", got)
	}
	if cp == nil || cp.Attempt != 3 || cp.State.Name != state.Name || cp.Knowledge == nil || cp.Knowledge.Name != knowledge.Name {
		t.Fatalf("checkpoint = %+v", cp)
	}
}

func TestResumeCheckpointClientNoContentMeansFreshStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	cp, err := NewClient(srv.URL).ResumeCheckpoint(context.Background(), "fresh", 2)
	if err != nil {
		t.Fatal(err)
	}
	if cp != nil {
		t.Fatalf("checkpoint = %+v, want nil", cp)
	}
}

func resumeArtifact(name string, data []byte, mediaType string) Artifact {
	sum := sha256.Sum256(data)
	return Artifact{Name: name, MediaType: mediaType, SHA256: hex.EncodeToString(sum[:]), Data: data}
}

func writeResumeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}
