package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func TestDurabilizeFinishReportKeepsSmallFinalFrameInline(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 1, 2, 3}
	durable, err := durabilizeFinishReport(farm.FinishReport{
		RunID:    "inline-frame",
		FramePNG: png,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer durable.cleanupUploads()

	for _, artifact := range durable.artifacts {
		if artifact.meta.Name != "final-frame.png" {
			continue
		}
		if !bytes.Equal(artifact.inline, png) {
			t.Fatalf("inline final frame = %x, want %x", artifact.inline, png)
		}
		if artifact.meta.Store != "" || artifact.meta.ObjectKey != "" {
			t.Fatalf("small final frame unexpectedly uses object storage: %+v", artifact.meta)
		}
		if len(artifact.meta.Data) != 0 {
			t.Fatal("final frame bytes leaked into artifact metadata")
		}
		return
	}
	t.Fatal("durable finish did not contain final-frame.png")
}

func TestCaptureFinishFrameFetchesAttachedRunner(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 10, 11, 12}
	runner := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/frame.png" {
			http.NotFound(res, req)
			return
		}
		res.Header().Set("Content-Type", "image/png")
		_, _ = res.Write(png)
	}))
	defer runner.Close()

	w := NewWall("")
	w.mu.Lock()
	w.tiles["finishing-run"] = &Tile{
		RunID: "finishing-run", Status: statusRunning,
		workerAddrs: []string{strings.TrimPrefix(runner.URL, "http://")},
	}
	w.order = append(w.order, "finishing-run")
	w.mu.Unlock()

	if got := w.captureFinishFrame("finishing-run"); !bytes.Equal(got, png) {
		t.Fatalf("captureFinishFrame = %x, want %x", got, png)
	}
}

func TestControlPlaneFrameServesHistoricalInlineThumbnail(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 4, 5, 6}
	w, cp := newFrameControlPlaneTestWall(t, "history-run", png)

	nextCalls := 0
	next := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		nextCalls++
		http.NotFound(res, req)
	})
	h := w.controlPlaneFrameHTTPHandler(next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/frame?run=history-run", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /frame = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if nextCalls != 0 {
		t.Fatalf("fallback handler called %d times, want 0", nextCalls)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), png) {
		t.Fatalf("frame = %x, want %x", rec.Body.Bytes(), png)
	}

	stored, ok, err := cp.storedFinalFrame("history-run")
	if err != nil || !ok || !bytes.Equal(stored, png) {
		t.Fatalf("storedFinalFrame = %x, %v, %v; want %x, true, nil", stored, ok, err, png)
	}
}

func TestControlPlaneFrameNeverOverlaysActiveRunWithHistoricalThumbnail(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 7, 8, 9}
	w, _ := newFrameControlPlaneTestWall(t, "requeued-run", png)
	w.mu.Lock()
	w.tiles["requeued-run"] = &Tile{RunID: "requeued-run", Status: statusRunning}
	w.order = append(w.order, "requeued-run")
	w.mu.Unlock()

	next := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(res, "live path")
	})
	rec := httptest.NewRecorder()
	w.controlPlaneFrameHTTPHandler(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/frame?run=requeued-run", nil))

	if rec.Code != http.StatusBadGateway || rec.Body.String() != "live path" {
		t.Fatalf("active run response = %d %q, want live-path 502", rec.Code, rec.Body.String())
	}
}

func newFrameControlPlaneTestWall(t *testing.T, runID string, png []byte) (*Wall, *controlPlane) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE runs (run_id TEXT PRIMARY KEY, status TEXT NOT NULL);
CREATE TABLE artifacts (
 run_id TEXT NOT NULL, attempt INTEGER NOT NULL, kind TEXT NOT NULL, name TEXT NOT NULL,
 metadata_json BLOB NOT NULL, inline_data BLOB
);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO runs(run_id,status) VALUES(?,?)`, runID, statusDone); err != nil {
		db.Close()
		t.Fatal(err)
	}
	sum := sha256.Sum256(png)
	meta := farm.Artifact{
		Name: "final-frame.png", MediaType: "image/png",
		SHA256: hex.EncodeToString(sum[:]), Size: int64(len(png)),
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO artifacts(run_id,attempt,kind,name,metadata_json,inline_data) VALUES(?,1,'finish','final-frame.png',?,?)`, runID, raw, png); err != nil {
		db.Close()
		t.Fatal(err)
	}

	w := NewWall("")
	cp := &controlPlane{db: db}
	wallControlPlanes.Store(w, cp)
	t.Cleanup(func() {
		wallControlPlanes.Delete(w)
		_ = db.Close()
	})
	return w, cp
}
