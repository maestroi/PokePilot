package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// The bundle download must ask for the same attempt the packet describes.
// Otherwise pokereplay (and pokewall behind it) resolve the artifact name
// against the run's latest attempt, and an older failing attempt's bundle comes
// back 404 even though the packet named it.
func TestFetchDebugArtifactScopesToAttempt(t *testing.T) {
	var mu sync.Mutex
	var attempts []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		attempts = append(attempts, req.Params.Arguments["attempt"])
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{"structuredContent": map[string]any{
				"run_id":         "run-x",
				"name":           req.Params.Arguments["name"],
				"content_base64": base64.StdEncoding.EncodeToString([]byte("payload")),
			}},
		})
	}))
	t.Cleanup(srv.Close)

	data, err := fetchDebugArtifact(context.Background(), srv.URL, "token", "run-x", "repro.json", 23)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Fatalf("data = %q", data)
	}
	mu.Lock()
	got := append([]any(nil), attempts...)
	mu.Unlock()
	if len(got) != 1 || got[0] != float64(23) {
		t.Fatalf("attempt argument = %#v, want 23", got)
	}

	// Zero stays omitted so the historical latest-attempt default is unchanged.
	if _, err := fetchDebugArtifact(context.Background(), srv.URL, "token", "run-x", "repro.json", 0); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got = append([]any(nil), attempts...)
	mu.Unlock()
	if len(got) != 2 || got[1] != nil {
		t.Fatalf("latest attempt argument = %#v, want omitted", got)
	}
}
