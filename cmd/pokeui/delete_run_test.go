package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDeleteRunPurgesArtifactsBeforeWallHistory(t *testing.T) {
	var calls []string
	replay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/runs/run-1/artifacts" {
			t.Fatalf("unexpected replay request: %s %s", r.Method, r.URL.Path)
		}
		calls = append(calls, "replay")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"purged"}`))
	}))
	defer replay.Close()

	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run-1" {
			t.Fatalf("unexpected wall path: %s %s", r.Method, r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			calls = append(calls, "inspect")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"run":{"status":"done"}}`))
		case http.MethodDelete:
			calls = append(calls, "wall")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"deleted":true}`))
		default:
			t.Fatalf("unexpected wall request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer wall.Close()

	ui := httptest.NewServer(handlerWithServices(wall.URL, replay.URL, ""))
	defer ui.Close()
	req, err := http.NewRequest(http.MethodDelete, ui.URL+"/v1/runs/run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("DELETE run = %d, want 200", res.StatusCode)
	}
	if want := []string{"inspect", "replay", "wall"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestDeleteRunRetainsWallHistoryWhenArtifactPurgeFails(t *testing.T) {
	wallDeleteCalled := false
	replay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"s3 unavailable"}`))
	}))
	defer replay.Close()
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"run":{"status":"done"}}`))
		case http.MethodDelete:
			wallDeleteCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	ui := httptest.NewServer(handlerWithServices(wall.URL, replay.URL, ""))
	defer ui.Close()
	req, err := http.NewRequest(http.MethodDelete, ui.URL+"/v1/runs/run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("DELETE run = %d, want 502; body=%s", res.StatusCode, body)
	}
	if wallDeleteCalled {
		t.Fatal("wall delete was called after artifact purge failure")
	}
}

func TestDeleteActiveRunDoesNotTouchReplayStorage(t *testing.T) {
	replayCalled := false
	replay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replayCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer replay.Close()
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-1" {
			t.Fatalf("unexpected wall request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"run":{"status":"running"}}`))
	}))
	defer wall.Close()

	ui := httptest.NewServer(handlerWithServices(wall.URL, replay.URL, ""))
	defer ui.Close()
	req, err := http.NewRequest(http.MethodDelete, ui.URL+"/v1/runs/run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("DELETE active run = %d, want 409", res.StatusCode)
	}
	if replayCalled {
		t.Fatal("replay cleanup was called for active run")
	}
}
