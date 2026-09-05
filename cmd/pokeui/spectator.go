package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

// The spectator surface is intentionally separate from the operator console.
// A process started with -spectator mounts only these embedded assets plus
// read-only data routes. It never mounts queue, cancel, delete, triage, MCP,
// raw dashboard, artifact browsing, or replay render routes.
//
//go:embed ui/watch.html
var watchHTML []byte

//go:embed ui/watch.js
var watchJS []byte

const (
	spectatorHistoryLimit   = 12
	spectatorReplayCacheTTL = 30 * time.Second
)

type spectatorDashboard struct {
	Now     int64            `json:"now"`
	Runs    []spectatorRun   `json:"runs"`
	Summary spectatorSummary `json:"summary"`
}

type spectatorSummary struct {
	Live      int `json:"live"`
	Queued    int `json:"queued"`
	Completed int `json:"completed"`
}

type spectatorRun struct {
	RunID       string           `json:"run_id"`
	Status      string           `json:"status"`
	Starter     string           `json:"starter,omitempty"`
	Dest        string           `json:"dest,omitempty"`
	Goal        string           `json:"goal,omitempty"`
	QueuedAt    int64            `json:"queued_at,omitempty"`
	EndedAt     int64            `json:"ended_at,omitempty"`
	Frame       uint64           `json:"frame"`
	Map         uint8            `json:"map"`
	X           uint8            `json:"x"`
	Y           uint8            `json:"y"`
	Decision    string           `json:"decision,omitempty"`
	StopSoFar   string           `json:"stop_so_far,omitempty"`
	Stats       *spectatorStats  `json:"stats,omitempty"`
	Player      *farm.Player     `json:"player,omitempty"`
	Sprites     []farm.MapSprite `json:"sprites,omitempty"`
	Trail       [][2]uint8       `json:"trail,omitempty"`
	Attempts    int              `json:"attempts,omitempty"`
	Reason      string           `json:"reason,omitempty"`
	ReplayReady bool             `json:"replay_ready,omitempty"`
	Highlight   string           `json:"highlight,omitempty"`
}

type spectatorStats struct {
	Round        int     `json:"round"`
	RoundsLeft   int     `json:"rounds_left"`
	Calls        int     `json:"calls"`
	Rounds       int     `json:"rounds"`
	Rejected     int     `json:"rejected"`
	Repeats      int     `json:"repeats"`
	LastSeconds  float64 `json:"last_seconds"`
	AvgSeconds   float64 `json:"avg_seconds"`
	GoalSummary  string  `json:"goal_summary,omitempty"`
	GoalCurrent  int     `json:"goal_current,omitempty"`
	GoalTarget   int     `json:"goal_target,omitempty"`
	GoalComplete bool    `json:"goal_complete,omitempty"`
}

// spectatorSourceRun includes private wall fields used only to decide whether a
// finished run is interesting enough for the public archive. It is never
// serialized to the browser. In particular, the issue payload must not cross
// the spectator trust boundary.
type spectatorSourceRun struct {
	spectatorRun
	Issue json.RawMessage `json:"issue,omitempty"`
}

type spectatorSourceDashboard struct {
	Now  int64                `json:"now"`
	Runs []spectatorSourceRun `json:"runs"`
}

type spectatorReplayStatus struct {
	RunID string `json:"run_id"`
	State string `json:"state"`
	Size  int64  `json:"size,omitempty"`
}

type spectatorReplayCacheEntry struct {
	Status    spectatorReplayStatus
	CheckedAt time.Time
}

// spectatorReplayCatalog keeps replay readiness cheap enough for the 2-second
// public watch poll and doubles as the server-side allowlist for public video
// reads. A run becomes public only after the wall says it is noteworthy and the
// replay sidecar confirms a cached MP4 is ready.
type spectatorReplayCatalog struct {
	replayBase string
	client     *http.Client

	mu      sync.RWMutex
	cache   map[string]spectatorReplayCacheEntry
	allowed map[string]struct{}
}

