package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const runDeleteTimeout = 30 * time.Second

// deleteRunHandler makes operator deletion a two-phase cleanup. The replay
// service owns S3 credentials, so it purges run-owned remote artifacts first;
// only after that succeeds does pokeui remove the run from pokewall history.
// If cleanup fails, the history row is deliberately retained so the operator
// can retry instead of losing the only catalog entry that names the objects.
func deleteRunHandler(wallBase, replayBase string) http.HandlerFunc {
	wallDelete := proxy(wallBase, false)
	replayBase = strings.TrimRight(strings.TrimSpace(replayBase), "/")
	if replayBase == "" {
		// Local/small installs without the replay sidecar preserve the existing
		// wall-only behavior. They cannot have sidecar-managed S3 artifacts.
		return wallDelete
	}

	client := &http.Client{Timeout: runDeleteTimeout}
	return func(res http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		endpoint := replayBase + "/v1/runs/" + url.PathEscape(id) + "/artifacts"
		up, err := http.NewRequestWithContext(req.Context(), http.MethodDelete, endpoint, nil)
		if err != nil {
			writeDeleteError(res, http.StatusBadGateway, "build replay cleanup request: "+err.Error())
			return
		}
		cleanup, err := client.Do(up)
		if err != nil {
			writeDeleteError(res, http.StatusBadGateway, "replay service unreachable")
			return
		}
		defer cleanup.Body.Close()
		if cleanup.StatusCode < 200 || cleanup.StatusCode >= 300 {
			copyDeleteHeaders(res.Header(), cleanup.Header, "Content-Type", "Cache-Control")
			res.WriteHeader(cleanup.StatusCode)
			_, _ = io.Copy(res, cleanup.Body)
			return
		}
		_, _ = io.Copy(io.Discard, cleanup.Body)

		wallDelete(res, req)
	}
}

func copyDeleteHeaders(dst, src http.Header, names ...string) {
	for _, name := range names {
		if value := src.Get(name); value != "" {
			dst.Set(name, value)
		}
	}
}

func writeDeleteError(res http.ResponseWriter, status int, message string) {
	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-store")
	res.WriteHeader(status)
	_ = json.NewEncoder(res).Encode(map[string]string{"error": message})
}
