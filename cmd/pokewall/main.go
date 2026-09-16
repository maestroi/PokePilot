// Command pokewall is the pokefarm wall: it queues run specs, leases them to
// pokepilot runners, tracks heartbeats and cooperative cancels, exposes the
// operator control plane, and indexes durable run metadata in PostgreSQL.
// Large artifacts remain outside the database; local dump/checkpoint paths are
// caches for compatibility and debugging, never the production source of truth.
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
	"strings"
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
	dumpsDir := flag.String("dumps", "/tmp/pokewall", "local cache/temp directory for finish/checkpoint artifacts")
	publishDir := flag.String("publish", "", "if set, also publish the dashboard (grid + live frames) to this directory for the browser-facing relay")
	publishEvery := flag.Duration("publish-every", 2*time.Second, "how often the published dashboard is refreshed")
	databaseURL := flag.String("database", strings.TrimSpace(os.Getenv("POKEPILOT_DATABASE_URL")), "PostgreSQL DSN for the production control plane (defaults to POKEPILOT_DATABASE_URL)")
	// These two flags remain only as an inexpensive local/test compatibility
	// path. Production deployment must use -database and must not combine the
	// stores, otherwise there would be two durable authorities again.
	stateFile := flag.String("state", "", "legacy local/dev state file; cannot be combined with -database")
	catalogPath := flag.String("catalog", "", "legacy local/dev SQLite history catalog; cannot be combined with -database")
	artifactRetention := flag.Duration("artifact-retention", defaultArtifactRetention, "how long local artifact caches are kept; <=0 disables automatic retention")
	artifactRetentionEvery := flag.Duration("artifact-retention-every", defaultArtifactSweepEvery, "how often local artifact caches are expired")
	issuesAPI := flag.String("issues-api", "", "issue sink API base")
	issuesProject := flag.String("issues-project", "", "issue sink project key")
	issuesUI := flag.String("issues-ui", "", "issue UI base used for linked issue URLs")
	issuesTimeout := flag.Duration("issues-timeout", defaultIssueTimeout, "timeout for issue sink HTTP calls")
	flag.Parse()

	if *databaseURL != "" && (*stateFile != "" || *catalogPath != "") {
		log.Fatal("pokewall: -database cannot be combined with legacy -state/-catalog persistence")
	}
	if err := os.MkdirAll(*dumpsDir, 0o755); err != nil {
		log.Fatalf("pokewall: cannot create local cache directory %s: %v", *dumpsDir, err)
	}

	wall := NewWall(*dumpsDir)
	wall.Version = version
	client, err := parseIssueFlags(*issuesAPI, *issuesProject, *issuesUI)
	if err != nil {
		log.Fatalf("pokewall: %v", err)
	}

	postgresMode := strings.TrimSpace(*databaseURL) != ""
	if postgresMode {
		if err := wall.SetControlPlaneDatabase(*databaseURL); err != nil {
			log.Fatalf("pokewall: open PostgreSQL control plane: %v", err)
		}
		if cp := controlPlaneFor(wall); cp != nil {
			if err := cp.migrateCheckpointArtifacts(); err != nil {
				log.Fatalf("pokewall: migrate PostgreSQL checkpoint storage: %v", err)
			}
			if err := cp.migrateSpectatorControl(); err != nil {
				log.Fatalf("pokewall: migrate spectator controls: %v", err)
			}
			if err := cp.rebuildFinishCache(wall); err != nil {
				log.Fatalf("pokewall: rebuild finish cache from PostgreSQL: %v", err)
			}
		}
		defer wall.CloseControlPlane() //nolint:errcheck
		go wall.RunCatalogSettlementSweep(5 * time.Second)
		go wall.RunControlPlaneSweep(2 * time.Second)
	} else {
		if *stateFile != "" {
			if err := os.MkdirAll(filepath.Dir(*stateFile), 0o755); err != nil {
				log.Fatalf("pokewall: cannot create state directory %s: %v", filepath.Dir(*stateFile), err)
			}
			wall.SetStatePath(*stateFile)
		}
		if *catalogPath != "" {
			if err := wall.SetCatalogPath(*catalogPath); err != nil {
				log.Fatalf("pokewall: open local catalog %s: %v", *catalogPath, err)
			}
			defer wall.CloseCatalog() //nolint:errcheck
			go wall.RunCatalogSettlementSweep(5 * time.Second)
		}
	}

	if client != nil {
		// Persisted remote IDs are only meaningful for the sink that minted them.
		// Switching sinks must not quarantine new occurrences against stale IDs.
		if removed := wall.reconcileIssueSink(*issuesUI); removed > 0 {
			log.Printf("pokewall: detached %d persisted issue link(s) from the previous sink", removed)
		}
		if *issuesTimeout > 0 {
			client.http.Timeout = *issuesTimeout
		}
		wall.SetIssueClient(client)
		if postgresMode {
			// PostgreSQL is the objective-failure outbox in production. Never scan
			// historical local dump files to invent work after a clean DB reset.
			go wall.RunControlPlaneObjectiveFailures(defaultObjectiveFailureReportEvery)
		} else {
			go wall.RunObjectiveFailureEvents(defaultObjectiveFailureReportEvery)
		}
		go wall.RunIssueVerification(defaultIssueVerificationEvery)
	}

	go wall.RunReaper(5 * time.Second)
	if *artifactRetention > 0 {
		go wall.RunArtifactRetention(*artifactRetentionEvery, *artifactRetention)
	}
	if *publishDir != "" {
		if err := os.MkdirAll(filepath.Join(*publishDir, "live"), 0o755); err != nil {
			log.Fatalf("pokewall: cannot create publish directory %s: %v", *publishDir, err)
		}
		go wall.RunPublisher(*publishDir, *publishEvery)
		log.Printf("pokewall listening on http://%s (local cache %s, publishing dashboard to %s every %s)",
			*httpAddr, *dumpsDir, *publishDir, *publishEvery)
	} else {
		log.Printf("pokewall listening on http://%s (local cache %s)", *httpAddr, *dumpsDir)
	}

	// In PostgreSQL mode use the compatibility operator wrapper directly. The
	// old runtime wrapper's finish-dump event queue performs a startup directory
	// scan; production objective failure delivery now comes from the DB outbox.
	operator := http.Handler(runtimeOperatorHTTPHandler(wall))
	if postgresMode {
		operator = operatorHTTPHandler(wall)
	}
	// Pause/resume must sit inside the catalog/control-plane wrappers so those
	// persistence layers observe the final paused transition, not the temporary
	// cancelled/queued state used to cooperate with existing runners.
	operator = pauseHTTPHandler(wall, operator)
	baseHandler := wall.outcomesCompatibility(wall.catalogOperatorCompatibility(wall.catalogHTTPHandler(workerControlHTTPHandler(wall, operator))))
	modelHandler := controlPlaneModelExperimentHTTPHandler(wall, baseHandler)
	handler := archiveHTTPHandler(wall, modelHandler)
	if postgresMode {
		handler = wall.controlPlaneCheckpointHTTPHandler(wall.controlPlaneHTTPHandler(handler))
	}
	handler = spectatorControlHTTPHandler(wall, handler)
	if postgresMode {
		handler = wall.controlPlaneFrameHTTPHandler(handler)
	}
	server := &http.Server{
		Addr:              *httpAddr,
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("pokewall: server stopped: %v", err)
		}
	case <-ctx.Done():
		log.Printf("pokewall: shutting down")
		if cp := controlPlaneFor(wall); cp != nil {
			if err := cp.persistExperimentController(wall); err != nil {
				log.Printf("pokewall: final experiment persist: %v", err)
			}
			if err := cp.persistWall(wall); err != nil {
				log.Printf("pokewall: final control-plane persist: %v", err)
			}
		}
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