func newSpectatorReplayCatalog(replayBase string) *spectatorReplayCatalog {
	return &spectatorReplayCatalog{
		replayBase: strings.TrimRight(strings.TrimSpace(replayBase), "/"),
		client:     &http.Client{Timeout: proxyTimeout},
		cache:      make(map[string]spectatorReplayCacheEntry),
		allowed:    make(map[string]struct{}),
	}
}

func (c *spectatorReplayCatalog) enabled() bool {
	return c != nil && c.replayBase != ""
}

func (c *spectatorReplayCatalog) status(ctx context.Context, runID string) spectatorReplayStatus {
	if !c.enabled() {
		return spectatorReplayStatus{RunID: runID, State: "unavailable"}
	}
	now := time.Now()
	c.mu.RLock()
	cached, ok := c.cache[runID]
	c.mu.RUnlock()
	if ok && now.Sub(cached.CheckedAt) < spectatorReplayCacheTTL {
		return cached.Status
	}

	status := spectatorReplayStatus{RunID: runID, State: "unavailable"}
	up, err := http.NewRequestWithContext(ctx, http.MethodGet, c.replayBase+"/v1/runs/"+url.PathEscape(runID)+"/replay/status", nil)
	if err == nil {
		if resp, doErr := c.client.Do(up); doErr == nil {
			if resp.StatusCode == http.StatusOK {
				var decoded spectatorReplayStatus
				if decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&decoded); decodeErr == nil {
					decoded.RunID = runID
					switch decoded.State {
					case "ready", "missing", "generating", "disabled", "error":
						status = decoded
					}
				}
			}
			resp.Body.Close()
		}
	}

	c.mu.Lock()
	c.cache[runID] = spectatorReplayCacheEntry{Status: status, CheckedAt: now}
	c.mu.Unlock()
	return status
}

func (c *spectatorReplayCatalog) setAllowed(runIDs []string) {
	if c == nil {
		return
	}
	next := make(map[string]struct{}, len(runIDs))
	for _, runID := range runIDs {
		next[runID] = struct{}{}
	}
	c.mu.Lock()
	c.allowed = next
	c.mu.Unlock()
}

func (c *spectatorReplayCatalog) isAllowed(runID string) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	_, ok := c.allowed[runID]
	c.mu.RUnlock()
	return ok
}

func spectatorHandler(wallBase string) http.Handler {
	return spectatorHandlerWithReplay(wallBase, "")
}

func spectatorHandlerWithReplay(wallBase, replayBase string) http.Handler {
	catalog := newSpectatorReplayCatalog(replayBase)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(watchHTML) //nolint:errcheck // best effort: the page polls itself
	})
	mux.HandleFunc("GET /watch.js", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(watchJS) //nolint:errcheck // best effort
	})
	mountMaps(mux)
	mux.HandleFunc("GET /v1/watch", spectatorSnapshotWithReplay(wallBase, catalog))
	mux.HandleFunc("GET /frame", spectatorFrame(wallBase))

	if catalog.enabled() {
		mux.HandleFunc("GET /v1/watch/runs/{id}/replay/status", spectatorReplayStatusHandler(catalog))
		mux.HandleFunc("GET /v1/watch/runs/{id}/replay/video", spectatorReplayVideo(catalog))
	}
	return spectatorSecurityHeaders(mux)
}

func spectatorSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; media-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
		res.Header().Set("Referrer-Policy", "no-referrer")
		res.Header().Set("X-Content-Type-Options", "nosniff")
		res.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(res, req)
	})
}

func spectatorSnapshot(wallBase string) http.HandlerFunc {
	return spectatorSnapshotWithReplay(wallBase, newSpectatorReplayCatalog(""))
}

