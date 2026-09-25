package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSpectatorRenderStateProxy(t *testing.T) {
	const payload = `{"schema_version":1,"game":{"id":"pokemon-red","revision":"en-us-rev0"},"clock":{"frame":77},"scene":"overworld","capabilities":["map","player","layers"],"map":{"id":"route 1","name":"ROUTE_1","width":20,"height":36},"player":{"id":"player","kind":"player","position":{"x":10,"y":22},"facing":"up"},"layers":[]}`
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/render-state" || req.URL.Query().Get("run") != "live-red" {
			http.NotFound(res, req)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		io.WriteString(res, payload) //nolint:errcheck // test server
	}))
	t.Cleanup(wall.Close)

	ui := httptest.NewServer(spectatorHandler(wall.URL))
	t.Cleanup(ui.Close)

	resp, err := http.Get(ui.URL + "/render-state?run=live-red")
	if err != nil {
		t.Fatalf("GET render state: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET render state = %d, want 200: %s", resp.StatusCode, got)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if string(got) != payload {
		t.Fatalf("payload changed: %s", got)
	}
}

func TestSpectatorRenderStateRejectsInvalidProtocol(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		io.WriteString(res, `{"schema_version":99}`) //nolint:errcheck // test server
	}))
	t.Cleanup(wall.Close)

	ui := httptest.NewServer(spectatorHandler(wall.URL))
	t.Cleanup(ui.Close)

	resp, err := http.Get(ui.URL + "/render-state?run=live-red")
	if err != nil {
		t.Fatalf("GET render state: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("invalid protocol = %d, want 502: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "spectator feed unavailable") {
		t.Fatalf("unexpected error body: %s", body)
	}
}

func TestOperatorRenderStateUsesSharedValidatedProxy(t *testing.T) {
	const payload = `{"schema_version":1,"game":{"id":"pokemon-red","revision":"en-us-rev0"},"clock":{"frame":91},"scene":"overworld","capabilities":["map","player","layers"],"map":{"id":"viridian city","name":"VIRIDIAN_CITY","width":20,"height":18},"player":{"id":"player","kind":"player","position":{"x":8,"y":9},"facing":"left"},"layers":[]}`
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/render-state" || req.URL.Query().Get("run") != "operator-red" {
			http.NotFound(res, req)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		io.WriteString(res, payload) //nolint:errcheck // test server
	}))
	t.Cleanup(wall.Close)

	ui := httptest.NewServer(handlerWithServices(wall.URL, "", ""))
	t.Cleanup(ui.Close)

	resp, err := http.Get(ui.URL + "/render-state?run=operator-red")
	if err != nil {
		t.Fatalf("GET operator render state: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET operator render state = %d, want 200: %s", resp.StatusCode, got)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if string(got) != payload {
		t.Fatalf("payload changed: %s", got)
	}
}
