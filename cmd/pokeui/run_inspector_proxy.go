package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// mountRunInspectorRoutes keeps the browser on one origin while preserving
// service ownership: pokewall answers metadata/debug/checkpoint repro reads and
// writes, pokereplay owns S3 bytes and derived video, and pokeui only
// allowlists/proxies the two surfaces. Destructive worker controls are also
// allowlisted here because this function is mounted only by the private
// operator surface, never by the public spectator relay.
func mountRunInspectorRoutes(mux *http.ServeMux, wallBase, replayBase string) {
	mux.HandleFunc("GET /v1/runs/{id}", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/debug", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/attempts", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/artifacts", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/checkpoints", proxy(wallBase, true))
	mux.HandleFunc("POST /v1/runs/{id}/repro", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/repro-source", proxy(wallBase, true))
	mux.HandleFunc("POST /v1/workers/{addr}/force-end", proxy(wallBase, false))
	mux.HandleFunc("GET /v1/media/render-jobs", proxy(wallBase, true))
	// Studio review remains operator-only: no matching spectator mount exists.
	// Dedicated thumbnail routes are attached below when replay is configured.
	mux.HandleFunc("POST /v1/media/render-jobs/{id}/retry", proxy(wallBase, false))
	mux.HandleFunc("POST /v1/media/render-jobs/{id}/cancel", proxy(wallBase, false))
	mux.HandleFunc("DELETE /v1/media/render-jobs/{id}", proxy(wallBase, false))

	replayBase = strings.TrimRight(strings.TrimSpace(replayBase), "/")
	if replayBase == "" {
		mux.HandleFunc("GET /v1/live/broadcast/config", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/artifacts/{name}/content", streamProxy(wallBase))
		mux.HandleFunc("GET /v1/runs/{id}/replay/status", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/replay/render", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/replay/video", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/replay/semantic", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/highlights/status", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/highlights/render", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/highlights/video", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/highlights/manifest", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/live/commentary/status", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/live/commentary/start", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/live/commentary/stop", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/live/status", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/live/stream.mjpeg", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/live/broadcast/status", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/live/broadcast/start", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/live/broadcast/stop", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/publish/youtube", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/publish/youtube", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/studio/publications", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/studio/publications", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/studio/edits", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/studio/edits/{hash}/render", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/studio/edits/{hash}/status", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/studio/edits/{hash}", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/studio/edits/{hash}/video", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/producer/assessments/{hash}", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/studio/publications/{candidate}/{action}", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/producer/assess", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/studio/packages", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/studio/packages", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/studio/packages/{hash}", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/studio/packages/{hash}/thumbnails", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/studio/packages/{hash}/thumbnails", replayUnavailable)
		mux.HandleFunc("GET /v1/runs/{id}/studio/packages/{hash}/thumbnails/{candidate}/image", replayUnavailable)
		mux.HandleFunc("POST /v1/runs/{id}/studio/packages/{hash}/thumbnails/{candidate}/select", replayUnavailable)
		return
	}
	mux.HandleFunc("GET /v1/live/broadcast/config", proxy(replayBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/artifacts/{name}/content", streamProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/replay/status", proxy(replayBase, true))
	mux.HandleFunc("POST /v1/runs/{id}/replay/render", proxy(replayBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/replay/video", streamProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/replay/semantic", streamProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/highlights/status", proxy(replayBase, true))
	mux.HandleFunc("POST /v1/runs/{id}/highlights/render", proxy(replayBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/highlights/video", streamProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/highlights/manifest", streamProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/live/commentary/status", proxy(replayBase, true))
	mux.HandleFunc("POST /v1/runs/{id}/live/commentary/start", proxy(replayBase, false))
	mux.HandleFunc("POST /v1/runs/{id}/live/commentary/stop", proxy(replayBase, false))
	mux.HandleFunc("GET /v1/runs/{id}/live/status", proxy(replayBase, true))
	mux.HandleFunc("GET /v1/runs/{id}/live/stream.mjpeg", streamProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/live/broadcast/status", proxy(replayBase, true))
	mux.HandleFunc("POST /v1/runs/{id}/live/broadcast/start", proxy(replayBase, false))
	mux.HandleFunc("POST /v1/runs/{id}/live/broadcast/stop", proxy(replayBase, false))
	mux.HandleFunc("GET /v1/runs/{id}/publish/youtube", proxy(replayBase, true))
	mux.HandleFunc("POST /v1/runs/{id}/publish/youtube", proxy(replayBase, false))
	mux.HandleFunc("GET /v1/runs/{id}/studio/publications", studioThumbnailProxy(replayBase))
	mux.HandleFunc("POST /v1/runs/{id}/studio/publications", studioThumbnailProxy(replayBase))
	mux.HandleFunc("POST /v1/runs/{id}/studio/edits", studioThumbnailProxy(replayBase))
	mux.HandleFunc("POST /v1/runs/{id}/studio/edits/{hash}/render", studioThumbnailProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/studio/edits/{hash}/status", studioThumbnailProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/studio/edits/{hash}", studioThumbnailProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/studio/edits/{hash}/video", streamProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/producer/assessments/{hash}", studioThumbnailProxy(replayBase))
	mux.HandleFunc("POST /v1/runs/{id}/studio/publications/{candidate}/{action}", studioThumbnailProxy(replayBase))
	// The image generation request may take longer than the five-second wall
	// proxy. All of these routes exist on the private operator surface only.
	mux.HandleFunc("POST /v1/runs/{id}/producer/assess", studioThumbnailProxy(replayBase))
	mux.HandleFunc("POST /v1/runs/{id}/studio/packages", studioThumbnailProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/studio/packages", studioThumbnailProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/studio/packages/{hash}", studioThumbnailProxy(replayBase))
	mux.HandleFunc("POST /v1/runs/{id}/studio/packages/{hash}/thumbnails", studioThumbnailProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/studio/packages/{hash}/thumbnails", studioThumbnailProxy(replayBase))
	mux.HandleFunc("GET /v1/runs/{id}/studio/packages/{hash}/thumbnails/{candidate}/image", studioThumbnailProxy(replayBase))
	mux.HandleFunc("POST /v1/runs/{id}/studio/packages/{hash}/thumbnails/{candidate}/select", studioThumbnailProxy(replayBase))
}

// studioThumbnailProxy is operator-only, uses the incoming cancellation
// context, and supports long image-provider calls without proxyTimeout.
func studioThumbnailProxy(base string) http.HandlerFunc {
	client := &http.Client{Timeout: 5 * time.Minute}
	return func(w http.ResponseWriter, r *http.Request) {
		up, err := http.NewRequestWithContext(r.Context(), r.Method, strings.TrimRight(base, "/")+r.URL.RequestURI(), r.Body)
		if err != nil {
			writeUnreachable(w)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "" {
			up.Header.Set("Content-Type", ct)
		}
		res, err := client.Do(up)
		if err != nil {
			writeUnreachable(w)
			return
		}
		defer res.Body.Close()
		w.Header().Set("Cache-Control", "no-store")
		for _, h := range []string{"Content-Type", "Content-Length", "X-Content-Type-Options"} {
			if v := res.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	}
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
		if err != nil {
			writeStreamProxyError(w)
			return
		}
		for _, name := range []string{"Range", "If-Range"} {
			if value := r.Header.Get(name); value != "" {
				up.Header.Set(name, value)
			}
		}
		res, err := client.Do(up)
		if err != nil {
			writeStreamProxyError(w)
			return
		}
		defer res.Body.Close()
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition", "ETag", "Last-Modified", "Cache-Control", "X-PokePilot-Live-FPS"} {
			if value := res.Header.Get(name); value != "" {
				w.Header().Set(name, value)
			}
		}
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
