// Command pokeui is the browser-facing frontend for a pokefarm wall: it
// serves an embedded operator console and proxies an allowlisted set of
// wall routes. The wall stays on the swarm overlay with no host ports;
// pokeui is the process the browser can reach.
package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// version is this build's identity (git SHA), stamped by the Dockerfile via
// -ldflags "-X main.version=..."; "dev" for local builds.
var version = "dev"

//go:embed ui/index.html
var indexHTML []byte

//go:embed ui/ui.js
var uiJS []byte

//go:embed ui/stats.js
var statsJS []byte

//go:embed ui/inspector.js
var inspectorJS []byte

// The operator used to expose Goal as an unrestricted text box even though
// only structured syntax had a deterministic stop condition. Keep the prompt
// human-readable, but constrain normal UI runs to the finite presets the agent
// can prove complete. External/API callers can still send structured goals or
// arbitrary prompt-only prose explicitly.
var goalInputHTML = []byte(`<label class="llm-only goal-field">goal <input name="goal" value="Earn the Boulder Badge." autocomplete="off"></label>`)

var goalPresetHTML = []byte(`<label class="llm-only goal-field">goal <select name="goal"><option value="Earn the Boulder Badge." selected>Earn the Boulder Badge</option><option value="Earn 2 badges.">Earn 2 badges</option><option value="Earn 3 badges.">Earn 3 badges</option><option value="Earn 4 badges.">Earn 4 badges</option><option value="Earn 5 badges.">Earn 5 badges</option><option value="Earn 6 badges.">Earn 6 badges</option><option value="Earn 7 badges.">Earn 7 badges</option><option value="Earn all 8 badges.">Earn all 8 badges</option><option value="Beat the Elite Four and Champion.">Beat the Elite Four + Champion</option><option value="">Free play (no automatic stop)</option></select></label>`)

// mapFiles holds build-time semantic map JSON used by the operator console.
//
//go:embed ui/maps
var mapFiles embed.FS

const (
	proxyTimeout            = 5 * time.Second
	fallbackMapSize         = 128
	serverReadHeaderTimeout = 5 * time.Second
	serverIdleTimeout       = 60 * time.Second
	serverShutdownTimeout   = 10 * time.Second
)

// handler is the browser-only default used by tests and local callers that do
// not configure MCP or replay. Production main supplies both optional services.
func handler(wallBase string) http.Handler {
	return handlerWithServices(wallBase, "", "")
}

func operatorIndexPage() []byte {
	page := bytes.Replace(indexHTML, goalInputHTML, goalPresetHTML, 1)
	extra := []byte("<script src=\"/stats.js\"></script>\n<script src=\"/inspector.js\"></script>\n</body>")
	return bytes.Replace(page, []byte("</body>"), extra, 1)
}

// handlerWithMCP preserves the test/local entrypoint used before replay was a
// separate service. Production uses handlerWithServices below.
func handlerWithMCP(wallBase, token string) http.Handler {
	return handlerWithServices(wallBase, "", token)
}

// handlerWithServices serves the private operator console and forwards only
// allowlisted operator/debug routes. Runner-only paths (lease, heartbeat,
// finish) remain unreachable. Replay is optional: without a replayBase the
// run/debug/artifact catalog still works and replay endpoints answer 503.
func handlerWithServices(wallBase, replayBase, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(operatorIndexPage()) //nolint:errcheck // best effort: the page polls itself
	})
	mux.HandleFunc("GET /ui.js", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(uiJS) //nolint:errcheck // best effort
	})
	mux.HandleFunc("GET /stats.js", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(statsJS) //nolint:errcheck // best effort
	})
	mux.HandleFunc("GET /inspector.js", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(inspectorJS) //nolint:errcheck // best effort
	})
	mountMaps(mux)
	mux.HandleFunc("GET /v1/version", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(res).Encode(map[string]string{"version": version}) //nolint:errcheck
	})
	mux.HandleFunc("GET /v1/dashboard", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/stats", statsHandler(wallBase))
	mux.HandleFunc("GET /v1/triage", proxy(wallBase, true))
	mux.HandleFunc("POST /v1/specs", proxy(wallBase, false))
	mux.HandleFunc("POST /v1/triage/{key}/investigate", proxy(wallBase, false))
	mux.HandleFunc("POST /v1/runs/{id}/cancel", proxy(wallBase, false))
	mux.HandleFunc("DELETE /v1/runs/{id}", proxy(wallBase, false))
	mux.HandleFunc("GET /frame", proxy(wallBase, true))
	mountRunInspectorRoutes(mux, wallBase, replayBase)
	if token = strings.TrimSpace(token); token != "" {
		mux.Handle("/mcp", newMCPHandler(wallBase, token))
	}
	return mux
}

