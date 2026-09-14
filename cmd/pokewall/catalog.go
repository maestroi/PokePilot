package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/farm"
	_ "modernc.org/sqlite"
)

const catalogSchema = `
CREATE TABLE IF NOT EXISTS runs (
	run_id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	planner TEXT NOT NULL DEFAULT '',
	starter TEXT NOT NULL DEFAULT '',
	goal TEXT NOT NULL DEFAULT '',
	llm_profile TEXT NOT NULL DEFAULT '',
	queued_at INTEGER NOT NULL DEFAULT 0,
	ended_at INTEGER NOT NULL DEFAULT 0,
	outcome TEXT NOT NULL DEFAULT '',
	how TEXT NOT NULL DEFAULT '',
	starter_facet TEXT NOT NULL DEFAULT '',
	row_json BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS runs_status_ended_idx ON runs(status, ended_at DESC, queued_at DESC);
CREATE INDEX IF NOT EXISTS runs_history_filter_idx ON runs(status, outcome, how, starter_facet);
`

type runCatalog struct {
	db *sql.DB
}

var wallCatalogs sync.Map // *Wall -> *runCatalog

func openRunCatalog(path string) (*runCatalog, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, fmt.Errorf("create catalog directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, false, fmt.Errorf("open catalog: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA synchronous=NORMAL;`); err != nil {
		db.Close()
		return nil, false, fmt.Errorf("configure catalog: %w", err)
	}
	if _, err := db.Exec(catalogSchema); err != nil {
		db.Close()
		return nil, false, fmt.Errorf("create catalog schema: %w", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count); err != nil {
		db.Close()
		return nil, false, fmt.Errorf("count catalog rows: %w", err)
	}
	return &runCatalog{db: db}, count == 0, nil
}

func catalogFor(w *Wall) *runCatalog {
	if value, ok := wallCatalogs.Load(w); ok {
		return value.(*runCatalog)
	}
	return nil
}

// SetCatalogPath enables the durable history catalog. State must be loaded
// first: every state tile is upserted idempotently before finished rows are
// evicted from RAM, which makes the first rollout its own migration.
func (w *Wall) SetCatalogPath(path string) error {
	if old := catalogFor(w); old != nil {
		_ = old.db.Close()
		wallCatalogs.Delete(w)
	}
	catalog, _, err := openRunCatalog(path)
	if err != nil || catalog == nil {
		return err
	}
	wallCatalogs.Store(w, catalog)
	if err := w.syncCatalogFromRAM(true); err != nil {
		wallCatalogs.Delete(w)
		_ = catalog.db.Close()
		return err
	}
	w.evictCatalogFinished()
	w.saveState()
	return nil
}

func (w *Wall) CloseCatalog() error {
	catalog := catalogFor(w)
	if catalog == nil {
		return nil
	}
	wallCatalogs.Delete(w)
	return catalog.db.Close()
}

func catalogOutcome(row tileRow) string {
	if reason := strings.ToLower(strings.TrimSpace(row.Reason)); reason != "" {
		return reason
	}
	return strings.ToLower(strings.TrimSpace(row.Status))
}

func catalogHow(row tileRow) string {
	if row.Planner == "scripted" {
		return "walk"
	}
	return "play"
}

func catalogStarter(row tileRow) string {
	if starter := strings.TrimSpace(row.Starter); starter != "" {
		return starter
	}
	if row.Planner == "scripted" {
		return "squirtle"
	}
	return "LLM picks"
}

func sanitizeCatalogRow(row tileRow) tileRow {
	// Raw exchanges, map sprites and trails are live observations. Keeping them
	// out of SQLite prevents the history index from becoming a second dump store.
	row.Raw = ""
	row.Sprites = nil
	row.Trail = nil
	row.ResumeProtected = false
	return row
}

func (c *runCatalog) upsert(row tileRow) error {
	row = sanitizeCatalogRow(row)
	data, err := json.Marshal(row)
	if err != nil {
		return err
	}
	_, err = c.db.Exec(`
INSERT INTO runs(run_id,status,planner,starter,goal,llm_profile,queued_at,ended_at,outcome,how,starter_facet,row_json)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(run_id) DO UPDATE SET
 status=excluded.status,
 planner=excluded.planner,
 starter=excluded.starter,
 goal=excluded.goal,
 llm_profile=excluded.llm_profile,
 queued_at=excluded.queued_at,
 ended_at=excluded.ended_at,
 outcome=excluded.outcome,
 how=excluded.how,
 starter_facet=excluded.starter_facet,
 row_json=excluded.row_json`,
		row.RunID, row.Status, row.Planner, row.Starter, row.Goal, row.LLMProfile,
		row.QueuedAt, row.EndedAt, catalogOutcome(row), catalogHow(row), catalogStarter(row), data)
	if err != nil {
		return fmt.Errorf("upsert run %s: %w", row.RunID, err)
	}
	return nil
}

func (c *runCatalog) get(runID string) (tileRow, bool, error) {
	var data []byte
	err := c.db.QueryRow(`SELECT row_json FROM runs WHERE run_id=?`, runID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return tileRow{}, false, nil
	}
	if err != nil {
		return tileRow{}, false, err
	}
	var row tileRow
	if err := json.Unmarshal(data, &row); err != nil {
		return tileRow{}, false, fmt.Errorf("decode catalog run %s: %w", runID, err)
	}
	return row, true, nil
}

func (c *runCatalog) delete(runID string) error {
	_, err := c.db.Exec(`DELETE FROM runs WHERE run_id=?`, runID)
	return err
}

func (w *Wall) ramRow(runID string) (tileRow, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.tiles[runID]
	if t == nil {
		return tileRow{}, false
	}
	return w.tileRowLocked(t), true
}

func (w *Wall) syncCatalogFromRAM(includeFinished bool) error {
	catalog := catalogFor(w)
	if catalog == nil {
		return nil
	}
	w.mu.Lock()
	rows := make([]tileRow, 0, len(w.tiles))
	for _, id := range w.order {
		t := w.tiles[id]
		if t == nil || (!includeFinished && t.Finished) {
			continue
		}
		rows = append(rows, w.tileRowLocked(t))
	}
	w.mu.Unlock()
	for _, row := range rows {
		if err := catalog.upsert(row); err != nil {
			return err
		}
	}
	return nil
}

func (w *Wall) evictCatalogFinished() {
	if catalogFor(w) == nil {
		return
	}
	w.mu.Lock()
	lineage := w.liveCheckpointLineageLocked()
	changed := false
	for id, t := range w.tiles {
		if t == nil || !t.Finished {
			continue
		}
		if _, keep := lineage[id]; keep {
			continue
		}
		delete(w.tiles, id)
		delete(w.cancel, id)
		w.order = removeID(w.order, id)
		changed = true
	}
	w.mu.Unlock()
	if changed {
		w.saveState()
	}
}

func (w *Wall) catalogSnapshotRun(runID string) (tileRow, bool) {
	catalog := catalogFor(w)
	if catalog == nil {
		return tileRow{}, false
	}
	row, ok, err := catalog.get(strings.TrimSpace(runID))
	if err != nil {
		log.Printf("pokewall: catalog get %s: %v", runID, err)
		return tileRow{}, false
	}
	return row, ok
}

// RunCatalogSettlementSweep catches terminal transitions produced by the stale
// worker reaper rather than an HTTP finish request. It deliberately ignores
// live heartbeat mutations, keeping heartbeats RAM-only.
func (w *Wall) RunCatalogSettlementSweep(interval time.Duration) {
	if catalogFor(w) == nil {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for range tick.C {
		catalog := catalogFor(w)
		if catalog == nil {
			return
		}
		w.mu.Lock()
		rows := make([]tileRow, 0)
		for _, id := range w.order {
			if t := w.tiles[id]; t != nil && t.Finished {
				rows = append(rows, w.tileRowLocked(t))
			}
		}
		w.mu.Unlock()
		ok := true
		for _, row := range rows {
			if err := catalog.upsert(row); err != nil {
				log.Printf("pokewall: catalog terminal sweep: %v", err)
				ok = false
				break
			}
		}
		if ok && len(rows) > 0 {
			w.evictCatalogFinished()
		}
	}
}

type catalogBufferedWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newCatalogBufferedWriter() *catalogBufferedWriter {
	return &catalogBufferedWriter{header: make(http.Header)}
}

func (w *catalogBufferedWriter) Header() http.Header { return w.header }
func (w *catalogBufferedWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *catalogBufferedWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}
func (w *catalogBufferedWriter) flush(dst http.ResponseWriter) {
	for key, values := range w.header {
		dst.Header()[key] = append([]string(nil), values...)
	}
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	dst.WriteHeader(status)
	_, _ = io.Copy(dst, &w.body)
}

func isCatalogLifecycleRequest(req *http.Request) bool {
	if req.Method != http.MethodPost {
		return false
	}
	if req.URL.Path == "/v1/specs" || req.URL.Path == "/v1/lease" {
		return true
	}
	return strings.HasPrefix(req.URL.Path, "/v1/runs/") && strings.HasSuffix(req.URL.Path, "/finish")
}

// catalogHTTPHandler is production-only. Compatibility/unit handlers remain
// RAM-only when -catalog is empty.
func (w *Wall) catalogHTTPHandler(next http.Handler) http.Handler {
	if catalogFor(w) == nil {
		return next
	}
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/dashboard":
			w.handleCatalogDashboard(res, req)
			return
		case req.Method == http.MethodGet && req.URL.Path == "/v1/outcomes":
			w.handleCatalogOutcomes(res)
			return
		case req.Method == http.MethodDelete && strings.HasPrefix(req.URL.Path, "/v1/runs/"):
			if w.handleCatalogOnlyDelete(res, req) {
				return
			}
		}

		if !isCatalogLifecycleRequest(req) {
			next.ServeHTTP(res, req)
			return
		}

		buffered := newCatalogBufferedWriter()
		next.ServeHTTP(buffered, req)
		if buffered.status >= 200 && buffered.status < 300 {
			if err := w.catalogAfterLifecycle(req); err != nil {
				writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "catalog update: " + err.Error()})
				return
			}
		}
		buffered.flush(res)
	})
}

func (w *Wall) catalogAfterLifecycle(req *http.Request) error {
	catalog := catalogFor(w)
	if catalog == nil {
		return nil
	}
	if req.URL.Path == "/v1/specs" || req.URL.Path == "/v1/lease" {
		return w.syncCatalogFromRAM(false)
	}
	id := req.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
		if len(parts) >= 3 {
			id = parts[2]
		}
	}
	row, ok := w.ramRow(id)
	if !ok {
		return nil
	}
	if err := catalog.upsert(row); err != nil {
		return err
	}
	if row.Status == statusDone {
		w.evictCatalogFinished()
	}
	return nil
}

func parseCatalogDashboardQuery(req *http.Request) (runtimeDashboardQuery, error) {
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
			return query, errors.New("limit must be a positive integer")
		}
		query.limit = limit
	}
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return query, errors.New("offset must be a non-negative integer")
		}
		query.offset = offset
	}
	return query, nil
}

func (w *Wall) handleCatalogDashboard(res http.ResponseWriter, req *http.Request) {
	query, err := parseCatalogDashboardQuery(req)
	if err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// The default dashboard is now the live control plane. Historical reads are
	// explicit and therefore paginatable/queryable from SQLite.
	history := query.status == statusDone || query.outcome != "" || query.how != "" || query.starter != "" || query.facets
	if query.active || !history {
		query.active = true
		writeJSON(res, http.StatusOK, w.runtimeDashboardSnapshot(query))
		return
	}
	view, err := w.catalogHistorySnapshot(query)
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "query catalog: " + err.Error()})
		return
	}
	writeJSON(res, http.StatusOK, view)
}

func (w *Wall) catalogHistorySnapshot(query runtimeDashboardQuery) (runtimeDashboardView, error) {
	catalog := catalogFor(w)
	if catalog == nil {
		return runtimeDashboardView{}, errors.New("catalog disabled")
	}
	where := []string{"status = ?"}
	args := []any{statusDone}
	if query.outcome != "" {
		where = append(where, "outcome = ?")
		args = append(args, query.outcome)
	}
	if query.how != "" {
		where = append(where, "how = ?")
		args = append(args, query.how)
	}
	if query.starter != "" {
		where = append(where, "starter_facet = ?")
		args = append(args, query.starter)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := catalog.db.QueryRow(`SELECT COUNT(*) FROM runs WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return runtimeDashboardView{}, err
	}

	selectSQL := `SELECT row_json FROM runs WHERE ` + whereSQL + ` ORDER BY ended_at DESC, queued_at DESC, run_id DESC`
	queryArgs := append([]any(nil), args...)
	if query.limit > 0 {
		selectSQL += ` LIMIT ? OFFSET ?`
		queryArgs = append(queryArgs, query.limit, query.offset)
	} else if query.offset > 0 {
		selectSQL += ` LIMIT -1 OFFSET ?`
		queryArgs = append(queryArgs, query.offset)
	}
	rows, err := catalog.db.Query(selectSQL, queryArgs...)
	if err != nil {
		return runtimeDashboardView{}, err
	}
	defer rows.Close()
	history := make([]tileRow, 0)
	if query.limit > 0 {
		history = make([]tileRow, 0, query.limit)
	}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return runtimeDashboardView{}, err
		}
		var row tileRow
		if err := json.Unmarshal(data, &row); err != nil {
			return runtimeDashboardView{}, err
		}
		if w.runArtifactsProtected(row.RunID) {
			row.ResumeProtected = true
		}
		history = append(history, row)
	}
	if err := rows.Err(); err != nil {
		return runtimeDashboardView{}, err
	}

	active := w.runtimeDashboardSnapshot(runtimeDashboardQuery{active: true})
	view := runtimeDashboardView{Now: time.Now().Unix(), WallVersion: w.Version, Runs: history, Workers: active.Workers, Total: total}
	if query.facets {
		facets, err := catalog.historyFacets()
		if err != nil {
			return runtimeDashboardView{}, err
		}
		view.Facets = facets
	}
	return view, nil
}

