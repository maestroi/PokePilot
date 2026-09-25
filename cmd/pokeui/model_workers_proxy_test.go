package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestModelWorkerPatchIsProxiedOnlyByOperator(t *testing.T) {
	var hits atomic.Int32
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/models/qwen38-27b-7900" {
			http.Error(w, "unexpected upstream request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"qwen38-27b-7900","max_parallel_workers":2}`))
	}))
	defer wall.Close()

	operator := httptest.NewServer(handlerWithServices(wall.URL, "", ""))
	defer operator.Close()
	req, err := http.NewRequest(http.MethodPatch, operator.URL+"/v1/models/qwen38-27b-7900", strings.NewReader(`{"max_parallel_workers":2}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("operator patch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("operator patch = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if hits.Load() != 1 {
		t.Fatalf("operator upstream hits = %d, want 1", hits.Load())
	}

	spectator := httptest.NewServer(spectatorHandlerWithReplay(wall.URL, ""))
	defer spectator.Close()
	req, err = http.NewRequest(http.MethodPatch, spectator.URL+"/v1/models/qwen38-27b-7900", strings.NewReader(`{"max_parallel_workers":2}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("spectator patch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("spectator patch = %d, want rejected", resp.StatusCode)
	}
	if hits.Load() != 1 {
		t.Fatalf("spectator reached wall; upstream hits = %d, want 1", hits.Load())
	}
}

func TestModelEndpointManagementIsProxiedOnlyByOperator(t *testing.T) {
	var hits atomic.Int32
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"lab-gpu"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/models/test":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"lab-gpu","model_id":"dynamic-9b"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/models/lab-gpu":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected upstream request "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
		}
	}))
	defer wall.Close()

	operator := httptest.NewServer(handlerWithServices(wall.URL, "", ""))
	defer operator.Close()
	spectator := httptest.NewServer(spectatorHandlerWithReplay(wall.URL, ""))
	defer spectator.Close()

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/v1/models", `{"id":"lab-gpu"}`},
		{http.MethodPost, "/v1/models/test", `{"id":"lab-gpu","endpoint":"http://127.0.0.1:9/v1"}`},
		{http.MethodDelete, "/v1/models/lab-gpu", ""},
	} {
		req, err := http.NewRequest(tc.method, operator.URL+tc.path, strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("operator %s %s: %v", tc.method, tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotFound {
			t.Fatalf("operator %s %s = %d, want proxied model-management route", tc.method, tc.path, resp.StatusCode)
		}
		if resp.StatusCode >= 400 {
			t.Fatalf("operator %s %s = %d, want success from wall", tc.method, tc.path, resp.StatusCode)
		}

		req, err = http.NewRequest(tc.method, spectator.URL+tc.path, strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("spectator %s %s: %v", tc.method, tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Fatalf("spectator %s %s = %d, want rejected", tc.method, tc.path, resp.StatusCode)
		}
	}
	if hits.Load() != 3 {
		t.Fatalf("operator upstream hits = %d, want 3", hits.Load())
	}
}
