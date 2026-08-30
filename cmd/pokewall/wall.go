// Package main is the pokefarm wall: a small orchestrator that queues run
// specs, leases them to pokepilot runners, tracks their heartbeats, takes
// cooperative cancel requests, and keeps durable finish dumps. It speaks
// only the farm wire contract and the standard library — no emu, skill,
// agent, red, Docker, or Swarm here.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

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
	// Attempts counts completed attempts; a retried run keeps its history
	// so the grid can show where it is in its retry budget.
	Attempts  int
	Frame     uint64
	Map       uint8
	X         uint8
	Y         uint8
	Trace     string
	StopSoFar string
	Reason    string
	Detail    string
	Finished  bool
	// workerAddrs is where this run's runner watch server is reachable,
	// last reported by its heartbeats. Unexported: it is proxy input, not
	// grid data.
	workerAddrs []string
	// lastUpdate is when this tile last changed (enqueue, lease, or
	// heartbeat). The reaper uses it to spot runs whose runner went quiet.
	// It is deliberately NOT persisted: every restored tile gets a fresh
	// grace period on load, which is exactly right after a wall outage —
	// live runners refresh it within one heartbeat, dead ones age out.
	lastUpdate time.Time
	// lastFrame is the most recent live screen we successfully proxied
	// (or grabbed on finish). Not persisted: a wall restart leaves
	// history cards without a picture until a new run writes one.
	lastFrame []byte
}

// tileRow is a plain-value snapshot of a Tile, taken under w.mu so the
// grid template never reads live tiles after unlock. Rendering []*Tile
// after unlock is what raced with heartbeat/cancel/finish.
type tileRow struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	Planner   string `json:"planner"`
	Starter   string `json:"starter"`
	Dest      string `json:"dest"`
	Seed      int64  `json:"seed"`
	FPS       int    `json:"fps"`
	MaxRounds int    `json:"max_rounds"`
	MaxFrames int    `json:"max_frames"`
	Frame     uint64 `json:"frame"`
	Map       uint8  `json:"map"`
	X         uint8  `json:"x"`
	Y         uint8  `json:"y"`
	Trace     string `json:"trace"`
	StopSoFar string `json:"stop_so_far"`
	Attempts  int    `json:"attempts"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail"`
}

// Wall owns the spec queue, the tile map, cancel flags, the optional dump
// directory, and the optional state file. Every state change happens under
// w.mu; filesystem I/O for dumps and state happens outside it.
type Wall struct {
	mu           sync.Mutex
	order        []string               // run IDs in insertion order, for the grid
	queue        []string               // queued run IDs, oldest first
	tiles        map[string]*Tile       // every known run
	cancel       map[string]bool        // cooperative cancel flags
	dumpsDir     string                 // "" disables durable dumps
	statePath    string                 // "" disables tile/queue persistence
	workers      map[string]*workerInfo // runner presence, keyed by first reported addr
	workerExpiry time.Duration          // how long a worker may go unseen before the reaper drops it
	staleAfter   time.Duration          // how long a run may go quiet before the reaper declares it lost
}

// NewWall builds a Wall. If dumpsDir is non-empty, finish reports are also
// written there as JSON.
func NewWall(dumpsDir string) *Wall {
	return &Wall{
		tiles:        map[string]*Tile{},
		cancel:       map[string]bool{},
		dumpsDir:     dumpsDir,
		workers:      map[string]*workerInfo{},
		workerExpiry: 15 * time.Second,
		staleAfter:   defaultStaleExpiry,
	}
}

// SetStatePath enables persistence of the tile map and queue to path, and
// loads any previously saved state. A missing or corrupt file is not an
// error: the wall simply starts empty.
func (w *Wall) SetStatePath(path string) {
	w.statePath = path
	w.loadState()
}

