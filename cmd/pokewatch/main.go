package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	nodeMode := flag.Bool("node", false, "run the per-node reporter instead of the watcher")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	token := operatorapi.ReadSecretFile(os.Getenv("POKEPILOT_OPS_TOKEN_FILE"))
	if token == "" {
		log.Fatal("pokewatch: POKEPILOT_OPS_TOKEN_FILE is required")
	}
	if *nodeMode {
		runNode(ctx, token)
		return
	}
	runWatch(ctx, token)
}

func runNode(ctx context.Context, token string) {
	host := env("NODE_NAME", "")
	if host == "" {
		host, _ = os.Hostname()
	}
	root := env("POKEWATCH_HOST_ROOT", "/host")
	mounts := strings.Split(env("POKEWATCH_MOUNTS", "/"), ",")
	ledger := env("POKEPILOT_FIXER_LEDGER", "/opt/pokefixer/state/ledger.tsv")
	ladder := env("POKEPILOT_TRIAGE_LADDER", "opencode:2,cursor:2,cursor/claude-opus-5-5-high:2")
	paidCap := envInt("POKEPILOT_PAID_DAILY_CAP", 20)
	target := env("POKEWATCH_URL", "http://watch:8080") + "/v1/node-report"
	client := &http.Client{Timeout: 20 * time.Second}
	tick := time.NewTicker(envDuration("POKEWATCH_NODE_INTERVAL", 5*time.Minute))
	defer tick.Stop()
	for {
		r := nodeReport(host, root, mounts, ledger, ladder, paidCap, time.Now())
		if err := operatorapi.PostOps(ctx, client, target, token, r); err != nil {
			log.Printf("pokewatch node: report: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, err := time.ParseDuration(env(key, "")); err == nil && v > 0 {
		return v
	}
	return fallback
}
