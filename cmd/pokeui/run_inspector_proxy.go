package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// mountRunInspectorRoutes keeps the browser on one origin while preserving
// service ownership: pokewall answers metadata/debug reads, pokereplay owns S3
// bytes and derived video, and pokeui only allowlists/proxies the two surfaces.
func mountRunInspectorRoutes(mux *http.ServeMux, wallBase, replayBase string) {
	mux.HandleFunc("GET /v1/runs/{id}", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/debug", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/artifacts", proxy(wallBase, true))

	replayBase = strings.TrimRight(strings.TrimSpace(replayBase), "/")
	if replayBase == "" {
		mux.HandleFunc("GET /v1/runs/{id}/artifacts/{name}/content", streamProxy(wallBase))
		mux.HandleFunc("GET /v1/runs/{id}/replay/status", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/replay/render", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/replay/video", replayUnavailable)
		return
	}
	mux.HandleFunc("GET /v1/runs/{id}/artifacts/{name}/content", streamProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/replay/status", proxy(replayBase, true))
	mux.HandleFunc("POST /v1/runs/{id}/replay/render", proxy(replayBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/replay/video", streamProxy(replayBase))
}

func replayUnavailable(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "replay service is not configured"})
}

// streamProxy has no five-second response timeout: video and large artifact
// downloads intentionally live as long as the browser request. The request
// context still cancels the upstream when the client disconnects.
func streamProxy(upstreamBase string) http.HandlerFunc {
	client := &http.Client{}
	upstreamBase = strings.TrimRight(upstreamBase, "/")
	return func(w http.ResponseWriter, r *http.Request) {
		up, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamBase+r.URL.RequestURI(), nil)
		if err != nil { writeStreamProxyError(w); return }
		for _, name := range []string{"Range", "If-Range"} { if value := r.Header.Get(name); value != "" { up.Header.Set(name, value) } }
		res, err := client.Do(up); if err != nil { writeStreamProxyError(w); return }
		defer res.Body.Close()
		for _, name := range []string{"Content-Type","Content-Length","Content-Range","Accept-Ranges","Content-Disposition","ETag","Last-Modified","Cache-Control"} { if value := res.Header.Get(name); value != "" { w.Header().Set(name, value) } }
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	}
}

func writeStreamProxyError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "artifact/replay service unreachable"})
}
