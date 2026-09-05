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

var version = "dev"

//go:embed ui/index.html
var indexHTML []byte

//go:embed ui/ui.js
var uiJS []byte

//go:embed ui/stats.js
var statsJS []byte

//go:embed ui/inspector.js
var inspectorJS []byte

var goalInputHTML = []byte(`<label class="llm-only goal-field">goal <input name="goal" value="Earn the Boulder Badge." autocomplete="off"></label>`)
var goalPresetHTML = []byte(`<label class="llm-only goal-field">goal <select name="goal"><option value="Earn the Boulder Badge." selected>Earn the Boulder Badge</option><option value="Earn 2 badges.">Earn 2 badges</option><option value="Earn 3 badges.">Earn 3 badges</option><option value="Earn 4 badges.">Earn 4 badges</option><option value="Earn 5 badges.">Earn 5 badges</option><option value="Earn 6 badges.">Earn 6 badges</option><option value="Earn 7 badges.">Earn 7 badges</option><option value="Earn all 8 badges.">Earn all 8 badges</option><option value="Beat the Elite Four and Champion.">Beat the Elite Four + Champion</option><option value="">Free play (no automatic stop)</option></select></label>`)

//go:embed ui/maps
var mapFiles embed.FS

const (
	proxyTimeout = 5 * time.Second
	fallbackMapSize = 128
	serverReadHeaderTimeout = 5 * time.Second
	serverIdleTimeout = 60 * time.Second
	serverShutdownTimeout = 10 * time.Second
)

func handler(wallBase string) http.Handler { return handlerWithServices(wallBase, "", "") }

func operatorIndexPage() []byte {
	page := bytes.Replace(indexHTML, goalInputHTML, goalPresetHTML, 1)
	extra := []byte("<script src=\"/stats.js\"></script>\n<script src=\"/inspector.js\"></script>\n</body>")
	return bytes.Replace(page, []byte("</body>"), extra, 1)
}

func handlerWithMCP(wallBase, token string) http.Handler { return handlerWithServices(wallBase, "", token) }

// handlerWithServices serves the private operator console and forwards only
// allowlisted operator/debug routes. Runner-only paths remain unreachable.
func handlerWithServices(wallBase, replayBase, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(res http.ResponseWriter, req *http.Request) { res.Header().Set("Content-Type", "text/html; charset=utf-8"); res.Header().Set("Cache-Control", "no-store"); _, _ = res.Write(operatorIndexPage()) })
	mux.HandleFunc("GET /ui.js", func(res http.ResponseWriter, req *http.Request) { res.Header().Set("Content-Type", "text/javascript; charset=utf-8"); res.Header().Set("Cache-Control", "no-store"); _, _ = res.Write(uiJS) })
	mux.HandleFunc("GET /stats.js", func(res http.ResponseWriter, req *http.Request) { res.Header().Set("Content-Type", "text/javascript; charset=utf-8"); res.Header().Set("Cache-Control", "no-store"); _, _ = res.Write(statsJS) })
	mux.HandleFunc("GET /inspector.js", func(res http.ResponseWriter, req *http.Request) { res.Header().Set("Content-Type", "text/javascript; charset=utf-8"); res.Header().Set("Cache-Control", "no-store"); _, _ = res.Write(inspectorJS) })
	mountMaps(mux)
	mux.HandleFunc("GET /v1/version", func(res http.ResponseWriter, req *http.Request) { res.Header().Set("Content-Type", "application/json"); res.Header().Set("Cache-Control", "no-store"); _ = json.NewEncoder(res).Encode(map[string]string{"version":version}) })
	mux.HandleFunc("GET /v1/dashboard", proxy(wallBase, true))
	mux.HandleFunc("GET /v1/stats", statsHandler(wallBase))
	mux.HandleFunc("GET /v1/triage", proxy(wallBase, true))
	mux.HandleFunc("POST /v1/specs", proxy(wallBase, false))
	mux.HandleFunc("POST /v1/triage/{key}/investigate", proxy(wallBase, false))
	mux.HandleFunc("POST /v1/runs/{id}/cancel", proxy(wallBase, false))
	mux.HandleFunc("DELETE /v1/runs/{id}", proxy(wallBase, false))
	mux.HandleFunc("GET /frame", proxy(wallBase, true))
	mountRunInspectorRoutes(mux, wallBase, replayBase)
	if token = strings.TrimSpace(token); token != "" { mux.Handle("/mcp", newMCPHandler(wallBase, token)) }
	return mux
}

