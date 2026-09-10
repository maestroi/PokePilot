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
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/runs/run-1" {
			t.Fatalf("unexpected wall request: %s %s", r.Method, r.URL.Path)
		}
		calls = append(calls, "wall")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deleted":true}`))
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
	if want := []string{"replay", "wall"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestDeleteRunRetainsWallHistoryWhenArtifactPurgeFails(t *testing.T) {
	wallCalled := false
	replay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"s3 unavailable"}`))
	}))
	defer replay.Close()
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wallCalled = true
		w.WriteHeader(http.StatusOK)
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
	if wallCalled {
		t.Fatal("wall delete was called after artifact purge failure")
	}
}