func spectatorSnapshotWithReplay(wallBase string, catalog *spectatorReplayCatalog) http.HandlerFunc {
	client := &http.Client{Timeout: proxyTimeout}
	wallBase = strings.TrimRight(wallBase, "/")
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()

		up, err := http.NewRequestWithContext(ctx, http.MethodGet, wallBase+"/v1/dashboard", nil)
		if err != nil {
			writeSpectatorUnavailable(res)
			return
		}
		resp, err := client.Do(up)
		if err != nil {
			writeSpectatorUnavailable(res)
			return
		}
		defer resp.Body.Close()

		res.Header().Set("Cache-Control", "no-store")
		if resp.StatusCode != http.StatusOK {
			writeSpectatorUnavailable(res)
			return
		}

		var source spectatorSourceDashboard
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&source); err != nil {
			writeSpectatorUnavailable(res)
			return
		}

		all := make([]spectatorRun, 0, len(source.Runs))
		for _, run := range source.Runs {
			all = append(all, run.spectatorRun)
		}
		snapshot := spectatorDashboard{
			Now:     source.Now,
			Summary: summarizeSpectatorRuns(all),
			Runs:    publicSpectatorRuns(ctx, source.Runs, catalog),
		}

		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(snapshot) //nolint:errcheck // best effort
	}
}

// publicSpectatorRuns keeps every in-flight run, but finished runs are a
// curated replay archive rather than raw history. A finished run must both be
// noteworthy and already have a cached video. Goal completion is inherently
// noteworthy; a linked engineering issue means the run was selected for
// analysis. Ordinary failures and recordings still remain available privately
// in the operator Run Inspector.
func publicSpectatorRuns(ctx context.Context, runs []spectatorSourceRun, catalog *spectatorReplayCatalog) []spectatorRun {
	active := make([]spectatorRun, 0, len(runs))
	for _, run := range runs {
		if run.Status != "done" {
			active = append(active, run.spectatorRun)
		}
	}

	if !catalog.enabled() {
		catalog.setAllowed(nil)
		return active
	}

	done := make([]spectatorRun, 0, spectatorHistoryLimit)
	allowed := make([]string, 0, spectatorHistoryLimit)
	for i := len(runs) - 1; i >= 0 && len(done) < spectatorHistoryLimit; i-- {
		run := runs[i]
		if run.Status != "done" {
			continue
		}
		highlight := spectatorHighlight(run)
		if highlight == "" {
			continue
		}
		status := catalog.status(ctx, run.RunID)
		if status.State != "ready" {
			continue
		}
		publicRun := run.spectatorRun
		publicRun.ReplayReady = true
		publicRun.Highlight = highlight
		done = append(done, publicRun)
		allowed = append(allowed, run.RunID)
	}

	// Restore chronological order for the wire contract; the browser can sort
	// presentation order independently.
	for left, right := 0, len(done)-1; left < right; left, right = left+1, right-1 {
		done[left], done[right] = done[right], done[left]
	}
	catalog.setAllowed(allowed)
	return append(active, done...)
}

func spectatorHighlight(run spectatorSourceRun) string {
	if run.Stats != nil && run.Stats.GoalComplete {
		return "goal complete"
	}
	switch strings.ToLower(strings.TrimSpace(run.Reason)) {
	case "goal", "goal_complete", "goal-complete":
		return "goal complete"
	}
	issue := strings.TrimSpace(string(run.Issue))
	if issue != "" && issue != "null" && issue != "{}" {
		return "analyzed"
	}
	return ""
}

func spectatorFrame(wallBase string) http.HandlerFunc {
	client := &http.Client{Timeout: proxyTimeout}
	wallBase = strings.TrimRight(wallBase, "/")
	return func(res http.ResponseWriter, req *http.Request) {
		runID := strings.TrimSpace(req.URL.Query().Get("run"))
		if runID == "" || len(runID) > 256 {
			http.NotFound(res, req)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()
		values := url.Values{"run": []string{runID}}
		up, err := http.NewRequestWithContext(ctx, http.MethodGet, wallBase+"/frame?"+values.Encode(), nil)
		if err != nil {
			writeSpectatorUnavailable(res)
			return
		}
		resp, err := client.Do(up)
		if err != nil {
			writeSpectatorUnavailable(res)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			http.NotFound(res, req)
			return
		}
		contentType := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			writeSpectatorUnavailable(res)
			return
		}
		res.Header().Set("Cache-Control", "no-store")
		res.Header().Set("Content-Type", contentType)
		io.Copy(res, resp.Body) //nolint:errcheck // response body closes with request
	}
}

