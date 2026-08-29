package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func postWorkerPing(t *testing.T, h http.Handler, addrs []string) {
	t.Helper()
	body, _ := json.Marshal(farm.WorkerPing{Addrs: addrs})
	req := httptest.NewRequest(http.MethodPost, "/v1/workers", bytes.NewReader(body))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/workers = %d, want 200: %s", res.Code, res.Body.String())
	}
}

func gridHTML(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", res.Code)
	}
	return res.Body.String()
}

func TestWallWorkerPresence(t *testing.T) {
	wall := NewWall(t.TempDir())
	h := wall.Handler()

	postWorkerPing(t, h, []string{"10.0.1.20:8099", "192.168.64.3:8099"})
	postWorkerPing(t, h, []string{"10.0.1.21:8099"})

	page := gridHTML(t, h)
	for _, want := range []string{"10.0.1.20:8099", "10.0.1.21:8099", "idle"} {
		if !strings.Contains(page, want) {
			t.Fatalf("grid missing %q:\n%s", want, page)
		}
	}

	// A heartbeat from worker 20 switches it to running <run>; worker 21
	// stays idle.
	specBody, _ := json.Marshal(farm.Spec{RunID: "worker-pres", Planner: "scripted", Starter: "squirtle", Dest: "viridian pokemon center"})
	req := httptest.NewRequest(http.MethodPost, "/v1/specs", bytes.NewReader(specBody))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/specs = %d", res.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/lease", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/lease = %d, want 200 (spec was queued)", res.Code)
	}
	hbBody, _ := json.Marshal(farm.Heartbeat{RunID: "worker-pres", Frame: 100, WorkerAddrs: []string{"10.0.1.20:8099"}})
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/worker-pres/heartbeat", bytes.NewReader(hbBody))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST heartbeat = %d", res.Code)
	}

	page = gridHTML(t, h)
	if !strings.Contains(page, "running worker-pres") {
		t.Fatalf("grid does not show the running worker:\n%s", page)
	}
	if !strings.Contains(page, "10.0.1.21:8099</td><td>idle") {
		t.Fatalf("grid lost the idle worker:\n%s", page)
	}
}

func TestWallWorkerPingRequiresAddrs(t *testing.T) {
	wall := NewWall(t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/v1/workers", bytes.NewReader([]byte(`{}`)))
	res := httptest.NewRecorder()
	wall.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("POST /v1/workers without addrs = %d, want 400", res.Code)
	}
}

func TestWallReapsStaleWorkers(t *testing.T) {
	wall := NewWall(t.TempDir())
	wall.workerExpiry = 50 * time.Millisecond
	h := wall.Handler()

	postWorkerPing(t, h, []string{"10.0.1.30:8099"})
	if page := gridHTML(t, h); !strings.Contains(page, "10.0.1.30:8099") {
		t.Fatalf("worker not shown before expiry:\n%s", page)
	}

	time.Sleep(80 * time.Millisecond)
	wall.reapStale(time.Now())
	if page := gridHTML(t, h); strings.Contains(page, "10.0.1.30:8099") {
		t.Fatalf("stale worker still shown after reap:\n%s", page)
	}
}
