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
	for _, viaPlane := range []bool{false, true} {
		name := "handleFinish"
		if viaPlane {
			name = "controlPlaneWrapper"
		}
		t.Run(name, func(t *testing.T) {
			w := NewWall("")
			if viaPlane {
				wallControlPlanes.Store(w, &controlPlane{})
				t.Cleanup(func() { wallControlPlanes.Delete(w) })
			}
			h := w.Handler()
			sp := spec("flag-fin")
			sp.RecoveryProfile = farm.RecoveryProfileResilient
			serveWallJSON(t, h, http.MethodPost, "/v1/specs", sp)
			serveWallJSON(t, h, http.MethodPost, "/v1/lease", struct{}{})
			if rec := postFlag(t, w, "flag-fin", "looping"); rec.Code != http.StatusOK {
				t.Fatalf("flag: %d %s", rec.Code, rec.Body)
			}
			// The LLM path reports a cooperative cancel as budget, no detail.
			rep := farm.FinishReport{RunID: "flag-fin", Attempt: 1, Reason: "budget"}
			if res := serveWallJSON(t, h, http.MethodPost, "/v1/runs/flag-fin/finish", rep); res.Code != http.StatusOK {
				t.Fatalf("finish: %d %s", res.Code, res.Body)
			}
			w.mu.Lock()
			tile := w.tiles["flag-fin"]
			reason, detail, finished, status, attempts := tile.Reason, tile.Detail, tile.Finished, tile.Status, tile.Attempts
			w.mu.Unlock()
			// A recovered attempt clears Reason and records the settled detail.
			if reason == "cancelled" || detail != "attempt 1 failed: operator flagged: looping" {
				t.Fatalf("outcome %q %q", reason, detail)
			}
			if finished || status != statusQueued || attempts != 1 {
				t.Fatalf("flagged stuck must be recovered, not cancelled: finished=%v status=%q attempts=%d", finished, status, attempts)
			}
		})
	}
}

func TestFlagRowFieldsVisibleOverHTTP(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "visible")
	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil))
	var view dashboardView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for _, row := range view.Runs {
		if row.RunID == id {
			if row.OperatorFlag != "visible" || row.OperatorFlagAttempt != 1 || row.OperatorFlaggedAt == 0 {
				t.Fatalf("dashboard row: %+v", row)
			}
			return
		}
	}
	t.Fatal("run missing from dashboard")
}

func TestFlagFinishedRunConflicts(t *testing.T) {
	w, id := flagTestWall(t)
	w.mu.Lock()
	w.tiles[id].Finished = true
	w.mu.Unlock()
	if rec := postFlag(t, w, id, "late"); rec.Code != http.StatusConflict {
		t.Fatalf("finished run: %d", rec.Code)
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

func TestRequeueSameIDClearsOperatorFlag(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "old flag")
	h := w.Handler()
	w.mu.Lock()
	w.tiles[id].Finished = true
	w.tiles[id].Attempts = 1
	w.mu.Unlock()
	serveWallJSON(t, h, http.MethodPost, "/v1/specs", spec(id))
	serveWallJSON(t, h, http.MethodPost, "/v1/lease", struct{}{})
	w.mu.Lock()
	tile := w.tiles[id]
	flag, attempt, at := tile.OperatorFlag, tile.OperatorFlagAttempt, tile.OperatorFlaggedAt
	w.mu.Unlock()
	if flag != "" || attempt != 0 || at != 0 {
		t.Fatalf("re-queue kept flag: %q %d %d", flag, attempt, at)
	}
	rep := farm.FinishReport{RunID: id, Attempt: 1, Reason: "cancelled"}
	w.operatorFlagFinish(&rep)
	if rep.Reason != "cancelled" {
		t.Fatalf("fresh attempt rewritten: %+v", rep)
	}
	w.mu.Lock()
	w.tiles[id].OperatorFlaggedAt = time.Now().Add(-time.Hour).Unix()
	settled := w.reapFlaggedLocked(w.tiles[id], time.Now())
	w.mu.Unlock()
	if settled {
		t.Fatal("reaper settled a re-queued run on a stale flag")
	}
}

func TestFlagQueuedRunConflicts(t *testing.T) {
	w := NewWall("")
	serveWallJSON(t, w.Handler(), http.MethodPost, "/v1/specs", spec("q1"))
	rec := postFlag(t, w, "q1", "x")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "run is queued; nothing to stop") {
		t.Fatalf("queued run: %d %s", rec.Code, rec.Body)
	}
}

func TestCancelAfterFlagStaysUserCancel(t *testing.T) {
	w, id := flagTestWall(t)
	postFlag(t, w, id, "x")
	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/runs/"+id+"/cancel", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	w.mu.Lock()
	tile := w.tiles[id]
	flag := tile.OperatorFlag
	w.mu.Unlock()
	if flag != "" {
		t.Fatalf("cancel kept flag %q", flag)
	}
	rep := farm.FinishReport{RunID: id, Attempt: 1, Reason: "cancelled"}
	w.operatorFlagFinish(&rep)
	if rep.Reason != "cancelled" {
		t.Fatalf("user cancel rewritten: %+v", rep)
	}
}
