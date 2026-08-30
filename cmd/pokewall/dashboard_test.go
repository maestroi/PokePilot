package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

type dashboardJSON struct {
	Now  int64 `json:"now"`
	Runs []struct {
		RunID      string `json:"run_id"`
		Status     string `json:"status"`
		Planner    string `json:"planner"`
		Starter    string `json:"starter"`
		Dest       string `json:"dest"`
		Goal       string `json:"goal"`
		Seed       int64  `json:"seed"`
		FPS        int    `json:"fps"`
		MaxRounds  int    `json:"max_rounds"`
		MaxFrames  int    `json:"max_frames"`
		Endless    bool   `json:"endless"`
		RandomSeed bool   `json:"random_seed"`
		QueuedAt   int64  `json:"queued_at"`
		EndedAt    int64  `json:"ended_at"`
		Attempts   int    `json:"attempts"`
		Frame      uint64 `json:"frame"`
		Map        uint8  `json:"map"`
		X          uint8  `json:"x"`
		Y          uint8  `json:"y"`
		Trace      string `json:"trace"`
		Question   string `json:"question"`
		Decision   string `json:"decision"`
		StopSoFar  string `json:"stop_so_far"`
		Reason     string `json:"reason"`
		Detail     string `json:"detail"`
	} `json:"runs"`
	Workers []struct {
		Addr    string `json:"addr"`
		RunID   string `json:"run_id"`
		SeenAgo string `json:"seen_ago"`
	} `json:"workers"`
}

func getDashboard(t *testing.T, h http.Handler) dashboardJSON {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/dashboard = %d, want 200: %s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got dashboardJSON
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode dashboard: %v\n%s", err, res.Body.String())
	}
	if got.Now == 0 {
		t.Error("now is 0, want a unix timestamp")
	}
	return got
}

func TestDashboardJSON(t *testing.T) {
	wall := NewWall("")
	h := wall.Handler()

	got := getDashboard(t, h)
	if len(got.Runs) != 0 || len(got.Workers) != 0 {
		t.Fatalf("empty wall: runs=%d workers=%d, want 0/0", len(got.Runs), len(got.Workers))
	}

	specBody, _ := json.Marshal(farm.Spec{
		RunID: "dash-1", Seed: 42, Planner: "scripted", Starter: "charmander",
		Dest: "pallet", Goal: "Earn the Boulder Badge.", FPS: 60, MaxRounds: 3, MaxFrames: 1000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/specs", bytes.NewReader(specBody))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/specs = %d", res.Code)
	}

	got = getDashboard(t, h)
	if len(got.Runs) != 1 {
		t.Fatalf("queued runs = %d, want 1", len(got.Runs))
	}
	r := got.Runs[0]
	if r.RunID != "dash-1" || r.Status != "queued" || r.Planner != "scripted" || r.Starter != "charmander" || r.Dest != "pallet" || r.Goal != "Earn the Boulder Badge." || r.Seed != 42 || r.FPS != 60 || r.MaxRounds != 3 || r.MaxFrames != 1000 {
		t.Fatalf("queued run = %+v", r)
	}
	if r.QueuedAt == 0 {
		t.Fatal("queued run has no queued_at")
	}
	if r.EndedAt != 0 {
		t.Fatalf("queued run ended_at = %d, want 0", r.EndedAt)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/lease", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/lease = %d", res.Code)
	}
	hbBody, _ := json.Marshal(farm.Heartbeat{
		RunID: "dash-1", Frame: 99, Map: 0x0c, X: 5, Y: 6,
		Trace: "stepped north", StopSoFar: "ok",
		WorkerAddrs: []string{"10.0.1.9:8099"},
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/dash-1/heartbeat", bytes.NewReader(hbBody))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("heartbeat = %d", res.Code)
	}

	got = getDashboard(t, h)
	r = got.Runs[0]
	if r.Status != "running" || r.Frame != 99 || r.Map != 0x0c || r.X != 5 || r.Y != 6 || r.Trace != "stepped north" || r.StopSoFar != "ok" {
		t.Fatalf("running run = %+v", r)
	}
	if len(got.Workers) != 1 || got.Workers[0].Addr != "10.0.1.9:8099" || got.Workers[0].RunID != "dash-1" {
		t.Fatalf("workers = %+v", got.Workers)
	}

	finBody, _ := json.Marshal(farm.FinishReport{RunID: "dash-1", Reason: "done", Detail: "arrived"})
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/dash-1/finish", bytes.NewReader(finBody))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("finish = %d", res.Code)
	}
	got = getDashboard(t, h)
	r = got.Runs[0]
	if r.Status != "done" || r.Reason != "done" || r.Detail != "arrived" {
		t.Fatalf("finished run = %+v", r)
	}
	if r.EndedAt == 0 {
		t.Fatal("finished run has no ended_at")
	}
	if r.Trace != "stepped north" {
		t.Errorf("finished run dropped last trace: %+v", r)
	}
}

