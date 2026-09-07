// Command pokerepro materializes a farm checkpoint locally and optionally
// launches the current checkout of pokepilot from it. This is intentionally a
// developer tool: it never mutates the source run and never uploads local state.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const localFetchTimeout = 30 * time.Second

var unsafeLocalName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

type runInspectEnvelope struct {
	Run struct {
		Planner string `json:"planner"`
		Goal    string `json:"goal"`
	} `json:"run"`
}

type reproManifest struct {
	Wall       string `json:"wall"`
	RunID      string `json:"run_id"`
	Attempt    int    `json:"attempt"`
	Checkpoint string `json:"checkpoint"`
	Goal       string `json:"goal,omitempty"`
}

func main() {
	wall := flag.String("wall", "http://localhost:8080", "pokewall base URL")
	runID := flag.String("run", "", "source farm run ID")
	attempt := flag.Int("attempt", 0, "source attempt; 0 selects the latest attempt")
	checkpoint := flag.String("checkpoint", "latest", "checkpoint state name or latest")
	out := flag.String("out", "", "directory to materialize the checkpoint into")
	play := flag.Bool("play", false, "launch the current checkout from the materialized checkpoint")
	flag.Parse()

	if strings.TrimSpace(*runID) == "" {
		log.Fatal("pokerepro: -run is required")
	}
	if *attempt < 0 {
		log.Fatal("pokerepro: -attempt cannot be negative")
	}

	ctx, cancel := context.WithTimeout(context.Background(), localFetchTimeout)
	defer cancel()
	client := farm.NewClient(*wall)
	cp, err := client.FetchCheckpoint(ctx, *runID, *attempt, *checkpoint)
	if err != nil {
		log.Fatalf("pokerepro: fetch checkpoint: %v", err)
	}
	if cp.Knowledge == nil {
		log.Fatalf("pokerepro: checkpoint %s has no paired agent knowledge", cp.State.Name)
	}

	dir := *out
	if dir == "" {
		dir = "repro-" + safeLocal(*runID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("pokerepro: create %s: %v", dir, err)
	}
	statePath := filepath.Join(dir, cp.State.Name)
	knowledgePath := filepath.Join(dir, cp.Knowledge.Name)
	if err := os.WriteFile(statePath, cp.State.Data, 0o644); err != nil {
		log.Fatalf("pokerepro: write state: %v", err)
	}
	if err := os.WriteFile(knowledgePath, cp.Knowledge.Data, 0o644); err != nil {
		log.Fatalf("pokerepro: write knowledge: %v", err)
	}

	planner, goal := inspectSource(ctx, *wall, *runID)
	if planner != "" && planner != "llm" {
		log.Fatalf("pokerepro: source planner is %q; local checkpoint repro currently supports llm runs", planner)
	}
	manifest := reproManifest{
		Wall:       strings.TrimRight(*wall, "/"),
		RunID:      *runID,
		Attempt:    cp.Attempt,
		Checkpoint: cp.State.Name,
		Goal:       goal,
	}
	if data, err := json.MarshalIndent(manifest, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "repro.json"), append(data, '\n'), 0o644)
	}

	fmt.Printf("materialized %s attempt %d checkpoint %s\n", *runID, cp.Attempt, cp.State.Name)
	fmt.Printf("  state:     %s\n", statePath)
	fmt.Printf("  knowledge: %s\n", knowledgePath)
	args := []string{"run", "./cmd/pokepilot", "-planner", "llm", "-resume", statePath}
	if goal != "" {
		args = append(args, "-goal", goal)
	}
	if !*play {
		fmt.Printf("\nrun current checkout with:\n  go")
		for _, arg := range args {
			fmt.Printf(" %q", arg)
		}
		fmt.Println()
		return
	}

	cmd := exec.Command("go", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		log.Fatalf("pokerepro: launch pokepilot: %v", err)
	}
}

func inspectSource(ctx context.Context, wall, runID string) (planner, goal string) {
	endpoint := strings.TrimRight(wall, "/") + "/v1/runs/" + url.PathEscape(runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", ""
	}
	client := &http.Client{Timeout: localFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	var envelope runInspectEnvelope
	if json.NewDecoder(resp.Body).Decode(&envelope) != nil {
		return "", ""
	}
	return envelope.Run.Planner, envelope.Run.Goal
}

func safeLocal(s string) string {
	s = unsafeLocalName.ReplaceAllString(s, "_")
	if s == "" || s == "." || s == ".." {
		return "run"
	}
	return s
}
