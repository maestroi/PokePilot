package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWorkerForceEndIsProxiedOnlyByOperator(t *testing.T) {
	var hits atomic.Int32
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/workers/10.0.1.23:8099/force-end" {
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"force-ended"}`))
	}))
	defer wall.Close()

	operator := httptest.NewServer(handlerWithServices(wall.URL, "", ""))
	defer operator.Close()
	resp, err := http.Post(operator.URL+"/v1/workers/10.0.1.23%3A8099/force-end", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("operator force-end: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("operator force-end = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if hits.Load() != 1 {
		t.Fatalf("operator upstream hits = %d, want 1", hits.Load())
	}

	spectator := httptest.NewServer(spectatorHandlerWithReplay(wall.URL, ""))
	defer spectator.Close()
	resp, err = http.Post(spectator.URL+"/v1/workers/10.0.1.23%3A8099/force-end", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("spectator force-end: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("spectator force-end = %d, want rejected", resp.StatusCode)
	}
	if hits.Load() != 1 {
		t.Fatalf("spectator reached wall; upstream hits = %d, want 1", hits.Load())
	}
}