// mountMaps serves the embedded semantic map JSON used by both the operator
// console and the public spectator overlay. Missing ROM exports still get a
// fallback grid so the live position marker has somewhere to sit.
func mountMaps(mux *http.ServeMux) {
	maps, err := fs.Sub(mapFiles, "ui/maps")
	if err != nil {
		return
	}
	mux.HandleFunc("GET /maps/{name}", func(res http.ResponseWriter, req *http.Request) {
		name := req.PathValue("name")
		if data, err := fs.ReadFile(maps, name); err == nil {
			http.ServeContent(res, req, name, time.Time{}, strings.NewReader(string(data)))
			return
		}
		if len(name) == len("00.json") && strings.HasSuffix(name, ".json") {
			res.Header().Set("Content-Type", "application/json")
			res.Header().Set("Cache-Control", "no-store")
			cells := strings.Repeat(".", fallbackMapSize*fallbackMapSize)
			json.NewEncoder(res).Encode(map[string]any{
				"width":    fallbackMapSize,
				"height":   fallbackMapSize,
				"cells":    cells,
				"fallback": true,
			}) //nolint:errcheck // best effort
			return
		}
		http.NotFound(res, req)
	})
}

// proxy copies one request to an upstream service and writes the response.
// noStore forces Cache-Control so the browser never keeps stale operator data.
// A dead upstream is 502, not a hung socket.
func proxy(upstreamBase string, noStore bool) http.HandlerFunc {
	client := &http.Client{Timeout: proxyTimeout}
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()
		up, err := http.NewRequestWithContext(ctx, req.Method, strings.TrimRight(upstreamBase, "/")+req.URL.RequestURI(), req.Body)
		if err != nil {
			writeUnreachable(res)
			return
		}
		if ct := req.Header.Get("Content-Type"); ct != "" {
			up.Header.Set("Content-Type", ct)
		}
		resp, err := client.Do(up)
		if err != nil {
			writeUnreachable(res)
			return
		}
		defer resp.Body.Close()
		if noStore {
			res.Header().Set("Cache-Control", "no-store")
		}
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			res.Header().Set("Content-Type", ct)
		}
		res.WriteHeader(resp.StatusCode)
		io.Copy(res, resp.Body) //nolint:errcheck // response body closes with the request
	}
}

func writeUnreachable(res http.ResponseWriter) {
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(res).Encode(map[string]string{"error": "wall unreachable"}) //nolint:errcheck // best effort
}

func main() {
	httpAddr := flag.String("http", ":8080", "listen address for the relay")
	wall := flag.String("wall", "", "upstream wall URL (e.g. http://wall:8080)")
	replay := flag.String("replay", "", "optional replay service URL (e.g. http://replay:8080)")
	spectator := flag.Bool("spectator", false, "serve only the public read-only spectator surface")
	flag.Parse()
	if *wall == "" {
		log.Fatal("pokeui: -wall is required")
	}

	wallBase := strings.TrimRight(*wall, "/")
	replayBase := strings.TrimRight(*replay, "/")
	mcpToken := strings.TrimSpace(os.Getenv("POKEPILOT_MCP_TOKEN"))
	var httpHandler http.Handler
	if *spectator {
		httpHandler = spectatorHandlerWithReplay(wallBase, replayBase)
		log.Printf("pokeui proxying %s on http://%s (public spectator mode; read-only; replay=%t)", *wall, *httpAddr, replayBase != "")
	} else {
		httpHandler = handlerWithServices(wallBase, replayBase, mcpToken)
		log.Printf("pokeui proxying %s on http://%s (MCP=%t, replay=%t)", *wall, *httpAddr, mcpToken != "", replayBase != "")
	}
	server := &http.Server{
		Addr:              *httpAddr,
		Handler:           httpHandler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("pokeui: server stopped: %v", err)
		}
	case <-ctx.Done():
		log.Printf("pokeui: shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("pokeui: graceful shutdown failed: %v", err)
			_ = server.Close()
		}
		cancel()
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("pokeui: server stopped during shutdown: %v", err)
		}
	}
}
