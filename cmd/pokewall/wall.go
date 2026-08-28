// Package main is the pokefarm wall: a small orchestrator that queues run
// specs, leases them to pokepilot runners, tracks their heartbeats, takes
// cooperative cancel requests, and keeps durable finish dumps. It speaks
// only the farm wire contract and the standard library — no emu, skill,
// agent, red, Docker, or Swarm here.
package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/maestroi/pokepilot/farm"
)

// Run status values, in lifecycle order.
const (
	statusQueued  = "queued"
	statusLeased  = "leased"
	statusRunning = "running"
	statusDone    = "done"
)

// Tile is one run's live state as the wall sees it.
type Tile struct {
	RunID     string
	Status    string
	Planner   string
	Starter   string
	Dest      string
	Seed      int64
	FPS       int
	MaxRounds int
	MaxFrames int
	Frame     uint64
	Map       uint8
	X         uint8
	Y         uint8
	Trace     string
	StopSoFar string
	Reason    string
	Detail    string
	Finished  bool
}

// tileRow is a plain-value snapshot of a Tile, taken under w.mu so the
// grid template never reads live tiles after unlock. Rendering []*Tile
// after unlock is what raced with heartbeat/cancel/finish.
type tileRow struct {
	RunID     string
	Status    string
	Planner   string
	Starter   string
	Dest      string
	Seed      int64
	FPS       int
	MaxRounds int
	MaxFrames int
	Frame     uint64
	Map       uint8
	X         uint8
	Y         uint8
	Trace     string
	StopSoFar string
	Reason    string
	Detail    string
}

// Wall owns the spec queue, the tile map, cancel flags, and the optional
// dump directory. Every state change happens under w.mu; filesystem I/O
// for dumps happens outside it.
type Wall struct {
	mu       sync.Mutex
	order    []string         // run IDs in insertion order, for the grid
	queue    []string         // queued run IDs, oldest first
	tiles    map[string]*Tile // every known run
	cancel   map[string]bool  // cooperative cancel flags
	dumpsDir string           // "" disables durable dumps
}

// NewWall builds a Wall. If dumpsDir is non-empty, finish reports are also
// written there as JSON.
func NewWall(dumpsDir string) *Wall {
	return &Wall{
		tiles:    map[string]*Tile{},
		cancel:   map[string]bool{},
		dumpsDir: dumpsDir,
	}
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// safeDumpName derives a filesystem-safe single filename from a run ID.
// Every path separator and every other odd byte becomes '_', so the result
// can never escape the dump directory — ".." itself is replaced outright.
func safeDumpName(runID string) string {
	name := unsafeName.ReplaceAllString(runID, "_")
	if name == "" || name == "." || name == ".." {
		name = "run"
	}
	return name + ".json"
}

// Handler wires the wall's endpoints.
func (w *Wall) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/specs", w.handleSpecs)
	mux.HandleFunc("POST /v1/lease", w.handleLease)
	mux.HandleFunc("POST /v1/runs/{id}/heartbeat", w.handleHeartbeat)
	mux.HandleFunc("POST /v1/runs/{id}/cancel", w.handleCancel)
	mux.HandleFunc("POST /v1/runs/{id}/finish", w.handleFinish)
	mux.HandleFunc("GET /", w.handleGrid)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

// handleSpecs enqueues a spec. A missing run_id is 400; a run ID that is
// already queued, leased, or running is 409. Finishing the same ID again
// after it completed re-queues it fresh.
func (w *Wall) handleSpecs(res http.ResponseWriter, req *http.Request) {
	var spec farm.Spec
	if err := json.NewDecoder(req.Body).Decode(&spec); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad spec: " + err.Error()})
		return
	}
	if spec.RunID == "" {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}

	w.mu.Lock()
	if tile, ok := w.tiles[spec.RunID]; ok && !tile.Finished {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already active: " + spec.RunID})
		return
	}
	if _, ok := w.tiles[spec.RunID]; !ok {
		w.order = append(w.order, spec.RunID)
		w.tiles[spec.RunID] = &Tile{}
	}
	w.queue = append(w.queue, spec.RunID)
	w.applySpec(spec.RunID, spec)
	delete(w.cancel, spec.RunID)
	w.mu.Unlock()
	writeJSON(res, http.StatusOK, map[string]string{"status": statusQueued})
}

func (w *Wall) applySpec(runID string, spec farm.Spec) {
	t := w.tiles[runID]
	t.RunID = spec.RunID
	t.Status = statusQueued
	t.Planner = spec.Planner
	t.Starter = spec.Starter
	t.Dest = spec.Dest
	t.Seed = spec.Seed
	t.FPS = spec.FPS
	t.MaxRounds = spec.MaxRounds
	t.MaxFrames = spec.MaxFrames
	t.Finished = false
}

// handleLease hands out the oldest queued spec exactly once; 204 when the
// queue is empty.
func (w *Wall) handleLease(res http.ResponseWriter, req *http.Request) {
	w.mu.Lock()
	if len(w.queue) == 0 {
		w.mu.Unlock()
		res.WriteHeader(http.StatusNoContent)
		return
	}
	runID := w.queue[0]
	w.queue = w.queue[1:]
	t := w.tiles[runID]
	t.Status = statusLeased
	spec := farm.Spec{
		RunID:     t.RunID,
		Seed:      t.Seed,
		Planner:   t.Planner,
		Starter:   t.Starter,
		Dest:      t.Dest,
		FPS:       t.FPS,
		MaxRounds: t.MaxRounds,
		MaxFrames: t.MaxFrames,
	}
	w.mu.Unlock()
	writeJSON(res, http.StatusOK, spec)
}

