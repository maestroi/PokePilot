package main

// pokeui is the browser-facing frontend: an allowlisted reverse proxy in
// front of the wall, plus an embedded operator console at GET /. These
// tests pin the four proxied routes, 404s for everything else (including
// runner-only /v1/lease), 502 when the wall is unreachable, and no-store
// on dashboard and frames.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPokeuiProxiesAllowlistedRoutes(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 1, 2, 3}
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/dashboard":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"now":1,"runs":[],"workers":[]}`))
		case req.Method == http.MethodGet && req.URL.Path == "/v1/triage":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`[{"pattern":"stuck","key":"abcdabcdabcdabcd","count":2}]`))
		case req.Method == http.MethodPost && req.URL.Path == "/v1/triage/abcdabcdabcdabcd/investigate":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"issue_number":42}`))
		case req.Method == http.MethodPost && req.URL.Path == "/v1/specs":
			body, _ := io.ReadAll(req.Body)
			if !bytes.Contains(body, []byte(`"run_id"`)) {
				res.WriteHeader(http.StatusBadRequest)
				return
			}
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"status":"queued"}`))
		case req.Method == http.MethodPost && req.URL.Path == "/v1/runs/r1/cancel":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"cancel":true}`))
		case req.Method == http.MethodDelete && req.URL.Path == "/v1/runs/r1":
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte(`{"deleted":true}`))
		case req.Method == http.MethodGet && req.URL.Path == "/frame" && req.URL.Query().Get("run") == "demo/1":
			res.Header().Set("Content-Type", "image/png")
			res.Write(png)
		case req.URL.Path == "/v1/lease":
			res.WriteHeader(http.StatusOK)
			res.Write([]byte(`should-not-be-reached`))
		default:
			res.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(wall.Close)

	ui := httptest.NewServer(handler(wall.URL))
	t.Cleanup(ui.Close)

	res, err := http.Get(ui.URL + "/v1/dashboard")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET /v1/dashboard = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("dashboard Content-Type = %q", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("dashboard Cache-Control = %q, want no-store", cc)
	}
	if !bytes.Contains(body, []byte(`"runs"`)) {
		t.Errorf("dashboard body = %s", body)
	}

	res, err = http.Get(ui.URL + "/v1/triage")
	if err != nil {
		t.Fatalf("GET triage: %v", err)
	}
	triage, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET /v1/triage = %d, want 200", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("triage Cache-Control = %q, want no-store", cc)
	}
	if !bytes.Contains(triage, []byte(`"pattern"`)) {
		t.Errorf("triage body = %s", triage)
	}

	res, err = http.Post(ui.URL+"/v1/triage/abcdabcdabcdabcd/investigate", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST investigate: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("POST investigate = %d, want 200", res.StatusCode)
	}

	res, err = http.Post(ui.URL+"/v1/triage/abcdabcdabcdabcd/other", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST other triage: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("POST /v1/triage/{key}/other = %d, want 404", res.StatusCode)
	}

	res, err = http.Get(ui.URL + "/api/issues/1")
	if err != nil {
		t.Fatalf("GET orchestrator path: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("GET /api/issues/1 = %d, want 404", res.StatusCode)
	}

	spec, _ := json.Marshal(map[string]string{"run_id": "r1"})
	res, err = http.Post(ui.URL+"/v1/specs", "application/json", bytes.NewReader(spec))
	if err != nil {
		t.Fatalf("POST specs: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck // test probe
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("POST /v1/specs = %d, want 200", res.StatusCode)
	}

	res, err = http.Post(ui.URL+"/v1/runs/r1/cancel", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck // test probe
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("POST cancel = %d, want 200", res.StatusCode)
	}

	req, err := http.NewRequest(http.MethodDelete, ui.URL+"/v1/runs/r1", nil)
	if err != nil {
		t.Fatalf("DELETE request: %v", err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE run: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck // test probe
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("DELETE /v1/runs/r1 = %d, want 200", res.StatusCode)
	}

	res, err = http.Get(ui.URL + "/frame?run=demo/1")
	if err != nil {
		t.Fatalf("GET frame: %v", err)
	}
	frame, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET /frame = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("frame Content-Type = %q", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("frame Cache-Control = %q, want no-store", cc)
	}
	if !bytes.Equal(frame, png) {
		t.Errorf("frame = %x, want %x", frame, png)
	}

	res, err = http.Post(ui.URL+"/v1/lease", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST lease: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck // test probe
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("POST /v1/lease = %d, want 404 (not allowlisted)", res.StatusCode)
	}
}

func TestPokeuiWallUnreachable(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)
	res, err := http.Get(ui.URL + "/v1/dashboard")
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("unreachable wall = %d, want 502", res.StatusCode)
	}
	if !bytes.Contains(body, []byte("wall unreachable")) {
		t.Errorf("502 body = %s, want wall unreachable", body)
	}
}

func TestPokeuiServesIndex(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)
	res, err := http.Get(ui.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET / = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	for _, want := range []string{"pokefarm", "Queue a run", "Play the game", `id="live"`, `id="workers"`, `id="history"`, `id="failures"`, "/ui.js"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("index missing %q", want)
		}
	}

	res, err = http.Get(ui.URL + "/ui.js")
	if err != nil {
		t.Fatalf("GET /ui.js: %v", err)
	}
	js, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET /ui.js = %d, want 200", res.StatusCode)
	}
	if !bytes.Contains(js, []byte("/v1/dashboard")) {
		t.Errorf("ui.js missing /v1/dashboard")
	}
	if !bytes.Contains(js, []byte("/v1/triage")) {
		t.Errorf("ui.js missing /v1/triage")
	}
}
