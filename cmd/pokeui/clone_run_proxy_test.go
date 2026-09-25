package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCloneRunIsProxiedOnlyByOperator(t *testing.T) {
	var hits atomic.Int32
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs/run-source/clone" {
			http.Error(w, "unexpected upstream request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read clone body: %v", err)
		}
		if string(body) != "{}" {
			t.Fatalf("clone body = %q, want {}", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"run_id":"run-clone","cloned_from":"run-source","status":"queued"}`))
	}))
	defer wall.Close()

	operator := httptest.NewServer(handlerWithServices(wall.URL, "", ""))
	defer operator.Close()

	resp, err := http.Post(operator.URL+"/v1/runs/run-source/clone", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("operator clone: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("operator clone = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if hits.Load() != 1 {
		t.Fatalf("operator upstream hits = %d, want 1", hits.Load())
	}

	spectator := httptest.NewServer(spectatorHandlerWithReplay(wall.URL, ""))
	defer spectator.Close()

	resp, err = http.Post(spectator.URL+"/v1/runs/run-source/clone", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("spectator clone: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("spectator clone = %d, want rejected", resp.StatusCode)
	}
	if hits.Load() != 1 {
		t.Fatalf("spectator reached wall; upstream hits = %d, want 1", hits.Load())
	}
}
