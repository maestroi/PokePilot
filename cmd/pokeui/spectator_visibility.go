package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"
)

const spectatorControlCacheTTL = time.Second

type wallSpectatorRunControl struct {
	Visible bool `json:"visible"`
}

type wallSpectatorControl struct {
	FeaturedRunID string                             `json:"featured_run_id,omitempty"`
	Runs          map[string]wallSpectatorRunControl `json:"runs"`
}

type spectatorControlCache struct {
	wallBase string
	client   *http.Client

	mu        sync.Mutex
	snapshot  wallSpectatorControl
	expiresAt time.Time
}

func newSpectatorControlCache(wallBase string) *spectatorControlCache {
	return &spectatorControlCache{
		wallBase: strings.TrimRight(strings.TrimSpace(wallBase), "/"),
		client:   &http.Client{Timeout: proxyTimeout},
	}
}

func (c *spectatorControlCache) get(ctx context.Context) (wallSpectatorControl, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.expiresAt) {
		return c.snapshot, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.wallBase+"/v1/spectator/control", nil)
	if err != nil {
		return wallSpectatorControl{}, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return wallSpectatorControl{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return wallSpectatorControl{}, fmt.Errorf("spectator control returned %s", resp.Status)
	}

	var snapshot wallSpectatorControl
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&snapshot); err != nil {
		return wallSpectatorControl{}, err
	}
	if snapshot.Runs == nil {
		snapshot.Runs = make(map[string]wallSpectatorRunControl)
	}
	c.snapshot = snapshot
	c.expiresAt = time.Now().Add(spectatorControlCacheTTL)
	return snapshot, nil
}

func spectatorVisibilityHTTPHandler(wallBase string, next http.Handler) http.Handler {
	cache := newSpectatorControlCache(wallBase)
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		protected := req.Method == http.MethodGet && (req.URL.Path == "/v1/watch" ||
			req.URL.Path == "/frame" ||
			req.URL.Path == "/render-state" ||
			strings.HasPrefix(req.URL.Path, "/v1/watch/runs/"))
		if !protected {
			next.ServeHTTP(res, req)
			return
		}

		control, err := cache.get(req.Context())
		if err != nil {
			writeSpectatorControlUnavailable(res)
			return
		}

		switch {
		case req.URL.Path == "/v1/watch":
			serveControlledSpectatorSnapshot(res, req, next, control)
			return
		case req.URL.Path == "/frame" || req.URL.Path == "/render-state":
			runID := strings.TrimSpace(req.URL.Query().Get("run"))
			if runID != "" && !spectatorRunVisible(control, runID) {
				http.NotFound(res, req)
				return
			}
		case strings.HasPrefix(req.URL.Path, "/v1/watch/runs/"):
			if runID, ok := spectatorRunIDFromPublicPath(req.URL.Path); ok && !spectatorRunVisible(control, runID) {
				http.NotFound(res, req)
				return
			}
		}
		next.ServeHTTP(res, req)
	})
}

func spectatorRunVisible(control wallSpectatorControl, runID string) bool {
	setting, ok := control.Runs[runID]
	return !ok || setting.Visible
}

func spectatorRunIDFromPublicPath(path string) (string, bool) {
	const prefix = "/v1/watch/runs/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	if split := strings.IndexByte(rest, '/'); split >= 0 {
		rest = rest[:split]
	}
	if rest == "" {
		return "", false
	}
	runID, err := url.PathUnescape(rest)
	if err != nil || strings.TrimSpace(runID) == "" {
		return "", false
	}
	return runID, true
}

func serveControlledSpectatorSnapshot(res http.ResponseWriter, req *http.Request, next http.Handler, control wallSpectatorControl) {
	recorder := httptest.NewRecorder()
	next.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		copyRecordedResponse(res, recorder)
		return
	}

	var source struct {
		Now  int64             `json:"now"`
		Runs []json.RawMessage `json:"runs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &source); err != nil {
		writeSpectatorControlUnavailable(res)
		return
	}

	filtered := make([]json.RawMessage, 0, len(source.Runs))
	var summary spectatorSummary
	featuredRunID := ""
	for _, raw := range source.Runs {
		var meta struct {
			RunID  string `json:"run_id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil || meta.RunID == "" {
			continue
		}
		// Queued runs have no gameplay/frame to watch yet. Keep them private to
		// the operator console until a worker has actually picked them up.
		if meta.Status == "queued" {
			continue
		}
		if !spectatorRunVisible(control, meta.RunID) {
			continue
		}
		if meta.RunID == control.FeaturedRunID {
			var decorated map[string]any
			if err := json.Unmarshal(raw, &decorated); err == nil {
				decorated["featured"] = true
				if encoded, err := json.Marshal(decorated); err == nil {
					raw = encoded
					featuredRunID = meta.RunID
				}
			}
		}
		filtered = append(filtered, raw)
		switch meta.Status {
		case "running", "leased":
			summary.Live++
		case "done":
			summary.Completed++
		}
	}

	payload := struct {
		Now           int64             `json:"now"`
		Runs          []json.RawMessage `json:"runs"`
		Summary       spectatorSummary  `json:"summary"`
		FeaturedRunID string            `json:"featured_run_id,omitempty"`
	}{
		Now:           source.Now,
		Runs:          filtered,
		Summary:       summary,
		FeaturedRunID: featuredRunID,
	}

	res.Header().Set("Cache-Control", "no-store")
	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(payload) //nolint:errcheck // best effort
}

func copyRecordedResponse(res http.ResponseWriter, recorder *httptest.ResponseRecorder) {
	for key, values := range recorder.Header() {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			res.Header().Add(key, value)
		}
	}
	res.WriteHeader(recorder.Code)
	_, _ = res.Write(recorder.Body.Bytes())
}

func writeSpectatorControlUnavailable(res http.ResponseWriter) {
	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-store")
	res.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(res).Encode(map[string]string{"error": "spectator control unavailable"}) //nolint:errcheck
}
