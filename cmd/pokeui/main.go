// Command pokeui is the browser-facing frontend for a pokefarm wall: it
// serves an embedded operator console and proxies an allowlisted set of
// wall routes. The wall stays on the swarm overlay with no host ports;
// pokeui is the process the browser can reach.
package main

import (
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

// mapFiles holds build-time semantic map exports. The directory is kept in
// the repository even before a local ROM owner generates the JSON assets.
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
// not configure MCP. Production main calls handlerWithMCP with the token from
// POKEPILOT_MCP_TOKEN.
func handler(wallBase string) http.Handler {
	return handlerWithMCP(wallBase, "")
}

// handlerWithMCP serves the console at GET / and forwards only the operator
// routes to wallBase. Runner-only paths (lease, heartbeat, finish) 404. MCP is
// mounted at /mcp only when token is non-empty; an unset secret means the
// remote control plane does not exist rather than existing anonymously.
func handlerWithMCP(wallBase, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(indexHTML) //nolint:errcheck // best effort: the page polls itself
	})
	mux.HandleFunc("GET /ui.js", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(uiJS) //nolint:errcheck // best effort
	})
	if maps, err := fs.Sub(mapFiles, "ui/maps"); err == nil {
		mux.HandleFunc("GET /maps/{name}", func(res http.ResponseWriter, req *http.Request) {
			name := req.PathValue("name")
			if data, err := fs.ReadFile(maps, name); err == nil {
				http.ServeContent(res, req, name, time.Time{}, strings.NewReader(string(data)))
				return
			}
			// A deployment may intentionally omit ROM-derived semantic assets.
			// Keep the live overlay usable instead of making the entire map panel
			// disappear: return a neutral coordinate field for map JSON requests.
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
	mux.HandleFunc("GET /v1/version", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(res).Encode(map[string]string{"version": version}) //nolint:errcheck
	})
	mux.HandleFunc("GET /v1/dashboard", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/triage", proxy(wallBase, true))
	mux.HandleFunc("POST /v1/specs", proxy(wallBase, false))
	mux.HandleFunc("POST /v1/triage/{key}/investigate", proxy(wallBase, false))
	mux.HandleFunc("POST /v1/runs/{id}/cancel", proxy(wallBase, false))
	mux.HandleFunc("DELETE /v1/runs/{id}", proxy(wallBase, false))
	mux.HandleFunc("GET /frame", proxy(wallBase, true))
	if token = strings.TrimSpace(token); token != "" {
		mux.Handle("/mcp", newMCPHandler(wallBase, token))
	}
	return mux
}

// proxy copies one request to the wall and writes the upstream response.
// noStore forces Cache-Control so the browser never keeps a stale grid or
// screen. A dead wall is 502, not a hung socket.
func proxy(wallBase string, noStore bool) http.HandlerFunc {
	client := &http.Client{Timeout: proxyTimeout}
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()
		up, err := http.NewRequestWithContext(ctx, req.Method, wallBase+req.URL.RequestURI(), req.Body)
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
	flag.Parse()
	if *wall == "" {
		log.Fatal("pokeui: -wall is required")
	}

	wallBase := strings.TrimRight(*wall, "/")
	mcpToken := strings.TrimSpace(os.Getenv("POKEPILOT_MCP_TOKEN"))
	if mcpToken == "" {
		log.Printf("pokeui proxying %s on http://%s (MCP disabled)", *wall, *httpAddr)
	} else {
		log.Printf("pokeui proxying %s on http://%s (MCP enabled at /mcp)", *wall, *httpAddr)
	}
	server := &http.Server{
		Addr:              *httpAddr,
		Handler:           handlerWithMCP(wallBase, mcpToken),
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
