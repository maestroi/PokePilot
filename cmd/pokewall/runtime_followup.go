package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// runtimeOperatorHTTPHandler adds the scale-sensitive production routes around
// the compatibility handler. Wall.Handler remains deliberately stable for
// runner/client tests, while production gets pagination, event-driven failure
// reporting, and local dump cleanup.
func runtimeOperatorHTTPHandler(w *Wall) http.Handler {
	fallback := operatorHTTPHandler(w)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/dashboard", w.handleRuntimeDashboard)
	mux.HandleFunc("POST /v1/runs/{id}/finish", w.handleRuntimeFinish)
	mux.HandleFunc("DELETE /v1/runs/{id}", w.handleRuntimeDelete)
	mux.Handle("/", fallback)
	return mux
}

type runtimeHistoryFacets struct {
	Outcomes []string `json:"outcomes"`
	Hows     []string `json:"hows"`
	Starters []string `json:"starters"`
}

type runtimeDashboardView struct {
	Now         int64                 `json:"now"`
	WallVersion string                `json:"wall_version,omitempty"`
	Runs        []tileRow             `json:"runs"`
	Workers     []workerRow           `json:"workers"`
	Total       int                   `json:"total"`
	Facets      *runtimeHistoryFacets `json:"history_facets,omitempty"`
}

type runtimeDashboardQuery struct {
	status  string
	limit   int
	offset  int
	active  bool
	facets  bool
	outcome string
	how     string
	starter string
}

func (w *Wall) handleRuntimeDashboard(res http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	query := runtimeDashboardQuery{
		status:  strings.ToLower(strings.TrimSpace(q.Get("status"))),
		active:  queryBool(q.Get("active")),
		facets:  queryBool(q.Get("facets")),
		outcome: strings.ToLower(strings.TrimSpace(q.Get("outcome"))),
		how:     strings.ToLower(strings.TrimSpace(q.Get("how"))),
		starter: strings.TrimSpace(q.Get("starter")),
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		query.limit = limit
	}
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "offset must be a non-negative integer"})
			return
		}
		query.offset = offset
	}
	writeJSON(res, http.StatusOK, w.runtimeDashboardSnapshot(query))
}

func queryBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func runtimeOutcome(t *Tile) string {
	if t == nil {
		return ""
	}
	if reason := strings.ToLower(strings.TrimSpace(t.Reason)); reason != "" {
		return reason
	}
	return strings.ToLower(strings.TrimSpace(t.Status))
}

func runtimeHow(t *Tile) string {
	if t != nil && t.Planner == "scripted" {
		return "walk"
	}
	return "play"
}

func runtimeStarter(t *Tile) string {
	if t == nil {
		return ""
	}
	if starter := strings.TrimSpace(t.Starter); starter != "" {
		return starter
	}
	if t.Planner == "scripted" {
		return "squirtle"
	}
	return "LLM picks"
}

func (w *Wall) runtimeDashboardSnapshot(query runtimeDashboardQuery) runtimeDashboardView {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	workers := make([]workerRow, 0, len(w.workers))
	for _, wk := range w.workers {
		if wk == nil || len(wk.Addrs) == 0 {
			continue
		}
		workers = append(workers, workerRow{
			Addr: wk.Addrs[0], Version: wk.Version, RunID: wk.RunID,
			SeenAgo: now.Sub(wk.LastSeen).Round(time.Second).String(),
		})
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].Addr < workers[j].Addr })

	outcomes := map[string]struct{}{}
	hows := map[string]struct{}{}
	starters := map[string]struct{}{}
	rows := make([]tileRow, 0, max(0, query.limit))
	total := 0
	for i := len(w.order) - 1; i >= 0; i-- {
		t := w.tiles[w.order[i]]
		if t == nil {
			continue
		}
		if query.facets && t.Status == statusDone {
			outcomes[runtimeOutcome(t)] = struct{}{}
			hows[runtimeHow(t)] = struct{}{}
			starters[runtimeStarter(t)] = struct{}{}
		}
		if query.active && t.Status == statusDone {
			continue
		}
		if query.status != "" && t.Status != query.status {
			continue
		}
		if query.outcome != "" && runtimeOutcome(t) != query.outcome {
			continue
		}
		if query.how != "" && runtimeHow(t) != query.how {
			continue
		}
		if query.starter != "" && runtimeStarter(t) != query.starter {
			continue
		}
		matchIndex := total
		total++
		if matchIndex < query.offset {
			continue
		}
		if query.limit > 0 && len(rows) >= query.limit {
			continue
		}
		rows = append(rows, w.tileRowLocked(t))
	}
	view := runtimeDashboardView{
		Now: now.Unix(), WallVersion: w.Version, Runs: rows, Workers: workers, Total: total,
	}
	if query.facets {
		view.Facets = &runtimeHistoryFacets{
			Outcomes: sortedStringSet(outcomes),
			Hows:     sortedStringSet(hows),
			Starters: sortedStringSet(starters),
		}
	}
	return view
}

func sortedStringSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

type statusCaptureWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCaptureWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCaptureWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *Wall) handleRuntimeFinish(res http.ResponseWriter, req *http.Request) {
	capture := &statusCaptureWriter{ResponseWriter: res}
	w.handleFinish(capture, req)
	if capture.status >= 200 && capture.status < 300 {
		w.queueLatestObjectiveFailureDumps(req.PathValue("id"))
	}
}

