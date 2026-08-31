// Command pokeui is the browser-facing frontend for a pokefarm wall: it
// serves an embedded operator console and proxies an allowlisted set of
// wall routes. The wall stays on the swarm overlay with no host ports;
// pokeui is the process the browser can reach.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"time"

	_ "embed"
)

// version is this build's identity (git SHA), stamped by the Dockerfile via
// -ldflags "-X main.version=..."; "dev" for local builds.
var version = "dev"

//go:embed ui/index.html
var indexHTML []byte

//go:embed ui/ui.js
var uiJS []byte

const proxyTimeout = 5 * time.Second

// handler serves the console at GET / and forwards only the operator
// routes to wallBase. Runner-only paths (lease, heartbeat, finish) 404.
func handler(wallBase string) http.Handler {
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

	log.Printf("pokeui proxying %s on http://%s", *wall, *httpAddr)
	if err := http.ListenAndServe(*httpAddr, handler(*wall)); err != nil {
		log.Fatalf("pokeui: server stopped: %v", err)
	}
}