// persistedTile is the JSON shape of one Tile. workerAddrs is persisted so
// a restarted wall can resume proxying frames without waiting for the next
// heartbeat; lastUpdate is not (see Tile.lastUpdate).
type persistedTile struct {
	RunID       string   `json:"run_id"`
	Status      string   `json:"status"`
	Planner     string   `json:"planner,omitempty"`
	Starter     string   `json:"starter,omitempty"`
	Dest        string   `json:"dest,omitempty"`
	Seed        int64    `json:"seed"`
	FPS         int      `json:"fps"`
	MaxRounds   int      `json:"max_rounds"`
	MaxFrames   int      `json:"max_frames"`
	Attempts    int      `json:"attempts"`
	Frame       uint64   `json:"frame"`
	Map         uint8    `json:"map"`
	X           uint8    `json:"x"`
	Y           uint8    `json:"y"`
	Trace       string   `json:"trace,omitempty"`
	StopSoFar   string   `json:"stop_so_far,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Detail      string   `json:"detail,omitempty"`
	Finished    bool     `json:"finished"`
	WorkerAddrs []string `json:"worker_addrs,omitempty"`
}

// persistedState is the wall's whole on-disk memory: run order, tiles, and
// the queue of not-yet-leased runs.
type persistedState struct {
	Order []string                 `json:"order"`
	Queue []string                 `json:"queue"`
	Tiles map[string]persistedTile `json:"tiles"`
}

// marshalStateLocked encodes the wall's memory. Caller holds w.mu.
func (w *Wall) marshalStateLocked() ([]byte, error) {
	ps := persistedState{
		Order: append([]string(nil), w.order...),
		Queue: append([]string(nil), w.queue...),
		Tiles: make(map[string]persistedTile, len(w.tiles)),
	}
	for id, t := range w.tiles {
		ps.Tiles[id] = persistedTile{
			RunID:       t.RunID,
			Status:      t.Status,
			Planner:     t.Planner,
			Starter:     t.Starter,
			Dest:        t.Dest,
			Seed:        t.Seed,
			FPS:         t.FPS,
			MaxRounds:   t.MaxRounds,
			MaxFrames:   t.MaxFrames,
			Attempts:    t.Attempts,
			Frame:       t.Frame,
			Map:         t.Map,
			X:           t.X,
			Y:           t.Y,
			Trace:       t.Trace,
			StopSoFar:   t.StopSoFar,
			Reason:      t.Reason,
			Detail:      t.Detail,
			Finished:    t.Finished,
			WorkerAddrs: append([]string(nil), t.workerAddrs...),
		}
	}
	return json.Marshal(ps)
}

// saveState writes the tile map and queue to the state file. It marshals
// under w.mu and writes outside it, so a slow disk cannot stall the wall;
// a failed write is logged, not fatal — the in-memory state is
// authoritative and the next mutation retries.
func (w *Wall) saveState() {
	if w.statePath == "" {
		return
	}
	w.mu.Lock()
	data, err := w.marshalStateLocked()
	w.mu.Unlock()
	if err != nil {
		log.Printf("pokewall: encode state: %v", err)
		return
	}
	if err := writeAtomic(w.statePath, data, 0o644); err != nil {
		log.Printf("pokewall: write state: %v", err)
	}
}

// loadState restores the tile map and queue from the state file. Every
// restored tile gets lastUpdate = now: after a wall outage, live runners
// refresh it within one heartbeat, and dead ones age out on the reaper's
// clock instead of being blamed for the downtime.
func (w *Wall) loadState() {
	if w.statePath == "" {
		return
	}
	data, err := os.ReadFile(w.statePath)
	if err != nil {
		return // first start or unreadable file: begin empty
	}
	var ps persistedState
	if err := json.Unmarshal(data, &ps); err != nil {
		log.Printf("pokewall: load state %s: %v; starting empty", w.statePath, err)
		return
	}
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, id := range ps.Order {
		pt, ok := ps.Tiles[id]
		if !ok {
			continue
		}
		w.order = append(w.order, id)
		w.tiles[id] = &Tile{
			RunID:       pt.RunID,
			Status:      pt.Status,
			Planner:     pt.Planner,
			Starter:     pt.Starter,
			Dest:        pt.Dest,
			Seed:        pt.Seed,
			FPS:         pt.FPS,
			MaxRounds:   pt.MaxRounds,
			MaxFrames:   pt.MaxFrames,
			Attempts:    pt.Attempts,
			Frame:       pt.Frame,
			Map:         pt.Map,
			X:           pt.X,
			Y:           pt.Y,
			Trace:       pt.Trace,
			StopSoFar:   pt.StopSoFar,
			Reason:      pt.Reason,
			Detail:      pt.Detail,
			Finished:    pt.Finished,
			workerAddrs: append([]string(nil), pt.WorkerAddrs...),
			lastUpdate:  now,
		}
	}
	w.queue = append(w.queue, ps.Queue...)
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// safeBase derives a filesystem-safe single filename stem from a run ID.
// Every path separator and every other odd byte becomes '_', so the result
// can never escape its parent directory — ".." itself is replaced outright.
// pokeui's frame route applies this same rule to ?run= (see cmd/pokeui).
func safeBase(runID string) string {
	name := unsafeName.ReplaceAllString(runID, "_")
	if name == "" || name == "." || name == ".." {
		name = "run"
	}
	return name
}

// safeDumpName derives a filesystem-safe single filename from a run ID.
func safeDumpName(runID string) string {
	return safeBase(runID) + ".json"
}

// Handler wires the wall's endpoints.
func (w *Wall) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/specs", w.handleSpecs)
	mux.HandleFunc("POST /v1/lease", w.handleLease)
	mux.HandleFunc("POST /v1/workers", w.handleWorkers)
	mux.HandleFunc("POST /v1/runs/{id}/heartbeat", w.handleHeartbeat)
	mux.HandleFunc("POST /v1/runs/{id}/cancel", w.handleCancel)
	mux.HandleFunc("POST /v1/runs/{id}/finish", w.handleFinish)
	mux.HandleFunc("GET /v1/dashboard", w.handleDashboard)
	mux.HandleFunc("GET /v1/triage", w.handleTriage)
	mux.HandleFunc("GET /", w.handleGrid)
	mux.HandleFunc("GET /frame", w.handleFrame)
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
	w.saveState()
	writeJSON(res, http.StatusOK, map[string]string{"status": statusQueued})
}

func (w *Wall) applySpec(runID string, spec farm.Spec) {
	t := w.tiles[runID]
	t.RunID = spec.RunID
	t.Status = statusQueued
	t.lastUpdate = time.Now()
	t.Planner = spec.Planner
	t.Starter = spec.Starter
	t.Dest = spec.Dest
	t.Seed = spec.Seed
	t.FPS = spec.FPS
	t.MaxRounds = spec.MaxRounds
	t.MaxFrames = spec.MaxFrames
	t.Attempts = 0 // a manual re-queue is a fresh start, not a retry
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
	t.lastUpdate = time.Now()
	spec := farm.Spec{
		RunID:     t.RunID,
		Attempt:   t.Attempts + 1,
		Seed:      t.Seed,
		Planner:   t.Planner,
		Starter:   t.Starter,
		Dest:      t.Dest,
		FPS:       t.FPS,
		MaxRounds: t.MaxRounds,
		MaxFrames: t.MaxFrames,
	}
	w.mu.Unlock()
	w.saveState()
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
	t.workerAddrs = hb.WorkerAddrs
	t.lastUpdate = time.Now()
	w.upsertWorkerLocked(hb.WorkerAddrs, id, t.lastUpdate)
	cancel := w.cancel[id]
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, farm.HeartbeatReply{Cancel: cancel})
}

// workerInfo is the wall's presence record for one runner, keyed by the
// first of the addresses the runner reports. It is ephemeral by design and
// never persisted: a worker that stops pinging (runner died or rolled) ages
// out of the grid, so the section never shows capacity that is not there.
type workerInfo struct {
	Addrs    []string
	RunID    string // "" while idle between runs
	LastSeen time.Time
}

// workerRow is the plain snapshot renderGrid hands to the template, taken
// under w.mu like tileRow.
type workerRow struct {
	Addr    string `json:"addr"`
	RunID   string `json:"run_id"`
	SeenAgo string `json:"seen_ago"`
}

// dashboardView is the JSON snapshot GET /v1/dashboard returns, and the
// source renderGrid uses for the in-network debug table.
type dashboardView struct {
	Now     int64       `json:"now"`
	Runs    []tileRow   `json:"runs"`
	Workers []workerRow `json:"workers"`
}

// upsertWorkerLocked records that the runner reporting addrs is alive: an
// idle ping carries runID "", a heartbeat its in-flight run. Caller holds
// w.mu.
func (w *Wall) upsertWorkerLocked(addrs []string, runID string, now time.Time) {
	if len(addrs) == 0 {
		return
	}
	w.workers[addrs[0]] = &workerInfo{Addrs: addrs, RunID: runID, LastSeen: now}
}

// handleWorkers is the idle half of worker presence: a runner pings it on
// every lease attempt so the grid shows available capacity, not just runs
// in flight. It is presence, not work: the queue and tiles are untouched.
func (w *Wall) handleWorkers(res http.ResponseWriter, req *http.Request) {
	var ping farm.WorkerPing
	if err := json.NewDecoder(req.Body).Decode(&ping); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad worker ping: " + err.Error()})
		return
	}
	if len(ping.Addrs) == 0 {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "addrs is required"})
		return
	}
	now := time.Now()
	w.mu.Lock()
	w.upsertWorkerLocked(ping.Addrs, "", now)
	w.mu.Unlock()
	writeJSON(res, http.StatusOK, map[string]string{"status": "ok"})
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
	} else if report.Attempt != 0 && report.Attempt != t.Attempts+1 {
		// A late finish from an earlier attempt (its runner died and the
		// run was retried) must not settle the attempt now in flight.
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("stale finish: run is on attempt %d, report claims %d", t.Attempts+1, report.Attempt),
		})
		return
	} else {
		addrs := append([]string(nil), t.workerAddrs...)
		w.mu.Unlock()
		if data, err := fetchRunnerFrame(addrs); err == nil {
			w.mu.Lock()
			if cur := w.tiles[id]; cur != nil {
				cur.lastFrame = data
			}
			w.mu.Unlock()
		}
		w.mu.Lock()
		w.settleRun(t, report.Reason, report.Detail, time.Now())
	}
	w.mu.Unlock()
	w.saveState()

	if w.dumpsDir != "" {
		data, err := json.Marshal(report)
		if err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "encode dump: " + err.Error()})
			return
		}
		// Attempt 1 keeps the historic name; retries get their own file so
		// no attempt's dump is overwritten by a later one.
		name := safeDumpName(report.RunID)
		if report.Attempt > 1 {
			name = fmt.Sprintf("%s-attempt-%d.json", safeBase(report.RunID), report.Attempt)
		}
		if err := os.WriteFile(filepath.Join(w.dumpsDir, name), data, 0o644); err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "write dump: " + err.Error()})
			return
		}
	}
	writeJSON(res, http.StatusOK, map[string]string{"status": statusDone})
}

// frameTimeout bounds one upstream fetch of a runner's frame: a wedged
// runner must not stall the caller. The live page issues these back-to-back,
// so the client is reused (keep-alive) rather than built per fetch.
const frameTimeout = time.Second

var frameClient = &http.Client{Timeout: frameTimeout}

// fetchRunnerFrame pulls one runner's live screen, trying the addresses the
// runner reported in its heartbeats in order until one answers.
func fetchRunnerFrame(addrs []string) ([]byte, error) {
	client := frameClient
	for _, addr := range addrs {
		up, err := client.Get("http://" + addr + "/frame.png")
		if err != nil {
			continue
		}
		if up.StatusCode != http.StatusOK {
			up.Body.Close()
			continue
		}
		data, rerr := io.ReadAll(up.Body)
		up.Body.Close()
		if rerr != nil {
			continue
		}
		return data, nil
	}
	return nil, errors.New("no worker address answered")
}

// handleFrame proxies one runner's live screen for the in-network dashboard.
// The wall reaches the specific runner over the swarm network using the
// addresses that runner reported in its heartbeats. Only a run that is
// actively running has frames — queued, leased and finished runs are 404.
func (w *Wall) handleFrame(res http.ResponseWriter, req *http.Request) {
	runID := req.URL.Query().Get("run")
	w.mu.Lock()
	t, ok := w.tiles[runID]
	var addrs []string
	var cached []byte
	live := false
	if ok {
		addrs = append([]string(nil), t.workerAddrs...)
		cached = t.lastFrame
		live = !t.Finished && t.Status == statusRunning
	}
	w.mu.Unlock()
	if !ok {
		res.WriteHeader(http.StatusNotFound)
		return
	}

	if live {
		data, err := fetchRunnerFrame(addrs)
		if err == nil {
			w.mu.Lock()
			if cur := w.tiles[runID]; cur != nil {
				cur.lastFrame = data
			}
			w.mu.Unlock()
			writePNG(res, data)
			return
		}
	}
	if len(cached) > 0 {
		writePNG(res, cached)
		return
	}
	if live {
		res.WriteHeader(http.StatusBadGateway)
		return
	}
	res.WriteHeader(http.StatusNotFound)
}

func writePNG(res http.ResponseWriter, data []byte) {
	res.Header().Set("Content-Type", "image/png")
	res.Header().Set("Cache-Control", "no-store")
	res.Write(data) //nolint:errcheck // best effort: the browser retries on refresh
}

var gridTmpl = template.Must(template.New("grid").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>pokefarm wall</title>
<meta http-equiv="refresh" content="2"></head>
<body>
<h1>pokefarm wall</h1>
<table border="1" cellspacing="0">
<tr><th>run</th><th>screen</th><th>status</th><th>planner</th><th>starter</th><th>dest</th><th>seed</th><th>frame</th><th>map</th><th>x,y</th><th>trace</th><th>stop so far</th><th>attempts</th><th>reason</th><th>detail</th></tr>
{{range .Rows}}<tr><td>{{.RunID}}</td><td>{{if eq .Status "running"}}<img src="/frame?run={{.RunID}}&t={{$.Now}}" width="160" style="image-rendering:pixelated">{{else}}&mdash;{{end}}</td><td>{{.Status}}</td><td>{{.Planner}}</td><td>{{.Starter}}</td><td>{{.Dest}}</td><td>{{.Seed}}</td><td>{{.Frame}}</td><td>{{printf "0x%02x" .Map}}</td><td>{{.X}},{{.Y}}</td><td>{{.Trace}}</td><td>{{.StopSoFar}}</td><td>{{.Attempts}}</td><td>{{.Reason}}</td><td>{{.Detail}}</td></tr>
{{else}}<tr><td colspan="15">no runs</td></tr>
{{end}}</table>
<h2>workers</h2>
<table border="1" cellspacing="0">
<tr><th>worker</th><th>status</th><th>seen</th></tr>
{{range .Workers}}<tr><td>{{.Addr}}</td><td>{{if .RunID}}running {{.RunID}}{{else}}idle{{end}}</td><td>{{.SeenAgo}} ago</td></tr>
{{else}}<tr><td colspan="3">no workers</td></tr>
{{end}}</table>
<h2>failure groups</h2>
<table border="1" cellspacing="0">
<tr><th>count</th><th>pattern</th><th>example</th><th>runs</th></tr>
{{range .Groups}}<tr><td>{{.Count}}</td><td>{{.Pattern}}</td><td>{{.Example}}</td><td>{{range $i, $id := .RunIDs}}{{if $i}}, {{end}}{{$id}}{{end}}</td></tr>
{{else}}<tr><td colspan="4">no failures</td></tr>
{{end}}</table>
</body></html>`))

