// Command pokewall is the pokefarm wall: it queues run specs, leases them
// to pokepilot runners, tracks heartbeats and cooperative cancels, keeps
// GET /v1/dashboard for the operator console (and GET / as an in-network
// debug table), and stores durable finish dumps on disk. Standard library
// plus the farm package only — no emu, skill, agent, red, Docker, or Swarm.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	httpAddr := flag.String("http", "localhost:8080", "listen address for the wall HTTP API")
	dumpsDir := flag.String("dumps", "/var/lib/pokewall/dumps", "directory for durable finish dumps")
	publishDir := flag.String("publish", "", "if set, also publish the dashboard (grid + live frames) to this directory for the browser-facing relay")
	publishEvery := flag.Duration("publish-every", 2*time.Second, "how often the published dashboard is refreshed")
	stateFile := flag.String("state", "", "if set, persist the tile map and queue here so a wall restart does not forget active runs")
	flag.Parse()

	if err := os.MkdirAll(*dumpsDir, 0o755); err != nil {
		log.Fatalf("pokewall: cannot create dump directory %s: %v", *dumpsDir, err)
	}

	wall := NewWall(*dumpsDir)
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
	if err := http.ListenAndServe(*httpAddr, wall.Handler()); err != nil {
		log.Fatalf("pokewall: server stopped: %v", err)
	}
}
