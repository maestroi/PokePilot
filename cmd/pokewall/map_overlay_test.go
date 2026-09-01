package main

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestWallCarriesMapOverlayButDoesNotPersistIt(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "wall-state.json")
	w := NewWall("")
	w.SetStatePath(stateFile)
	srv := newWallTestServer(t, w)

	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "map-run", Planner: "scripted", Starter: "squirtle", Dest: "pallet"})
	client := farm.NewClient(srv.URL)
	if spec, err := client.Lease(context.Background()); err != nil || spec == nil {
		t.Fatalf("lease = %#v, %v", spec, err)
	}
	hb := farm.Heartbeat{
		RunID: "map-run", Map: 0x0d, X: 7, Y: 31,
		Sprites: []farm.MapSprite{{X: 8, Y: 31, PictureID: 0x22, Slot: 3}},
		Trail:   [][2]uint8{{5, 31}, {6, 31}, {7, 31}},
	}
	if _, err := client.Heartbeat(context.Background(), hb); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	dash := doGet(t, srv.URL+"/v1/dashboard")
	for _, want := range []string{`"sprites"`, `"picture_id":34`, `"slot":3`, `"trail"`, `[5,31]`, `[7,31]`} {
		if !strings.Contains(dash, want) {
			t.Errorf("dashboard missing %s:\n%s", want, dash)
		}
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if strings.Contains(string(data), `"sprites"`) || strings.Contains(string(data), `"trail"`) {
		t.Fatalf("live map overlay leaked into persisted state:\n%s", data)
	}
}

func newWallTestServer(t *testing.T, w *Wall) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)
	return srv
}