func (c *runCatalog) historyFacets() (*runtimeHistoryFacets, error) {
	load := func(column string) ([]string, error) {
		rows, err := c.db.Query(`SELECT DISTINCT `+column+` FROM runs WHERE status=? AND `+column+` <> '' ORDER BY `+column, statusDone)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, rows.Err()
	}
	outcomes, err := load("outcome")
	if err != nil {
		return nil, err
	}
	hows, err := load("how")
	if err != nil {
		return nil, err
	}
	starters, err := load("starter_facet")
	if err != nil {
		return nil, err
	}
	return &runtimeHistoryFacets{Outcomes: outcomes, Hows: hows, Starters: starters}, nil
}

func (w *Wall) handleCatalogOnlyDelete(res http.ResponseWriter, req *http.Request) bool {
	id := req.PathValue("id")
	if id == "" {
		parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
		if len(parts) >= 3 {
			id = parts[2]
		}
	}
	w.mu.Lock()
	_, inRAM := w.tiles[id]
	w.mu.Unlock()
	if inRAM {
		return false
	}
	catalog := catalogFor(w)
	if catalog == nil {
		return false
	}
	row, ok, err := catalog.get(id)
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return true
	}
	if !ok {
		return false
	}
	attempts := row.Attempts
	if attempts < 1 {
		attempts = 1
	}
	if err := w.deleteLocalRunArtifacts(id, attempts); err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "delete local run artifacts: " + err.Error()})
		return true
	}
	if err := catalog.delete(id); err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "delete catalog run: " + err.Error()})
		return true
	}
	writeJSON(res, http.StatusOK, map[string]bool{"deleted": true})
	return true
}