func spectatorReplayStatusHandler(catalog *spectatorReplayCatalog) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		runID := strings.TrimSpace(req.PathValue("id"))
		if runID == "" || len(runID) > 256 || !catalog.isAllowed(runID) {
			http.NotFound(res, req)
			return
		}
		status := catalog.status(req.Context(), runID)
		if status.State != "ready" {
			writeSpectatorReplayUnavailable(res, http.StatusConflict)
			return
		}
		res.Header().Set("Cache-Control", "no-store")
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(status) //nolint:errcheck // best effort
	}
}

func spectatorReplayVideo(catalog *spectatorReplayCatalog) http.HandlerFunc {
	client := &http.Client{}
	return func(res http.ResponseWriter, req *http.Request) {
		runID := strings.TrimSpace(req.PathValue("id"))
		if runID == "" || len(runID) > 256 || !catalog.isAllowed(runID) {
			http.NotFound(res, req)
			return
		}
		up, err := http.NewRequestWithContext(req.Context(), http.MethodGet, catalog.replayBase+"/v1/runs/"+url.PathEscape(runID)+"/replay/video", nil)
		if err != nil {
			writeSpectatorReplayUnavailable(res, http.StatusBadGateway)
			return
		}
		for _, name := range []string{"Range", "If-Range"} {
			if value := req.Header.Get(name); value != "" {
				up.Header.Set(name, value)
			}
		}
		resp, err := client.Do(up)
		if err != nil {
			writeSpectatorReplayUnavailable(res, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			status := resp.StatusCode
			if status < 400 || status > 599 {
				status = http.StatusBadGateway
			}
			writeSpectatorReplayUnavailable(res, status)
			return
		}
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control"} {
			if value := resp.Header.Get(name); value != "" {
				res.Header().Set(name, value)
			}
		}
		res.Header().Set("Cache-Control", "public, max-age=60")
		res.WriteHeader(resp.StatusCode)
		io.Copy(res, resp.Body) //nolint:errcheck // streamed response
	}
}

func writeSpectatorReplayUnavailable(res http.ResponseWriter, status int) {
	res.Header().Set("Cache-Control", "no-store")
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(status)
	json.NewEncoder(res).Encode(map[string]string{"error": "replay unavailable"}) //nolint:errcheck // best effort
}

func writeSpectatorUnavailable(res http.ResponseWriter) {
	res.Header().Set("Cache-Control", "no-store")
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(res).Encode(map[string]string{"error": "spectator feed unavailable"}) //nolint:errcheck // best effort
}

func summarizeSpectatorRuns(runs []spectatorRun) spectatorSummary {
	var summary spectatorSummary
	for _, run := range runs {
		switch run.Status {
		case "running", "leased":
			summary.Live++
		case "queued":
			summary.Queued++
		case "done":
			summary.Completed++
		}
	}
	return summary
}

// spectatorRuns is retained as the pure history-bounding helper used by older
// tests and callers. Production public filtering happens in publicSpectatorRuns,
// which additionally requires a noteworthy run and a ready replay.
func spectatorRuns(runs []spectatorRun) []spectatorRun {
	active := make([]spectatorRun, 0, len(runs))
	done := make([]spectatorRun, 0, len(runs))
	for _, run := range runs {
		if run.Status == "done" {
			done = append(done, run)
			continue
		}
		active = append(active, run)
	}
	if len(done) > spectatorHistoryLimit {
		done = done[len(done)-spectatorHistoryLimit:]
	}
	return append(active, done...)
}
