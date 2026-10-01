package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// pokeui fetches repro artifact bytes through pokereplay, so the replay sidecar
// must scope an ?attempt= artifact read to that attempt's catalog. Without this
// an older failing attempt's bundle is looked up in the latest attempt's
// artifact list and comes back 404.
func TestArtifactContentHonorsAttemptSelector(t *testing.T) {
	var mu sync.Mutex
	var requested []string
	record := func(r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requested = append(requested, r.URL.Path+"?"+r.URL.RawQuery)
	}

	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/runs/run-1/artifacts":
			attempt, _ := strconv.Atoi(r.URL.Query().Get("attempt"))
			_ = json.NewEncoder(w).Encode(artifactList{
				RunID: "run-1", Attempt: attempt,
				Artifacts: []artifactRef{{Name: "repro.json", Inline: true}},
			})
		case "/v1/runs/run-1/artifacts/repro.json/content":
			_, _ = w.Write([]byte(`{"attempt":` + r.URL.Query().Get("attempt") + `}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	s := &replayServer{wallBase: wall.URL, wallHTTP: wall.Client()}
	serve := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.SetPathValue("id", "run-1")
		req.SetPathValue("name", "repro.json")
		rec := httptest.NewRecorder()
		s.handleArtifactContent(rec, req)
		return rec
	}

	rec := serve("/v1/runs/run-1/artifacts/repro.json/content?attempt=23")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"attempt":23}` {
		t.Fatalf("attempt=23 status=%d body=%q", rec.Code, rec.Body.String())
	}
	mu.Lock()
	got := strings.Join(requested, "\n")
	mu.Unlock()
	if !strings.Contains(got, "/v1/runs/run-1/artifacts?attempt=23") {
		t.Fatalf("catalog was not scoped to attempt 23:\n%s", got)
	}
	if !strings.Contains(got, "/v1/runs/run-1/artifacts/repro.json/content?attempt=23") {
		t.Fatalf("inline read did not forward attempt 23:\n%s", got)
	}

	// No selector keeps the latest-attempt default.
	if rec := serve("/v1/runs/run-1/artifacts/repro.json/content"); rec.Code != http.StatusOK {
		t.Fatalf("latest status = %d", rec.Code)
	}
	mu.Lock()
	latest := requested[len(requested)-2]
	mu.Unlock()
	if strings.Contains(latest, "attempt=") {
		t.Fatalf("latest catalog request carried an attempt selector: %s", latest)
	}

	// An invalid selector is rejected instead of silently reading latest.
	for _, target := range []string{
		"/v1/runs/run-1/artifacts/repro.json/content?attempt=0",
		"/v1/runs/run-1/artifacts/repro.json/content?attempt=abc",
	} {
		if rec := serve(target); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", target, rec.Code)
		}
	}
}
