// Command pokewall is the pokefarm wall: it queues run specs, leases them
// to pokepilot runners, tracks heartbeats and cooperative cancels, keeps
// GET /v1/dashboard for the operator console (and GET / as an in-network
// debug table), and stores durable finish dumps on disk. Standard library
// plus the farm package only — no emu, skill, agent, red, Docker, or Swarm.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverIdleTimeout       = 60 * time.Second
	serverShutdownTimeout   = 10 * time.Second
)

// version is this build's identity (git SHA), stamped by the Dockerfile via
// -ldflags "-X main.version=..."; "dev" for local builds.
var version = "dev"

func main() {
	httpAddr := flag.String("http", "localhost:8080", "listen address for the wall HTTP API")
	dumpsDir := flag.String("dumps", "/var/lib/pokewall/dumps", "directory for durable finish dumps")
	publishDir := flag.String("publish", "", "if set, also publish the dashboard (grid + live frames) to this directory for the browser-facing relay")
	publishEvery := flag.Duration("publish-every", 2*time.Second, "how often the published dashboard is refreshed")
	stateFile := flag.String("state", "", "if set, persist the tile map and queue here so a wall restart does not forget active runs")
	issuesAPI := flag.String("issues-api", "", "Agent Orchestrator API base (e.g. http://192.168.50.81:8080)")
	issuesProject := flag.String("issues-project", "", "Agent Orchestrator project UUID for PokePilot")
	issuesUI := flag.String("issues-ui", "", "Agent Orchestrator UI base (e.g. http://192.168.50.81:8081)")
	issuesTimeout := flag.Duration("issues-timeout", defaultIssueTimeout, "timeout for Agent Orchestrator issue HTTP calls")
	flag.Parse()

	if err := os.MkdirAll(*dumpsDir, 0o755); err != nil {
		log.Fatalf("pokewall: cannot create dump directory %s: %v", *dumpsDir, err)
	}

	wall := NewWall(*dumpsDir)
	wall.Version = version
	client, err := parseIssueFlags(*issuesAPI, *issuesProject, *issuesUI)
	if err != nil {
		log.Fatalf("pokewall: %v", err)
	}
	if client != nil {
		if *issuesTimeout > 0 {
			client.http.Timeout = *issuesTimeout
		}
		wall.SetIssueClient(client)
	}
	if *stateFile != "" {
		if err := os.MkdirAll(filepath.Dir(*stateFile), 0o755); err != nil {
			log.Fatalf("pokewall: cannot create state directory %s: %v", filepath.Dir(*stateFile), err)
		}
		wall.SetStatePath(*stateFile)
	}
	// The reaper runs whether or not state is persisted: a run whose runner
	// died must not sit "running" on the grid forever.
	go wall.RunReaper(5 * time.Second)
	if *publishDir != "" {
		if err := os.MkdirAll(filepath.Join(*publishDir, "live"), 0o755); err != nil {
			log.Fatalf("pokewall: cannot create publish directory %s: %v", *publishDir, err)
		}
		go wall.RunPublisher(*publishDir, *publishEvery)
		log.Printf("pokewall listening on http://%s (dumps in %s, publishing dashboard to %s every %s)",
			*httpAddr, *dumpsDir, *publishDir, *publishEvery)
	} else {
		log.Printf("pokewall listening on http://%s (dumps in %s)", *httpAddr, *dumpsDir)
	}

	server := &http.Server{
		Addr:              *httpAddr,
		Handler:           wall.Handler(),
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
			log.Fatalf("pokewall: server stopped: %v", err)
		}
	case <-ctx.Done():
		log.Printf("pokewall: shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("pokewall: graceful shutdown failed: %v", err)
			_ = server.Close()
		}
		cancel()
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("pokewall: server stopped during shutdown: %v", err)
		}
	}
}