// snapshot copies tiles and workers under w.mu so callers never read live
// maps after unlock. Insertion order is preserved for runs; workers are
// sorted by addr so the page does not flicker.
func (w *Wall) snapshot() dashboardView {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	workers := make([]workerRow, 0, len(w.workers))
	for _, wk := range w.workers {
		workers = append(workers, workerRow{
			Addr:    wk.Addrs[0],
			RunID:   wk.RunID,
			SeenAgo: now.Sub(wk.LastSeen).Round(time.Second).String(),
		})
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].Addr < workers[j].Addr })
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
			Attempts:  t.Attempts,
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
	return dashboardView{Now: now.Unix(), Runs: rows, Workers: workers}
}

// renderGrid renders the known tiles into the in-network debug HTML.
func (w *Wall) renderGrid() ([]byte, error) {
	dash := w.snapshot()
	var buf bytes.Buffer
	view := struct {
		Rows    []tileRow
		Workers []workerRow
		Groups  []triageGroup
		Now     int64
	}{dash.Runs, dash.Workers, w.triage(), dash.Now}
	if err := gridTmpl.Execute(&buf, view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (w *Wall) handleDashboard(res http.ResponseWriter, req *http.Request) {
	writeJSON(res, http.StatusOK, w.snapshot())
}

var (
	// triageHexRe matches 0x-prefixed hex literals (map ids and the like).
	triageHexRe = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	// triageNumRe matches runs of digits (coordinates, frame counts).
	triageNumRe = regexp.MustCompile(`\d+`)
)

const (
	// triagePatternCap bounds the normalised pattern. Past this length two
	// details sharing a long prefix would merge into one group, so the cap
	// is generous enough for every real error string and short enough that
	// only genuinely different tails can still split groups.
	triagePatternCap = 128
	// triageRunIDCap caps how many run ids a group reports. The count is
	// exact; the id list is a sample to go looking for the runs.
	triageRunIDCap = 5
)

// normalizeDetail reduces a failure detail to its pattern: 0x-prefixed hex
// literals become <hex>, runs of digits become <n>, then the length is
// capped. "map 0x0c at (10,35)" and "map 0x21 at (4,22)" land on the same
// pattern; the words that name the bug stay untouched.
func normalizeDetail(detail string) string {
	s := triageHexRe.ReplaceAllString(detail, "<hex>")
	s = triageNumRe.ReplaceAllString(s, "<n>")
	if len(s) > triagePatternCap {
		s = s[:triagePatternCap]
	}
	return s
}

// triageGroup is one cluster of failed runs sharing a normalised detail.
type triageGroup struct {
	Pattern string   `json:"pattern"` // the normalised detail
	Count   int      `json:"count"`   // exact number of runs in the group
	Example string   `json:"example"` // ONE verbatim detail from the group
	RunIDs  []string `json:"run_ids"` // capped at triageRunIDCap
}

// triageGroups groups finished, failed runs (reason error or lost) with a
// non-empty detail by normalised pattern, most frequent first. A run with an
// empty detail is not a cluster of one — it is a run that finished — and a
// run that finished cleanly never appears at all.
func triageGroups(order []string, tiles map[string]*Tile) []triageGroup {
	type acc struct {
		example string
		count   int
		ids     []string
	}
	groups := map[string]*acc{}
	var keys []string
	for _, id := range order {
		t := tiles[id]
		if t == nil || !t.Finished || (t.Reason != "error" && t.Reason != "lost") || t.Detail == "" {
			continue
		}
		key := normalizeDetail(t.Detail)
		a, ok := groups[key]
		if !ok {
			a = &acc{example: t.Detail}
			groups[key] = a
			keys = append(keys, key)
		}
		a.count++
		if len(a.ids) < triageRunIDCap {
			a.ids = append(a.ids, t.RunID)
		}
	}
	out := make([]triageGroup, 0, len(keys))
	for _, k := range keys {
		a := groups[k]
		out = append(out, triageGroup{Pattern: k, Count: a.count, Example: a.example, RunIDs: a.ids})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// triage is the wall's failure ranking, taken under w.mu.
func (w *Wall) triage() []triageGroup {
	w.mu.Lock()
	defer w.mu.Unlock()
	return triageGroups(w.order, w.tiles)
}

// handleTriage reports failure groups, most frequent first. It is a read:
// it creates no tasks, calls no runner, and writes nothing. A human reads
// the ranking and decides what to file — a queue filled automatically by a
// noisy classifier would be worse than no queue.
func (w *Wall) handleTriage(res http.ResponseWriter, req *http.Request) {
	writeJSON(res, http.StatusOK, w.triage())
}

// handleGrid renders the known tiles.
func (w *Wall) handleGrid(res http.ResponseWriter, req *http.Request) {
	data, err := w.renderGrid()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	res.Write(data) //nolint:errcheck // best effort: the page refreshes itself
}

// Publish writes the dashboard — the grid HTML plus each running run's
// latest frame — into dir, for a browser-facing relay (pokeui) that cannot
// reach the swarm network directly. Every file is written atomically (temp
// file + rename in the same directory), so the relay never reads a half
// written page or frame. A run whose runner does not answer keeps its
// previous frame; the next tick retries.
func (w *Wall) Publish(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "live"), 0o755); err != nil {
		return err
	}
	html, err := w.renderGrid()
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, "index.html"), html, 0o644); err != nil {
		return err
	}

	w.mu.Lock()
	type frameJob struct {
		id    string
		addrs []string
	}
	jobs := make([]frameJob, 0, len(w.order))
	for _, id := range w.order {
		t := w.tiles[id]
		if !t.Finished && t.Status == statusRunning {
			jobs = append(jobs, frameJob{id, append([]string(nil), t.workerAddrs...)})
		}
	}
	w.mu.Unlock()

	for _, job := range jobs {
		data, err := fetchRunnerFrame(job.addrs)
		if err != nil {
			continue
		}
		if err := writeAtomic(filepath.Join(dir, "live", safeBase(job.id)+".png"), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// writeAtomic writes data to path via a temp file + rename in the same
// directory, so readers never observe a partial file.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".publish-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename has succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// RunPublisher publishes the dashboard to dir every interval. The first
// publish happens immediately, so the relay has content without waiting a
// full interval after the wall starts.
func (w *Wall) RunPublisher(dir string, interval time.Duration) {
	tick := time.After(0)
	for {
		<-tick
		if err := w.Publish(dir); err != nil {
			log.Printf("pokewall: publish to %s: %v", dir, err)
		}
		tick = time.After(interval)
	}
}

// defaultStaleExpiry is how long a leased or running run may go without a
// state change (its heartbeats come every second) before the wall declares
// it lost. Thirty seconds is thirty heartbeat cycles of margin.
const defaultStaleExpiry = 30 * time.Second

// maxAttempts is how many times a run may be attempted before the wall
// settles it as failed.
const maxAttempts = 3

// settleRun records that a run stopped for reason/detail. error and lost
// are retried up to maxAttempts, each retry with a fresh seed (the same
// seed replays the same bad luck); everything else settles as done. A run
// the user cancelled is never retried: that stop was intentional. Caller
// holds w.mu; it returns the number of attempts completed so far.
func (w *Wall) settleRun(t *Tile, reason, detail string, now time.Time) int {
	t.Attempts++
	completed := t.Attempts
	t.lastUpdate = now
	_, cancelled := w.cancel[t.RunID]
	delete(w.cancel, t.RunID)
	if (reason != "error" && reason != "lost") || cancelled || completed >= maxAttempts {
		t.Status = statusDone
		t.Reason = reason
		t.Detail = detail
		t.Finished = true
		return completed
	}
	// Retry: fresh luck, fresh progress, back of the queue. The old
	// runner's addresses belong to its attempt, not this one.
	t.Status = statusQueued
	t.Seed = rand.Int64()
	t.Frame = 0
	t.Map = 0
	t.X = 0
	t.Y = 0
	t.Trace = ""
	t.StopSoFar = ""
	t.Reason = ""
	t.Detail = fmt.Sprintf("attempt %d failed: %s", completed, detail)
	t.workerAddrs = nil
	t.Finished = false
	w.queue = append(w.queue, t.RunID)
	return completed
}

// reapStale handles leased or running runs whose lastUpdate is older than
// w.staleAfter: settleRun declares them lost and retries them while
// attempts remain. It returns what it handled. Queued runs are never
// reaped: they are waiting for a runner, not on one. It also drops workers
// unseen for longer than workerExpiry; worker presence is never persisted,
// so only the run handling triggers a save.
func (w *Wall) reapStale(now time.Time) []string {
	w.mu.Lock()
	var reaped []string
	for _, id := range w.order {
		t := w.tiles[id]
		if t.Finished || t.Status == statusQueued {
			continue
		}
		age := now.Sub(t.lastUpdate)
		if age <= w.staleAfter {
			continue
		}
		reaped = append(reaped, id)
		w.settleRun(t, "lost", fmt.Sprintf("no heartbeat for %s", age.Round(time.Second)), now)
	}
	for key, wk := range w.workers {
		if now.Sub(wk.LastSeen) > w.workerExpiry {
			delete(w.workers, key)
		}
	}
	w.mu.Unlock()
	if len(reaped) > 0 {
		w.saveState()
	}
	return reaped
}

// RunReaper reaps stale runs on a fixed interval.
func (w *Wall) RunReaper(interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for now := range tick.C {
		if reaped := w.reapStale(now); len(reaped) > 0 {
			log.Printf("pokewall: reaped stale runs: %v", reaped)
		}
	}
}
