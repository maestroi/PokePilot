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
	wallBase = strings.TrimRight(strings.TrimSpace(wallBase), "/")
	replayBase = strings.TrimRight(strings.TrimSpace(replayBase), "/")
	if replayBase == "" {
		// Local/small installs without the replay sidecar preserve the existing
		// wall-only behavior.
		return wallDelete
	}

	client := &http.Client{Timeout: runDeleteTimeout}
	return func(res http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		escapedID := url.PathEscape(id)

		// Do the same finished-run guard as pokewall before touching storage.
		// This matters when a previously finished run ID has been re-queued:
		// its old dump can still name S3 artifacts even though the current run
		// is active, and DELETE must remain a side-effect-free 409 in that case.
		inspectReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, wallBase+"/v1/runs/"+escapedID, nil)
		if err != nil {
			writeDeleteError(res, http.StatusBadGateway, "build wall inspection request: "+err.Error())
			return
		}
		inspect, err := client.Do(inspectReq)
		if err != nil {
			writeDeleteError(res, http.StatusBadGateway, "wall unreachable")
			return
		}
		if inspect.StatusCode < 200 || inspect.StatusCode >= 300 {
			defer inspect.Body.Close()
			copyDeleteHeaders(res.Header(), inspect.Header, "Content-Type", "Cache-Control")
			res.WriteHeader(inspect.StatusCode)
			_, _ = io.Copy(res, inspect.Body)
			return
		}
		var view struct {
			Run struct {
				Status string `json:"status"`
			} `json:"run"`
		}
		decodeErr := json.NewDecoder(inspect.Body).Decode(&view)
		inspect.Body.Close()
		if decodeErr != nil {
			writeDeleteError(res, http.StatusBadGateway, "decode wall run status: "+decodeErr.Error())
			return
		}
		if view.Run.Status != "done" {
			writeDeleteError(res, http.StatusConflict, "run still active: "+id)
			return
		}

		endpoint := replayBase + "/v1/runs/" + escapedID + "/artifacts"
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
