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
// A process started with -spectator mounts only these embedded assets plus two
// read-only data routes. It never mounts queue, cancel, delete, triage, MCP, or
// the raw dashboard contract.
//
//go:embed ui/watch.html
var watchHTML []byte

//go:embed ui/watch.js
var watchJS []byte

const spectatorHistoryLimit = 12

type spectatorDashboard struct {
	Now  int64          `json:"now"`
	Runs []spectatorRun `json:"runs"`
}

type spectatorRun struct {
	RunID     string          `json:"run_id"`
	Status    string          `json:"status"`
	Starter   string          `json:"starter,omitempty"`
	Dest      string          `json:"dest,omitempty"`
	Goal      string          `json:"goal,omitempty"`
	QueuedAt  int64           `json:"queued_at,omitempty"`
	EndedAt   int64           `json:"ended_at,omitempty"`
	Frame     uint64          `json:"frame"`
	Map       uint8           `json:"map"`
	X         uint8           `json:"x"`
	Y         uint8           `json:"y"`
	Decision  string          `json:"decision,omitempty"`
	StopSoFar string          `json:"stop_so_far,omitempty"`
	Stats     *spectatorStats `json:"stats,omitempty"`
	Player    *farm.Player    `json:"player,omitempty"`
	Attempts  int             `json:"attempts,omitempty"`
	Reason    string          `json:"reason,omitempty"`
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

func spectatorHandler(wallBase string) http.Handler {
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
	mux.HandleFunc("GET /v1/watch", spectatorSnapshot(wallBase))
	mux.HandleFunc("GET /frame", spectatorFrame(wallBase))
	return spectatorSecurityHeaders(mux)
}

func spectatorSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; script-src 'self'; style-src 'self' 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
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

func writeSpectatorUnavailable(res http.ResponseWriter) {
	res.Header().Set("Cache-Control", "no-store")
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(res).Encode(map[string]string{"error": "spectator feed unavailable"}) //nolint:errcheck // best effort
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
