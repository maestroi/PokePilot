package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

func serveWallJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func TestControlPlaneFinishIgnoresUnavailableLocalCache(t *testing.T) {
	// The parent of this path deliberately does not exist, so the optional
	// os.WriteFile cache write fails. A production settlement must still be
	// successful so controlPlaneHTTPHandler can commit PostgreSQL afterwards.
	cache := filepath.Join(t.TempDir(), "missing", "finish-cache")
	w := NewWall(cache)
	wallControlPlanes.Store(w, &controlPlane{})
	t.Cleanup(func() { wallControlPlanes.Delete(w) })

	h := w.Handler()
	if res := serveWallJSON(t, h, http.MethodPost, "/v1/specs", spec("db-settlement")); res.Code != http.StatusOK {
		t.Fatalf("enqueue = %d: %s", res.Code, res.Body.String())
	}
	finish := farm.FinishReport{RunID: "db-settlement", Attempt: 1, Reason: "done"}
	if res := serveWallJSON(t, h, http.MethodPost, "/v1/runs/db-settlement/finish", finish); res.Code != http.StatusOK {
		t.Fatalf("finish = %d: %s", res.Code, res.Body.String())
	}
}

func TestControlPlaneFinishDoesNotSeedLegacyFileOutbox(t *testing.T) {
	w := NewWall(t.TempDir())
	wallControlPlanes.Store(w, &controlPlane{})
	t.Cleanup(func() { wallControlPlanes.Delete(w) })
	// Direct assignment avoids starting the legacy dispatcher; only the
	// presence check used by enqueueIssueAfterDump matters for this regression.
	w.issues = &issueClient{}

	h := w.Handler()
	if res := serveWallJSON(t, h, http.MethodPost, "/v1/specs", spec("db-failure")); res.Code != http.StatusOK {
		t.Fatalf("enqueue = %d: %s", res.Code, res.Body.String())
	}
	finish := farm.FinishReport{RunID: "db-failure", Attempt: 1, Reason: "error", Detail: "typed failure"}
	if res := serveWallJSON(t, h, http.MethodPost, "/v1/runs/db-failure/finish", finish); res.Code != http.StatusOK {
		t.Fatalf("finish = %d: %s", res.Code, res.Body.String())
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.outbox) != 0 {
		t.Fatalf("legacy file-backed outbox seeded in control-plane mode: %#v", w.outbox)
	}
}
