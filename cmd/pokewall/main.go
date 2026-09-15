// Command pokewall is the pokefarm wall: it queues run specs, leases them to
// pokepilot runners, tracks heartbeats and cooperative cancels, keeps
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
	stateFile := flag.String("state", "", "if set, persist live tiles, queue, issue links and outbox here so a wall restart does not forget active runs")
	catalogPath := flag.String("catalog", "", "if set, persist the queryable run history in SQLite; finished tiles then leave RAM")
	artifactRetention := flag.Duration("artifact-retention", defaultArtifactRetention, "how long finished-run dumps/checkpoints are kept locally; <=0 disables automatic retention")
	artifactRetentionEvery := flag.Duration("artifact-retention-every", defaultArtifactSweepEvery, "how often local finished-run artifacts are expired")
	issuesAPI := flag.String("issues-api", "", "issue sink API base")
	issuesProject := flag.String("issues-project", "", "issue sink project key")
	issuesUI := flag.String("issues-ui", "", "issue UI base used for linked issue URLs")
	issuesTimeout := flag.Duration("issues-timeout", defaultIssueTimeout, "timeout for issue sink HTTP calls")
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
	if *stateFile != "" {
		if err := os.MkdirAll(filepath.Dir(*stateFile), 0o755); err != nil {
			log.Fatalf("pokewall: cannot create state directory %s: %v", filepath.Dir(*stateFile), err)
		}
		wall.SetStatePath(*stateFile)
	}
	if *catalogPath != "" {
		if err := wall.SetCatalogPath(*catalogPath); err != nil {
			log.Fatalf("pokewall: open catalog %s: %v", *catalogPath, err)
		}
		defer wall.CloseCatalog() //nolint:errcheck // process exit closes the file descriptor too
		go wall.RunCatalogSettlementSweep(5 * time.Second)
	}
	if client != nil {
		// Persisted remote IDs are only meaningful for the sink that minted them.
		// Switching from Agent Orchestrator to GitHub (or between repositories)
		// must not leave new occurrences quarantined against stale issue UUIDs.
		if removed := wall.reconcileIssueSink(*issuesUI); removed > 0 {
			log.Printf("pokewall: detached %d persisted issue link(s) from the previous sink", removed)
		}
		if *issuesTimeout > 0 {
			client.http.Timeout = *issuesTimeout
		}
		// Start the outbox/status loops only after state restore and sink
		// reconciliation so restart recovery uses one coherent identity set.
		wall.SetIssueClient(client)
		go wall.RunObjectiveFailureEvents(defaultObjectiveFailureReportEvery)
		go wall.RunIssueVerification(defaultIssueVerificationEvery)
	}
	// The reaper runs whether or not state is persisted: a run whose runner
	// died must not sit "running" on the grid forever.
	go wall.RunReaper(5 * time.Second)
	if *artifactRetention > 0 {
		go wall.RunArtifactRetention(*artifactRetentionEvery, *artifactRetention)
	}
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

	baseHandler := wall.outcomesCompatibility(wall.catalogOperatorCompatibility(wall.catalogHTTPHandler(workerControlHTTPHandler(wall, runtimeOperatorHTTPHandler(wall)))))
	server := &http.Server{
		Addr:              *httpAddr,
		Handler:           modelExperimentHTTPHandler(wall, baseHandler),
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