type catalogOutcomeRun struct {
	RunID      string         `json:"run_id"`
	Status     string         `json:"status"`
	Planner    string         `json:"planner"`
	Starter    string         `json:"starter"`
	Goal       string         `json:"goal"`
	LLMProfile string         `json:"llm_profile"`
	MaxRounds  int            `json:"max_rounds"`
	MaxFrames  int            `json:"max_frames"`
	Endless    bool           `json:"endless"`
	RandomSeed bool           `json:"random_seed"`
	Attempts   int            `json:"attempts"`
	Reason     string         `json:"reason"`
	Stats      *farm.LLMStats `json:"stats"`
	Player     *farm.Player   `json:"player"`
}

func outcomeRun(row tileRow) catalogOutcomeRun {
	return catalogOutcomeRun{
		RunID: row.RunID, Status: row.Status, Planner: row.Planner, Starter: row.Starter,
		Goal: row.Goal, LLMProfile: row.LLMProfile, MaxRounds: row.MaxRounds, MaxFrames: row.MaxFrames,
		Endless: row.Endless, RandomSeed: row.RandomSeed, Attempts: row.Attempts, Reason: row.Reason,
		Stats: row.Stats, Player: row.Player,
	}
}

// /v1/outcomes streams the narrow stats projection. The wall never builds a
// whole-farm []tileRow just to answer the stats page.
func (w *Wall) handleCatalogOutcomes(res http.ResponseWriter) {
	catalog := catalogFor(w)
	if catalog == nil {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "catalog disabled"})
		return
	}
	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(res, `{"runs":[`)
	first := true
	emit := func(row tileRow) bool {
		data, err := json.Marshal(outcomeRun(row))
		if err != nil {
			return false
		}
		if !first {
			_, _ = io.WriteString(res, ",")
		}
		first = false
		_, _ = res.Write(data)
		return true
	}

	rows, err := catalog.db.Query(`SELECT row_json FROM runs WHERE status=? ORDER BY ended_at, run_id`, statusDone)
	if err == nil {
		for rows.Next() {
			var data []byte
			var row tileRow
			if rows.Scan(&data) == nil && json.Unmarshal(data, &row) == nil {
				emit(row)
			}
		}
		rows.Close()
	}

	w.mu.Lock()
	live := make([]tileRow, 0, len(w.tiles))
	for _, id := range w.order {
		if t := w.tiles[id]; t != nil && !t.Finished {
			live = append(live, w.tileRowLocked(t))
		}
	}
	w.mu.Unlock()
	sort.Slice(live, func(i, j int) bool { return live[i].RunID < live[j].RunID })
	for _, row := range live {
		emit(row)
	}
	_, _ = io.WriteString(res, `]}`)
}