func mountMaps(mux *http.ServeMux) {
	maps, err := fs.Sub(mapFiles, "ui/maps"); if err != nil { return }
	mux.HandleFunc("GET /maps/{name}", func(res http.ResponseWriter, req *http.Request) {
		name := req.PathValue("name")
		if data, err := fs.ReadFile(maps, name); err == nil { http.ServeContent(res, req, name, time.Time{}, strings.NewReader(string(data))); return }
		if len(name) == len("00.json") && strings.HasSuffix(name, ".json") {
			res.Header().Set("Content-Type", "application/json"); res.Header().Set("Cache-Control", "no-store"); cells := strings.Repeat(".", fallbackMapSize*fallbackMapSize)
			_ = json.NewEncoder(res).Encode(map[string]any{"width":fallbackMapSize,"height":fallbackMapSize,"cells":cells,"fallback":true}); return
		}
		http.NotFound(res, req)
	})
}

func proxy(upstreamBase string, noStore bool) http.HandlerFunc {
	client := &http.Client{Timeout:proxyTimeout}; upstreamBase = strings.TrimRight(upstreamBase, "/")
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout); defer cancel()
		up, err := http.NewRequestWithContext(ctx, req.Method, upstreamBase+req.URL.RequestURI(), req.Body); if err != nil { writeUnreachable(res); return }
		if ct:=req.Header.Get("Content-Type");ct!=""{up.Header.Set("Content-Type",ct)}
		resp, err := client.Do(up); if err != nil { writeUnreachable(res); return }; defer resp.Body.Close()
		if noStore { res.Header().Set("Cache-Control","no-store") }; if ct:=resp.Header.Get("Content-Type");ct!=""{res.Header().Set("Content-Type",ct)}; res.WriteHeader(resp.StatusCode); _,_=io.Copy(res,resp.Body)
	}
}

func writeUnreachable(res http.ResponseWriter){res.Header().Set("Content-Type","application/json");res.WriteHeader(http.StatusBadGateway);_=json.NewEncoder(res).Encode(map[string]string{"error":"wall unreachable"})}

func main(){
	httpAddr:=flag.String("http",":8080","listen address for the relay");wall:=flag.String("wall","","upstream wall URL (e.g. http://wall:8080)");replay:=flag.String("replay","","optional replay service URL (e.g. http://replay:8080)");spectator:=flag.Bool("spectator",false,"serve only the public read-only spectator surface");flag.Parse();if *wall==""{log.Fatal("pokeui: -wall is required")}
	wallBase:=strings.TrimRight(*wall,"/");replayBase:=strings.TrimRight(*replay,"/");mcpToken:=strings.TrimSpace(os.Getenv("POKEPILOT_MCP_TOKEN"));var httpHandler http.Handler
	if *spectator{httpHandler=spectatorHandler(wallBase);log.Printf("pokeui proxying %s on http://%s (public spectator mode; read-only)",*wall,*httpAddr)}else{httpHandler=handlerWithServices(wallBase,replayBase,mcpToken);log.Printf("pokeui proxying %s on http://%s (MCP=%t, replay=%t)",*wall,*httpAddr,mcpToken!="",replayBase!="")}
	server:=&http.Server{Addr:*httpAddr,Handler:httpHandler,ReadHeaderTimeout:serverReadHeaderTimeout,IdleTimeout:serverIdleTimeout};ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop();errCh:=make(chan error,1);go func(){errCh<-server.ListenAndServe()}()
	select{case err:=<-errCh:if err!=nil&&!errors.Is(err,http.ErrServerClosed){log.Fatalf("pokeui: server stopped: %v",err)};case <-ctx.Done():log.Printf("pokeui: shutting down");shutdownCtx,cancel:=context.WithTimeout(context.Background(),serverShutdownTimeout);if err:=server.Shutdown(shutdownCtx);err!=nil{log.Printf("pokeui: graceful shutdown failed: %v",err);_=server.Close()};cancel();if err:=<-errCh;err!=nil&&!errors.Is(err,http.ErrServerClosed){log.Fatalf("pokeui: server stopped during shutdown: %v",err)}}
}
