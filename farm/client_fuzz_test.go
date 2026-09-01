package farm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func FuzzClientHeartbeatPreservesRunID(f *testing.F) {
	for _, seed := range []string{"simple", "with space", "slash/inside", "../dot", "ümlaut", "question?mark", "hash#mark"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, runID string) {
		if runID == "" {
			return
		}
		var pathID string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pathID = r.PathValue("id")
			if pathID == "" {
				// httptest's plain handler has no ServeMux route variables; recover
				// the final escaped segment through the same mux shape as pokewall.
				mux := http.NewServeMux()
				mux.HandleFunc("POST /v1/runs/{id}/heartbeat", func(w http.ResponseWriter, r *http.Request) {
					pathID = r.PathValue("id")
					_ = json.NewEncoder(w).Encode(HeartbeatReply{})
				})
				mux.ServeHTTP(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(HeartbeatReply{})
		}))
		defer srv.Close()

		client := NewClient(srv.URL)
		if _, err := client.Heartbeat(context.Background(), Heartbeat{RunID: runID}); err != nil {
			t.Fatalf("Heartbeat(%q): %v", runID, err)
		}
		if pathID != runID {
			t.Fatalf("path run id = %q, want %q", pathID, runID)
		}
	})
}