// expireCatalogArtifacts complements expireLocalArtifacts after finished tiles
// have left w.tiles. History remains in SQLite while old local dumps/checkpoint
// trees still obey the existing retention window.
func (w *Wall) expireCatalogArtifacts(now time.Time, maxAge time.Duration) error {
	catalog := catalogFor(w)
	if catalog == nil || maxAge <= 0 || w.dumpsDir == "" {
		return nil
	}
	cutoff := now.Add(-maxAge).Unix()
	rows, err := catalog.db.Query(`SELECT row_json FROM runs WHERE status=? AND ended_at > 0 AND ended_at < ?`, statusDone, cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()
	var expired []tileRow
	for rows.Next() {
		var data []byte
		var row tileRow
		if err := rows.Scan(&data); err != nil {
			return err
		}
		if err := json.Unmarshal(data, &row); err != nil {
			return err
		}
		expired = append(expired, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	w.mu.Lock()
	lineage := w.liveCheckpointLineageLocked()
	pendingRuns := make(map[string]struct{})
	for _, entry := range w.outbox {
		if entry.Status == outboxPending {
			pendingRuns[entry.RunID] = struct{}{}
		}
	}
	w.mu.Unlock()
	coordinator := objectiveReporterFor(w)
	coordinator.mu.Lock()
	pendingPaths := make(map[string]struct{}, len(coordinator.pending))
	for path := range coordinator.pending {
		pendingPaths[path] = struct{}{}
	}
	coordinator.mu.Unlock()

	var errs []error
	for _, row := range expired {
		if _, keep := lineage[row.RunID]; keep {
			continue
		}
		if _, pending := pendingRuns[row.RunID]; pending {
			continue
		}
		attempts := row.Attempts
		if attempts < 1 {
			attempts = 1
		}
		busy := false
		for _, path := range localFinishDumpPaths(w.dumpsDir, row.RunID, attempts) {
			if _, ok := pendingPaths[path]; ok {
				busy = true
				break
			}
		}
		if busy {
			continue
		}
		if err := w.deleteLocalFinishDumps(row.RunID, attempts); err != nil {
			errs = append(errs, err)
		}
		if err := w.deleteLocalCheckpointTree(row.RunID); err != nil {
			errs = append(errs, err)
		}
		if row.ReplayAvailable {
			row.ReplayAvailable = false
			if err := catalog.upsert(row); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
