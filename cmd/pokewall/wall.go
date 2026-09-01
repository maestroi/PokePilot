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
	"strconv"
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
	RunID      string
	Status     string
	Planner    string
	Starter    string
	Dest       string
	Goal       string
	Seed       int64
	FPS        int
	MaxRounds  int
	MaxFrames  int
	Endless    bool
	RandomSeed bool
	QueuedAt   time.Time
	EndedAt    time.Time
	// Attempts counts completed attempts; a retried run keeps its history
	// so the grid can show where it is in its retry budget.
	Attempts int
	Frame    uint64
	Map      uint8
	X        uint8
	Y        uint8
	Trace    string
	Question string
	Decision string
	// Raw is the last verbatim model exchange from the heartbeat. Live
	// only: it is deliberately absent from persistedTile, so a wall
	// restart drops it rather than growing the state file.
	Raw       string
	StopSoFar string
	// Sprites and Trail are the live map overlay from the runner. Like Raw,
	// they are deliberately not persisted: a wall restart waits for the next
	// heartbeat rather than showing stale blockers or a stale path.
	Sprites []farm.MapSprite
	Trail   [][2]uint8
	// Stats is the llm planner's tally, last pushed by a heartbeat. Kept on
	// finish (the final tally explains the outcome), nilled on retry.
	Stats    *farm.LLMStats
	Reason   string
	Detail   string
	Finished bool
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
	RunID      string           `json:"run_id"`
	Status     string           `json:"status"`
	Planner    string           `json:"planner"`
	Starter    string           `json:"starter"`
	Dest       string           `json:"dest"`
	Goal       string           `json:"goal,omitempty"`
	Seed       int64            `json:"seed"`
	FPS        int              `json:"fps"`
	MaxRounds  int              `json:"max_rounds"`
	MaxFrames  int              `json:"max_frames"`
	Endless    bool             `json:"endless,omitempty"`
	RandomSeed bool             `json:"random_seed,omitempty"`
	QueuedAt   int64            `json:"queued_at,omitempty"`
	EndedAt    int64            `json:"ended_at,omitempty"`
	Frame      uint64           `json:"frame"`
	Map        uint8            `json:"map"`
	X          uint8            `json:"x"`
	Y          uint8            `json:"y"`
	Trace      string           `json:"trace"`
	Question   string           `json:"question,omitempty"`
	Decision   string           `json:"decision,omitempty"`
	Raw        string           `json:"raw,omitempty"`
	StopSoFar  string           `json:"stop_so_far"`
	Sprites    []farm.MapSprite `json:"sprites,omitempty"`
	Trail      [][2]uint8        `json:"trail,omitempty"`
	Stats      *farm.LLMStats   `json:"stats,omitempty"`
	Attempts   int              `json:"attempts"`
	Reason     string           `json:"reason"`
	Detail     string           `json:"detail"`
	Issue      *IssueLink       `json:"issue,omitempty"`
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
	Version      string                 // this wall's build identity, shown in the dashboard and grid
	issueLinks   map[string]IssueLink   // failure key → Agent Orchestrator issue
	outbox       map[string]outboxEntry // external occurrence id → report outbox
	issues       *issueClient
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
		issueLinks:   map[string]IssueLink{},
		outbox:       map[string]outboxEntry{},
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
	RunID       string         `json:"run_id"`
	Status      string         `json:"status"`
	Planner     string         `json:"planner,omitempty"`
	Starter     string         `json:"starter,omitempty"`
	Dest        string         `json:"dest,omitempty"`
	Goal        string         `json:"goal,omitempty"`
	Seed        int64          `json:"seed"`
	FPS         int            `json:"fps"`
	MaxRounds   int            `json:"max_rounds"`
	MaxFrames   int            `json:"max_frames"`
	Endless     bool           `json:"endless,omitempty"`
	RandomSeed  bool           `json:"random_seed,omitempty"`
	QueuedAt    int64          `json:"queued_at,omitempty"`
	EndedAt     int64          `json:"ended_at,omitempty"`
	Attempts    int            `json:"attempts"`
	Frame       uint64         `json:"frame"`
	Map         uint8          `json:"map"`
	X           uint8          `json:"x"`
	Y           uint8          `json:"y"`
	Trace       string         `json:"trace,omitempty"`
	Question    string         `json:"question,omitempty"`
	Decision    string         `json:"decision,omitempty"`
	StopSoFar   string         `json:"stop_so_far,omitempty"`
	Stats       *farm.LLMStats `json:"stats,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Detail      string         `json:"detail,omitempty"`
	Finished    bool           `json:"finished"`
	WorkerAddrs []string       `json:"worker_addrs,omitempty"`
}

// persistedState is the wall's whole on-disk memory: run order, tiles, and
// the queue of not-yet-leased runs.
type persistedState struct {
	Order      []string                 `json:"order"`
	Queue      []string                 `json:"queue"`
	Tiles      map[string]persistedTile `json:"tiles"`
	IssueLinks map[string]IssueLink     `json:"issue_links,omitempty"`
	Outbox     map[string]outboxEntry   `json:"outbox,omitempty"`
}

// marshalStateLocked encodes the wall's memory. Caller holds w.mu.
func (w *Wall) marshalStateLocked() ([]byte, error) {
	ps := persistedState{
		Order:      append([]string(nil), w.order...),
		Queue:      append([]string(nil), w.queue...),
		Tiles:      make(map[string]persistedTile, len(w.tiles)),
		IssueLinks: copyIssueLink(w.issueLinks),
		Outbox:     copyOutbox(w.outbox),
	}
	for id, t := range w.tiles {
		ps.Tiles[id] = persistedTile{
			RunID:       t.RunID,
			Status:      t.Status,
			Planner:     t.Planner,
			Starter:     t.Starter,
			Dest:        t.Dest,
			Goal:        t.Goal,
			Seed:        t.Seed,
			FPS:         t.FPS,
			MaxRounds:   t.MaxRounds,
			MaxFrames:   t.MaxFrames,
			Endless:     t.Endless,
			RandomSeed:  t.RandomSeed,
			QueuedAt:    unixTime(t.QueuedAt),
			EndedAt:     unixTime(t.EndedAt),
			Attempts:    t.Attempts,
			Frame:       t.Frame,
			Map:         t.Map,
			X:           t.X,
			Y:           t.Y,
			Trace:       t.Trace,
			Question:    t.Question,
			Decision:    t.Decision,
			StopSoFar:   t.StopSoFar,
			Stats:       t.Stats,
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
			Goal:        pt.Goal,
			Seed:        pt.Seed,
			FPS:         pt.FPS,
			MaxRounds:   pt.MaxRounds,
			MaxFrames:   pt.MaxFrames,
			Endless:     pt.Endless,
			RandomSeed:  pt.RandomSeed,
			QueuedAt:    timeFromUnix(pt.QueuedAt),
			EndedAt:     timeFromUnix(pt.EndedAt),
			Attempts:    pt.Attempts,
			Frame:       pt.Frame,
			Map:         pt.Map,
			X:           pt.X,
			Y:           pt.Y,
			Trace:       pt.Trace,
			Question:    pt.Question,
			Decision:    pt.Decision,
			StopSoFar:   pt.StopSoFar,
			Stats:       pt.Stats,
			Reason:      pt.Reason,
			Detail:      pt.Detail,
			Finished:    pt.Finished,
			workerAddrs: append([]string(nil), pt.WorkerAddrs...),
			lastUpdate:  now,
		}
	}
	w.queue = append(w.queue, ps.Queue...)
	w.issueLinks = copyIssueLink(ps.IssueLinks)
	w.outbox = copyOutbox(ps.Outbox)
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
	mux.HandleFunc("DELETE /v1/runs/{id}", w.handleDelete)
	mux.HandleFunc("POST /v1/runs/{id}/finish", w.handleFinish)
	mux.HandleFunc("POST /v1/runs/{id}/checkpoint", w.handleCheckpoint)
	mux.HandleFunc("GET /v1/dashboard", w.handleDashboard)
	mux.HandleFunc("GET /v1/triage", w.handleTriage)
	mux.HandleFunc("POST /v1/triage/{key}/investigate", w.handleInvestigate)
	mux.HandleFunc("GET /frame", w.handleFrame)
	mux.HandleFunc("GET /{$}", w.handleGrid)
	return mux
}

// handleSpecs enqueues a run. A run ID may be reused only after its previous
// run has finished; otherwise the operator gets 409 rather than silently
// replacing work in flight.
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
	if old := w.tiles[spec.RunID]; old != nil && !old.Finished {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already active: " + spec.RunID})
		return
	}
	w.enqueueLocked(spec)
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, map[string]string{"status": statusQueued})
}

func (w *Wall) enqueueLocked(spec farm.Spec) {
	t := &Tile{RunID: spec.RunID, Status: statusQueued}
	if old := w.tiles[spec.RunID]; old != nil {
		*t = *old
		t.Status = statusQueued
		t.Frame = 0
		t.Map = 0
		t.X = 0
		t.Y = 0
		t.Trace = ""
		t.Question = ""
		t.Decision = ""
		t.Raw = ""
		t.StopSoFar = ""
		t.Sprites = nil
		t.Trail = nil
		t.Stats = nil
		t.Reason = ""
		t.Detail = ""
		t.workerAddrs = nil
		t.lastFrame = nil
		delete(w.cancel, spec.RunID)
	}
	w.tiles[spec.RunID] = t
	if old := indexOf(w.order, spec.RunID); old < 0 {
		w.order = append(w.order, spec.RunID)
	}
	w.queue = append(w.queue, spec.RunID)
	t.Planner = spec.Planner
	t.Starter = spec.Starter
	t.Dest = spec.Dest
	t.Goal = spec.Goal
	t.Seed = spec.Seed
	t.FPS = spec.FPS
	t.MaxRounds = spec.MaxRounds
	t.MaxFrames = spec.MaxFrames
	t.Endless = spec.Endless
	t.RandomSeed = spec.RandomSeed
	t.QueuedAt = time.Now()
	t.EndedAt = time.Time{}
	t.Attempts = 0 // a manual re-queue is a fresh start, not a retry
	t.Finished = false
}

func indexOf(ids []string, id string) int {
	for i, x := range ids {
		if x == id {
			return i
		}
	}
	return -1
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
		RunID:      t.RunID,
		Attempt:    t.Attempts + 1,
		Seed:       t.Seed,
		Planner:    t.Planner,
		Starter:    t.Starter,
		Dest:       t.Dest,
		Goal:       t.Goal,
		FPS:        t.FPS,
		MaxRounds:  t.MaxRounds,
		MaxFrames:  t.MaxFrames,
		Endless:    t.Endless,
		RandomSeed: t.RandomSeed,
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
	t.Question = hb.Question
	t.Decision = hb.Decision
	t.Raw = hb.Raw
	t.StopSoFar = hb.StopSoFar
	t.Sprites = append(t.Sprites[:0], hb.Sprites...)
	t.Trail = append(t.Trail[:0], hb.Trail...)
	t.Stats = hb.Stats
	t.workerAddrs = hb.WorkerAddrs
	t.lastUpdate = time.Now()
	w.upsertWorkerLocked(hb.WorkerAddrs, id, hb.Version, t.lastUpdate)
	cancel := w.cancel[id]
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, farm.HeartbeatReply{Cancel: cancel})
}

// workerInfo is the wall's presence record one runner, keyed by the first of
// the addresses it reports. It is ephemeral and is never persisted.
type workerInfo struct {
	Addrs    []string
	Version  string
	RunID    string
	LastSeen time.Time
}

type workerRow struct {
	Addr    string `json:"addr"`
	Version string `json:"version,omitempty"`
	RunID   string `json:"run_id"`
	SeenAgo string `json:"seen_ago"`
}

type dashboardView struct {
	Now         int64       `json:"now"`
	WallVersion string      `json:"wall_version,omitempty"`
	Runs        []tileRow   `json:"runs"`
	Workers     []workerRow `json:"workers"`
}

func (w *Wall) upsertWorkerLocked(addrs []string, runID, version string, now time.Time) {
	if len(addrs) == 0 {
		return
	}
	w.workers[addrs[0]] = &workerInfo{Addrs: addrs, Version: version, RunID: runID, LastSeen: now}
}

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
	w.upsertWorkerLocked(ping.Addrs, "", ping.Version, now)
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

func (w *Wall) handleDelete(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	w.mu.Lock()
	t, ok := w.tiles[id]
	if !ok {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
		return
	}
	if !t.Finished {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run still active: " + id})
		return
	}
	delete(w.tiles, id)
	delete(w.cancel, id)
	w.order = removeID(w.order, id)
	w.mu.Unlock()
	w.saveState()
	writeJSON(res, http.StatusOK, map[string]bool{"deleted": true})
}

func removeID(ids []string, id string) []string {
	out := make([]string, 0, len(ids))
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

// handleFinish records why a run ended. An identical repeat is idempotent;
// a conflicting repeat is 409.
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
	if err := farm.ValidateFinishArtifacts(report); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
	} else if report.Attempt != 0 && report.Attempt != t.Attempts+1 {
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
		name := safeDumpName(report.RunID)
		if report.Attempt > 1 {
			name = fmt.Sprintf("%s-attempt-%d.json", safeBase(report.RunID), report.Attempt)
		}
		if err := os.WriteFile(filepath.Join(w.dumpsDir, name), data, 0o644); err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "write dump: " + err.Error()})
			return
		}
		w.enqueueIssueAfterDump(id, report)
		w.saveState()
	}
	writeJSON(res, http.StatusOK, map[string]string{"status": statusDone})
}

const frameTimeout = time.Second
var frameClient = &http.Client{Timeout: frameTimeout}

func fetchRunnerFrame(addrs []string) ([]byte, error) {
	var last error
	for _, addr := range addrs {
		resp, err := frameClient.Get("http://" + addr + "/frame.png")
		if err != nil {
			last = err
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			last = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			last = fmt.Errorf("%s returned %s", addr, resp.Status)
			continue
		}
		return data, nil
	}
	if last == nil {
		last = errors.New("no worker address")
	}
	return nil, last
}

func (w *Wall) handleFrame(res http.ResponseWriter, req *http.Request) {
	id := req.URL.Query().Get("run")
	w.mu.Lock()
	t := w.tiles[id]
	if t == nil {
		w.mu.Unlock()
		http.NotFound(res, req)
		return
	}
	addrs := append([]string(nil), t.workerAddrs...)
	last := append([]byte(nil), t.lastFrame...)
	done := t.Finished
	w.mu.Unlock()
	if !done {
		if data, err := fetchRunnerFrame(addrs); err == nil {
			res.Header().Set("Content-Type", "image/png")
			res.Header().Set("Cache-Control", "no-store")
			res.Write(data) //nolint:errcheck
			return
		}
	}
	if len(last) == 0 {
		http.NotFound(res, req)
		return
	}
	res.Header().Set("Content-Type", "image/png")
	res.Header().Set("Cache-Control", "no-store")
	res.Write(last) //nolint:errcheck
}

func (w *Wall) snapshot() dashboardView {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	workers := make([]workerRow, 0, len(w.workers))
	for _, wk := range w.workers {
		workers = append(workers, workerRow{
			Addr: wk.Addrs[0], Version: wk.Version, RunID: wk.RunID,
			SeenAgo: now.Sub(wk.LastSeen).Round(time.Second).String(),
		})
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].Addr < workers[j].Addr })
	rows := make([]tileRow, 0, len(w.order))
	for i := len(w.order) - 1; i >= 0; i-- {
		id := w.order[i]
		t := w.tiles[id]
		rows = append(rows, tileRow{
			RunID: t.RunID, Status: t.Status, Planner: t.Planner, Starter: t.Starter,
			Dest: t.Dest, Goal: t.Goal, Seed: t.Seed, FPS: t.FPS, MaxRounds: t.MaxRounds,
			MaxFrames: t.MaxFrames, Endless: t.Endless, RandomSeed: t.RandomSeed,
			QueuedAt: unixTime(t.QueuedAt), EndedAt: unixTime(t.EndedAt), Attempts: t.Attempts,
			Frame: t.Frame, Map: t.Map, X: t.X, Y: t.Y, Trace: t.Trace,
			Question: t.Question, Decision: t.Decision, Raw: t.Raw, StopSoFar: t.StopSoFar,
			Sprites: append([]farm.MapSprite(nil), t.Sprites...), Trail: append([][2]uint8(nil), t.Trail...),
			Stats: t.Stats, Reason: t.Reason, Detail: t.Detail, Issue: issueLinkFor(t, w.issueLinks),
		})
	}
	return dashboardView{Now: now.Unix(), WallVersion: w.Version, Runs: rows, Workers: workers}
}

func (w *Wall) renderGrid() ([]byte, error) {
	dash := w.snapshot()
	var buf bytes.Buffer
	view := struct {
		Rows    []tileRow
		Workers []workerRow
		Groups  []triageGroup
		Now     int64
		Version string
	}{dash.Runs, dash.Workers, w.triage(), dash.Now, w.Version}
	if err := gridTmpl.Execute(&buf, view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (w *Wall) handleDashboard(res http.ResponseWriter, req *http.Request) {
	writeJSON(res, http.StatusOK, w.snapshot())
}

var (
	triageHexRe = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	triageNumRe = regexp.MustCompile(`\d+`)
)
const (
	triagePatternCap = 128
	triageRunIDCap = 5
)

// The remainder of this file contains triage, retry, checkpoint and template
// helpers. It is unchanged by the live-map feature.
