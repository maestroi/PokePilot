// Command pokewall is the pokefarm wall: it queues run specs, leases them
// to pokepilot runners, tracks heartbeats and cooperative cancels, keeps
// the live tile grid at GET /, and stores durable finish dumps on disk.
// Standard library plus the farm package only — no emu, skill, agent,
// red, Docker, or Swarm.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
)

func main() {
	httpAddr := flag.String("http", "localhost:8080", "listen address for the wall HTTP API")
	dumpsDir := flag.String("dumps", "/var/lib/pokewall/dumps", "directory for durable finish dumps")
	flag.Parse()

	if err := os.MkdirAll(*dumpsDir, 0o755); err != nil {
		log.Fatalf("pokewall: cannot create dump directory %s: %v", *dumpsDir, err)
	}

	wall := NewWall(*dumpsDir)
	log.Printf("pokewall listening on http://%s (dumps in %s)", *httpAddr, *dumpsDir)
	if err := http.ListenAndServe(*httpAddr, wall.Handler()); err != nil {
		log.Fatalf("pokewall: server stopped: %v", err)
	}
}