// TestDashboardShowsLatestPlan is why the watch pane exists for an llm
// run: the last emulator trace line is "control regained", which says
// nothing about what the model was asked or what it chose. Those two
// strings ride the heartbeat and stay on the tile after finish.
func TestDashboardShowsLatestPlan(t *testing.T) {
	wall := NewWall("")
	h := wall.Handler()

	specBody, _ := json.Marshal(farm.Spec{
		RunID: "plan-1", Planner: "llm", Starter: "squirtle",
		Goal: "Earn the Boulder Badge.", Seed: 0,
	})
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
		t.Fatalf("POST /v1/lease = %d", res.Code)
	}

	question := "1: go to pallet town\n2: talk at (5,3)"
	decision := "go to pallet town"
	hbBody, _ := json.Marshal(farm.Heartbeat{
		RunID: "plan-1", Frame: 100, Map: 0x28, X: 5, Y: 6,
		Trace: "control: control regained", Question: question, Decision: decision,
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/plan-1/heartbeat", bytes.NewReader(hbBody))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("heartbeat = %d", res.Code)
	}

	got := getDashboard(t, h)
	if len(got.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(got.Runs))
	}
	r := got.Runs[0]
	if r.Question != question {
		t.Errorf("question = %q, want %q", r.Question, question)
	}
	if r.Decision != decision {
		t.Errorf("decision = %q, want %q", r.Decision, decision)
	}

	finBody, _ := json.Marshal(farm.FinishReport{RunID: "plan-1", Reason: "done"})
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/plan-1/finish", bytes.NewReader(finBody))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("finish = %d", res.Code)
	}
	got = getDashboard(t, h)
	r = got.Runs[0]
	if r.Question != question || r.Decision != decision {
		t.Errorf("finished run dropped the plan: question=%q decision=%q", r.Question, r.Decision)
	}
}

// TestDashboardNewestRunFirst is the operator's reading order: a newly
// queued run must land at the top of the list, not after everything
// already there.
func TestDashboardNewestRunFirst(t *testing.T) {
	wall := NewWall("")
	h := wall.Handler()
	for _, id := range []string{"old", "new"} {
		body, _ := json.Marshal(farm.Spec{RunID: id, Planner: "scripted", Starter: "squirtle", Dest: "pallet"})
		req := httptest.NewRequest(http.MethodPost, "/v1/specs", bytes.NewReader(body))
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("POST /v1/specs %s = %d", id, res.Code)
		}
	}
	got := getDashboard(t, h)
	if len(got.Runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(got.Runs))
	}
	if got.Runs[0].RunID != "new" || got.Runs[1].RunID != "old" {
		t.Fatalf("order = %s, %s; want new, old", got.Runs[0].RunID, got.Runs[1].RunID)
	}
}

// TestDeleteFinishedRun removes a history row. An active run stays put
// (cancel it instead); an unknown id is 404.
func TestDeleteFinishedRun(t *testing.T) {
	wall := NewWall("")
	h := wall.Handler()

	specBody, _ := json.Marshal(farm.Spec{RunID: "hist-1", Planner: "scripted", Starter: "squirtle", Dest: "pallet"})
	req := httptest.NewRequest(http.MethodPost, "/v1/specs", bytes.NewReader(specBody))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/specs = %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/runs/hist-1", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("DELETE active = %d, want 409", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/lease", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	finBody, _ := json.Marshal(farm.FinishReport{RunID: "hist-1", Reason: "done"})
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/hist-1/finish", bytes.NewReader(finBody))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("finish = %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/runs/hist-1", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("DELETE finished = %d, want 200: %s", res.Code, res.Body.String())
	}
	got := getDashboard(t, h)
	if len(got.Runs) != 0 {
		t.Fatalf("after delete runs = %d, want 0: %+v", len(got.Runs), got.Runs)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/runs/hist-1", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("DELETE unknown = %d, want 404", res.Code)
	}
}
