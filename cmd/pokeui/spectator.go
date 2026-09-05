package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

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

const spectatorHistoryLimit = 12

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
	RunID     string           `json:"run_id"`
	Status    string           `json:"status"`
	Starter   string           `json:"starter,omitempty"`
	Dest      string           `json:"dest,omitempty"`
	Goal      string           `json:"goal,omitempty"`
	QueuedAt  int64            `json:"queued_at,omitempty"`
	EndedAt   int64            `json:"ended_at,omitempty"`
	Frame     uint64           `json:"frame"`
	Map       uint8            `json:"map"`
	X         uint8            `json:"x"`
	Y         uint8            `json:"y"`
	Decision  string           `json:"decision,omitempty"`
	StopSoFar string           `json:"stop_so_far,omitempty"`
	Stats     *spectatorStats  `json:"stats,omitempty"`
	Player    *farm.Player     `json:"player,omitempty"`
	Sprites   []farm.MapSprite `json:"sprites,omitempty"`
	Trail     [][2]uint8       `json:"trail,omitempty"`
	Attempts  int              `json:"attempts,omitempty"`
	Reason    string           `json:"reason,omitempty"`
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

type spectatorReplayStatus struct {
	RunID string `json:"run_id"`
	State string `json:"state"`
	Size  int64  `json:"size,omitempty"`
}

func spectatorHandler(wallBase string) http.Handler {
	return spectatorHandlerWithReplay(wallBase, "")
}

func spectatorHandlerWithReplay(wallBase, replayBase string) http.Handler {
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
	mux.HandleFunc("GET /v1/watch", spectatorSnapshot(wallBase))
	mux.HandleFunc("GET /frame", spectatorFrame(wallBase))

	replayBase = strings.TrimRight(strings.TrimSpace(replayBase), "/")
	if replayBase != "" {
		mux.HandleFunc("GET /v1/watch/runs/{id}/replay/status", spectatorReplayStatusHandler(replayBase))
		mux.HandleFunc("GET /v1/watch/runs/{id}/replay/video", spectatorReplayVideo(replayBase))
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

		var snapshot spectatorDashboard
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&snapshot); err != nil {
			writeSpectatorUnavailable(res)
			return
		}
		snapshot.Summary = summarizeSpectatorRuns(snapshot.Runs)
		snapshot.Runs = spectatorRuns(snapshot.Runs)

		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(snapshot) //nolint:errcheck // best effort
	}
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

func spectatorReplayStatusHandler(replayBase string) http.HandlerFunc {
	client := &http.Client{Timeout: proxyTimeout}
	return func(res http.ResponseWriter, req *http.Request) {
		runID := strings.TrimSpace(req.PathValue("id"))
		if runID == "" || len(runID) > 256 {
			http.NotFound(res, req)
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()
		up, err := http.NewRequestWithContext(ctx, http.MethodGet, replayBase+"/v1/runs/"+url.PathEscape(runID)+"/replay/status", nil)
		if err != nil {
			writeSpectatorReplayUnavailable(res, http.StatusBadGateway)
			return
		}
		resp, err := client.Do(up)
		if err != nil {
			writeSpectatorReplayUnavailable(res, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			http.NotFound(res, req)
			return
		}
		if resp.StatusCode != http.StatusOK {
			writeSpectatorReplayUnavailable(res, http.StatusBadGateway)
			return
		}
		var status spectatorReplayStatus
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&status); err != nil {
			writeSpectatorReplayUnavailable(res, http.StatusBadGateway)
			return
		}
		status.RunID = runID
		switch status.State {
		case "ready", "missing", "generating", "disabled", "error":
		default:
			status.State = "unavailable"
		}
		res.Header().Set("Cache-Control", "no-store")
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(status) //nolint:errcheck // best effort
	}
}

func spectatorReplayVideo(replayBase string) http.HandlerFunc {
	client := &http.Client{}
	return func(res http.ResponseWriter, req *http.Request) {
		runID := strings.TrimSpace(req.PathValue("id"))
		if runID == "" || len(runID) > 256 {
			http.NotFound(res, req)
			return
		}
		up, err := http.NewRequestWithContext(req.Context(), http.MethodGet, replayBase+"/v1/runs/"+url.PathEscape(runID)+"/replay/video", nil)
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

// spectatorRuns keeps every in-flight run and only a small tail of finished
// history. Besides keeping the public payload bounded, the copy step makes the
// filtering rule explicit at the trust boundary.
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
