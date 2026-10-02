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

func flagTestWall(t *testing.T) (*Wall, string) {
	t.Helper()
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	t.Cleanup(srv.Close)
	postJSON(t, srv.URL+"/v1/specs", spec("flag1"))
	postJSON(t, srv.URL+"/v1/lease", struct{}{})
	postJSON(t, srv.URL+"/v1/runs/flag1/heartbeat", farm.Heartbeat{RunID: "flag1"})
	return w, "flag1"
}

func postFlag(t *testing.T, w *Wall, id, note string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"note": note})
	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/runs/"+id+"/flag-stuck", bytes.NewReader(body)))
	return rec
}

func TestFlagStuckRequestsCancelAndRecordsAttempt(t *testing.T) {
	w, id := flagTestWall(t)
	if rec := postFlag(t, w, id, "looping in menu"); rec.Code != http.StatusOK {
		t.Fatalf("flag: %d %s", rec.Code, rec.Body)
	}
	w.mu.Lock()
	tile, cancel := w.tiles[id], w.cancel[id]
	w.mu.Unlock()
	if !cancel {
		t.Fatal("flag must ask the runner to stop through the cooperative cancel flag")
	}
	if tile.OperatorFlag != "looping in menu" || tile.OperatorFlagAttempt != 1 || tile.OperatorFlaggedAt == 0 {
		t.Fatalf("tile flag fields: %+v", tile)
	}
	if rec := postFlag(t, w, "nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown run: %d", rec.Code)
	}
}

func TestFlaggedCancelFinishBecomesStuck(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "")
	report := farm.FinishReport{RunID: id, Attempt: 1, Reason: "budget"}
	w.operatorFlagFinish(&report)
	if report.Reason != "stuck" || report.Detail != "operator flagged: no note" {
		t.Fatalf("rewrite: %q %q", report.Reason, report.Detail)
	}
	w.mu.Lock()
	w.settleRun(w.tiles[id], report.Reason, report.Detail, time.Now())
	w.mu.Unlock()
	w.mu.Lock()
	tile := w.tiles[id]
	w.mu.Unlock()
	if tile.Reason == "cancelled" {
		t.Fatal("a flagged run must not settle as a user cancel")
	}
}

func TestFlagDoesNotRewriteOtherOutcomes(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "x")
	for _, r := range []farm.FinishReport{
		{RunID: id, Attempt: 1, Reason: "done"},
		{RunID: id, Attempt: 1, Reason: "error", Detail: "boom"},
		{RunID: id, Attempt: 1, Reason: "budget", Detail: "frame budget exhausted"},
		{RunID: id, Attempt: 2, Reason: "budget"},
	} {
		before := r
		w.operatorFlagFinish(&r)
		if r.Reason != before.Reason || r.Detail != before.Detail {
			t.Fatalf("rewrote %+v into %+v", before, r)
		}
	}
}

func TestReaperSettlesFlaggedRunThatNeverStops(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "wedged")
	w.mu.Lock()
	w.tiles[id].OperatorFlaggedAt = time.Now().Add(-11 * time.Minute).Unix()
	w.tiles[id].lastUpdate = time.Now() // still heartbeating
	w.mu.Unlock()
	w.reapStale(time.Now())
	w.mu.Lock()
	tile := w.tiles[id]
	w.mu.Unlock()
	if tile.Attempts != 1 {
		t.Fatalf("attempt not settled: %+v", tile)
	}
	last := tile.Activity[len(tile.Activity)-1]
	if !strings.Contains(last.Detail, "runner did not stop within 10m") {
		t.Fatalf("activity detail: %+v", last)
	}
}