func (w *Wall) queueLatestObjectiveFailureDumps(runID string) {
	if w.dumpsDir == "" || w.issueClient() == nil {
		return
	}
	run, ok := w.snapshotRun(runID)
	if !ok || run.Attempts < 1 {
		return
	}
	// Attempt 1 has the historical filename. Queue it as a fallback too: very
	// old/hand-built clients may omit report.Attempt even on a retry and the
	// finish handler then preserves that legacy filename.
	w.queueObjectiveFailureDump(filepath.Join(w.dumpsDir, safeDumpName(runID)))
	if run.Attempts > 1 {
		w.queueObjectiveFailureDump(filepath.Join(w.dumpsDir, fmt.Sprintf("%s-attempt-%d.json", safeBase(runID), run.Attempts)))
	}
}

func (w *Wall) handleRuntimeDelete(res http.ResponseWriter, req *http.Request) {
	runID := req.PathValue("id")
	run, ok := w.snapshotRun(runID)
	if ok && run.Status == statusDone {
		if w.runArtifactsProtected(runID) {
			writeJSON(res, http.StatusConflict, map[string]string{"error": "run is still required by an active resume lineage: " + runID})
			return
		}
		attempts := run.Attempts
		if attempts < 1 {
			attempts = 1
		}
		if err := w.deleteLocalRunArtifacts(runID, attempts); err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "delete local run artifacts: " + err.Error()})
			return
		}
	}
	w.handleDelete(res, req)
}

func (w *Wall) deleteLocalFinishDumps(runID string, attempts int) error {
	if w.dumpsDir == "" {
		return nil
	}
	paths := []string{filepath.Join(w.dumpsDir, safeDumpName(runID))}
	for attempt := 2; attempt <= attempts; attempt++ {
		paths = append(paths, filepath.Join(w.dumpsDir, fmt.Sprintf("%s-attempt-%d.json", safeBase(runID), attempt)))
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		objectiveReporterFor(w).complete(path)
	}
	return nil
}

type objectiveReporterCoordinator struct {
	mu      sync.Mutex
	pending map[string]bool
	ch      chan string
}

var objectiveReporterCoordinators sync.Map // *Wall -> *objectiveReporterCoordinator

func objectiveReporterFor(w *Wall) *objectiveReporterCoordinator {
	if existing, ok := objectiveReporterCoordinators.Load(w); ok {
		return existing.(*objectiveReporterCoordinator)
	}
	created := &objectiveReporterCoordinator{pending: make(map[string]bool), ch: make(chan string, 256)}
	actual, _ := objectiveReporterCoordinators.LoadOrStore(w, created)
	return actual.(*objectiveReporterCoordinator)
}

func (c *objectiveReporterCoordinator) queue(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	c.mu.Lock()
	if c.pending[path] {
		c.mu.Unlock()
		return
	}
	c.pending[path] = true
	c.mu.Unlock()
	// Backpressure is intentional. Losing a durable failure occurrence because
	// a remote issue service is slow is worse than briefly delaying finish ACKs.
	c.ch <- path
}

func (c *objectiveReporterCoordinator) retry(path string) {
	c.ch <- path
}

func (c *objectiveReporterCoordinator) complete(path string) {
	c.mu.Lock()
	delete(c.pending, path)
	c.mu.Unlock()
}

func (w *Wall) queueObjectiveFailureDump(path string) {
	if w.issueClient() == nil {
		return
	}
	objectiveReporterFor(w).queue(path)
}

// RunObjectiveFailureEvents performs one recovery scan at startup, then reacts
// only to finish-dump commit events. Transient Agent Orchestrator failures are
// retried from the same immutable dump without rescanning/stat-ing the whole
// directory every two seconds.
func (w *Wall) RunObjectiveFailureEvents(retryEvery time.Duration) {
	if retryEvery <= 0 {
		retryEvery = defaultObjectiveFailureReportEvery
	}
	if w.dumpsDir == "" || w.issueClient() == nil {
		return
	}
	if paths, err := objectiveFailureDumpPaths(w.dumpsDir); err != nil {
		log.Printf("pokewall: initial objective failure dump scan: %v", err)
	} else {
		for _, path := range paths {
			w.processObjectiveFailureDumpEvent(path, retryEvery, false)
		}
	}
	coordinator := objectiveReporterFor(w)
	for path := range coordinator.ch {
		w.processObjectiveFailureDumpEvent(path, retryEvery, true)
	}
}

func objectiveFailureDumpPaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func (w *Wall) processObjectiveFailureDumpEvent(path string, retryEvery time.Duration, tracked bool) {
	retry, err := w.reportObjectiveFailureDump(path)
	if errors.Is(err, os.ErrNotExist) {
		retry, err = false, nil
	}
	if err != nil {
		log.Printf("pokewall: objective failure report %s: %v", filepath.Base(path), err)
	}
	coordinator := objectiveReporterFor(w)
	if retry {
		if !tracked {
			coordinator.mu.Lock()
			coordinator.pending[path] = true
			coordinator.mu.Unlock()
		}
		time.AfterFunc(retryEvery, func() { coordinator.retry(path) })
		return
	}
	coordinator.complete(path)
}