// handleHeartbeat records live state for a run and answers with its
// current cancel flag. The URL path ID and the body run_id must agree.
func (w *Wall) handleHeartbeat(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var hb farm.Heartbeat
	if err := json.NewDecoder(req.Body).Decode(&hb); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad heartbeat: " + err.Error()})
		return
	}
	if hb.RunID != id {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "run_id mismatch: path " + id + " body " + hb.RunID})
		return
	}

	w.mu.Lock()
	t, ok := w.tiles[id]
	if !ok {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
		return
	}
	if t.Finished {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already finished: " + id})
		return
	}
	t.Status = statusRunning
	t.Frame = hb.Frame
	t.Map = hb.Map
	t.X = hb.X
	t.Y = hb.Y
	t.Trace = hb.Trace
	t.StopSoFar = hb.StopSoFar
	cancel := w.cancel[id]
	w.mu.Unlock()
	writeJSON(res, http.StatusOK, farm.HeartbeatReply{Cancel: cancel})
}

// handleCancel marks an active run for cooperative cancellation. Unknown
// IDs are 404; a run that already finished is 409.
func (w *Wall) handleCancel(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	w.mu.Lock()
	t, ok := w.tiles[id]
	if !ok {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
		return
	}
	if t.Finished {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already finished: " + id})
		return
	}
	w.cancel[id] = true
	w.mu.Unlock()
	writeJSON(res, http.StatusOK, map[string]bool{"cancel": true})
}

// handleFinish records why a run ended. An identical repeat is idempotent;
// a conflicting repeat is 409. When a dump directory is configured the
// report is written (or rewritten, on an idempotent retry) there BEFORE a
// 200 goes out — and the write happens outside w.mu.
func (w *Wall) handleFinish(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	var report farm.FinishReport
	if err := json.NewDecoder(req.Body).Decode(&report); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad finish report: " + err.Error()})
		return
	}
	if report.RunID != id {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "run_id mismatch: path " + id + " body " + report.RunID})
		return
	}

	w.mu.Lock()
	t, ok := w.tiles[id]
	if !ok {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
		return
	}
	if t.Finished {
		if t.Reason != report.Reason || t.Detail != report.Detail {
			w.mu.Unlock()
			writeJSON(res, http.StatusConflict, map[string]string{"error": "conflicting finish for " + id})
			return
		}
		// Identical duplicate: idempotent, but the dump below is still
		// rewritten so a previously failed write self-heals.
	} else {
		t.Status = statusDone
		t.Reason = report.Reason
		t.Detail = report.Detail
		t.Finished = true
		delete(w.cancel, id)
	}
	w.mu.Unlock()

	if w.dumpsDir != "" {
		data, err := json.Marshal(report)
		if err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "encode dump: " + err.Error()})
			return
		}
		if err := os.WriteFile(filepath.Join(w.dumpsDir, safeDumpName(report.RunID)), data, 0o644); err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "write dump: " + err.Error()})
			return
		}
	}
	writeJSON(res, http.StatusOK, map[string]string{"status": statusDone})
}

var gridTmpl = template.Must(template.New("grid").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>pokefarm wall</title></head>
<body>
<h1>pokefarm wall</h1>
<table border="1" cellspacing="0">
<tr><th>run</th><th>status</th><th>planner</th><th>starter</th><th>dest</th><th>seed</th><th>frame</th><th>map</th><th>x,y</th><th>trace</th><th>stop so far</th><th>reason</th><th>detail</th></tr>
{{range .}}<tr><td>{{.RunID}}</td><td>{{.Status}}</td><td>{{.Planner}}</td><td>{{.Starter}}</td><td>{{.Dest}}</td><td>{{.Seed}}</td><td>{{.Frame}}</td><td>{{printf "0x%02x" .Map}}</td><td>{{.X}},{{.Y}}</td><td>{{.Trace}}</td><td>{{.StopSoFar}}</td><td>{{.Reason}}</td><td>{{.Detail}}</td></tr>
{{else}}<tr><td colspan="13">no runs</td></tr>
{{end}}</table>
</body></html>`))

// handleGrid renders the known tiles. It snapshots plain row values under
// w.mu and only after unlocking does the template read those copies, so a
// concurrent heartbeat/cancel/finish cannot race with the render.
func (w *Wall) handleGrid(res http.ResponseWriter, req *http.Request) {
	w.mu.Lock()
	rows := make([]tileRow, 0, len(w.order))
	for _, id := range w.order {
		t := w.tiles[id]
		rows = append(rows, tileRow{
			RunID:     t.RunID,
			Status:    t.Status,
			Planner:   t.Planner,
			Starter:   t.Starter,
			Dest:      t.Dest,
			Seed:      t.Seed,
			FPS:       t.FPS,
			MaxRounds: t.MaxRounds,
			MaxFrames: t.MaxFrames,
			Frame:     t.Frame,
			Map:       t.Map,
			X:         t.X,
			Y:         t.Y,
			Trace:     t.Trace,
			StopSoFar: t.StopSoFar,
			Reason:    t.Reason,
			Detail:    t.Detail,
		})
	}
	w.mu.Unlock()

	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := gridTmpl.Execute(res, rows); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
	}
}
