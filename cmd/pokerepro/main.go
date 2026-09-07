// Command pokerepro materializes a farm checkpoint locally and optionally
// launches the current checkout of pokepilot from it. This is intentionally a
// developer tool: it never mutates the source run and never uploads local state.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const localFetchTimeout = 30 * time.Second

var unsafeLocalName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

type runInspectEnvelope struct {
	Run struct {
		Planner    string `json:"planner"`
		Goal       string `json:"goal"`
		LLMProfile string `json:"llm_profile"`
	} `json:"run"`
}

type reproManifest struct {
	Wall       string `json:"wall"`
	RunID      string `json:"run_id"`
	Attempt    int    `json:"attempt"`
	Checkpoint string `json:"checkpoint"`
	Goal       string `json:"goal,omitempty"`
	LLMProfile string `json:"llm_profile,omitempty"`
}

func main() {
	wall := flag.String("wall", "http://localhost:8080", "pokewall base URL")
	runID := flag.String("run", "", "source farm run ID")
	attempt := flag.Int("attempt", 0, "source attempt; 0 selects the latest attempt")
	checkpoint := flag.String("checkpoint", "latest", "checkpoint state name or latest")
	out := flag.String("out", "", "directory to materialize the checkpoint into")
	play := flag.Bool("play", false, "launch the current checkout from the materialized checkpoint")
	token := flag.String("token", os.Getenv("POKEPILOT_TOKEN"), "bearer token for an authenticated wall; defaults to $POKEPILOT_TOKEN, then the pokepilot MCP token in ~/.claude.json")
	flag.Parse()

	if strings.TrimSpace(*token) == "" {
		*token = claudeToken()
	}

	if strings.TrimSpace(*runID) == "" {
		log.Fatal("pokerepro: -run is required")
	}
	if *attempt < 0 {
		log.Fatal("pokerepro: -attempt cannot be negative")
	}

	ctx, cancel := context.WithTimeout(context.Background(), localFetchTimeout)
	defer cancel()
	transport := transportFor(*token)
	hc := &http.Client{Timeout: localFetchTimeout, Transport: transport}
	cp, err := fetchCheckpoint(ctx, hc, *wall, *runID, *attempt, *checkpoint)
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

	planner, goal, profile := inspectSource(ctx, *wall, *runID, transport)
	if planner != "" && planner != "llm" {
		log.Fatalf("pokerepro: source planner is %q; local checkpoint repro currently supports llm runs", planner)
	}
	manifest := reproManifest{
		Wall:       strings.TrimRight(*wall, "/"),
		RunID:      *runID,
		Attempt:    cp.Attempt,
		Checkpoint: cp.State.Name,
		Goal:       goal,
		LLMProfile: profile,
	}
	if data, err := json.MarshalIndent(manifest, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "repro.json"), append(data, '\n'), 0o644)
	}

	fmt.Printf("materialized %s attempt %d checkpoint %s\n", *runID, cp.Attempt, cp.State.Name)
	fmt.Printf("  state:     %s\n", statePath)
	fmt.Printf("  knowledge: %s\n", knowledgePath)
	// Launch through `make run-llm`, not `go run` directly: the model's API key
	// lives in .env / ~/.config/pokepilot/env, and that target is what sources
	// them. Bypassing it gets a 401 from the model server.
	playArgs := "-resume " + shellQuote(statePath)
	if goal != "" {
		playArgs += " -goal " + shellQuote(goal)
	}
	// Match the source run's endpoint routing; "auto" is GPU-first, which is
	// what makes a local replay bearable to sit through.
	if profile != "" {
		playArgs += " -llm-profile " + shellQuote(profile)
	}
	if !*play {
		fmt.Printf("\nrun current checkout with:\n  make run-llm ARGS=\"%s\"\n", playArgs)
		return
	}

	cmd := exec.Command("make", "run-llm", "ARGS="+playArgs)
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

// fetchCheckpoint assembles a checkpoint from the run inspector's read-only
// endpoints — the checkpoint listing plus two artifact downloads — instead of
// farm.Client.FetchCheckpoint. Deployed walls sit behind a proxy that only
// exposes the read-only subset, so GET /v1/runs/{id}/checkpoint is a 404 from
// outside the cluster while these three routes work everywhere.
func fetchCheckpoint(ctx context.Context, hc *http.Client, wall, runID string, attempt int, want string) (*farm.ResumeCheckpoint, error) {
	base := strings.TrimRight(wall, "/") + "/v1/runs/" + url.PathEscape(runID)
	listing := base + "/checkpoints"
	if attempt > 0 {
		listing += "?attempt=" + strconv.Itoa(attempt)
	}
	var index struct {
		Attempt     int `json:"attempt"`
		Checkpoints []struct {
			Name         string `json:"name"`
			Round        int    `json:"round"`
			Replayable   bool   `json:"replayable"`
			HasKnowledge bool   `json:"has_knowledge"`
		} `json:"checkpoints"`
	}
	if err := getJSON(ctx, hc, listing, &index); err != nil {
		return nil, err
	}

	state := ""
	for _, c := range index.Checkpoints {
		switch {
		case !c.Replayable || !c.HasKnowledge:
			continue
		case want == "" || want == "latest":
			if state == "" || c.Name > state { // names sort by round, zero-padded
				state = c.Name
			}
		case c.Name == want:
			state = c.Name
		}
	}
	if state == "" {
		return nil, fmt.Errorf("no replayable checkpoint %q with paired knowledge in attempt %d", want, index.Attempt)
	}

	var artifacts struct {
		Artifacts []struct {
			Name string `json:"name"`
		} `json:"artifacts"`
	}
	if err := getJSON(ctx, hc, base+"/artifacts", &artifacts); err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(state, ".state") + ".knowledge-v"
	knowledge := ""
	for _, a := range artifacts.Artifacts {
		if strings.HasPrefix(a.Name, prefix) && strings.HasSuffix(a.Name, ".json") && a.Name > knowledge {
			knowledge = a.Name
		}
	}
	if knowledge == "" {
		return nil, fmt.Errorf("checkpoint %s has no paired agent knowledge artifact", state)
	}

	stateData, err := fetchArtifact(ctx, hc, base, state)
	if err != nil {
		return nil, err
	}
	knowledgeData, err := fetchArtifact(ctx, hc, base, knowledge)
	if err != nil {
		return nil, err
	}
	return &farm.ResumeCheckpoint{
		Attempt:   index.Attempt,
		State:     farm.Artifact{Name: state, MediaType: "application/octet-stream", Data: stateData},
		Knowledge: &farm.Artifact{Name: knowledge, MediaType: "application/json", Data: knowledgeData},
	}, nil
}

func fetchArtifact(ctx context.Context, hc *http.Client, base, name string) ([]byte, error) {
	resp, err := get(ctx, hc, base+"/artifacts/"+url.PathEscape(name)+"/content")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func getJSON(ctx context.Context, hc *http.Client, endpoint string, into any) error {
	resp, err := get(ctx, hc, endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(into)
}

func get(ctx context.Context, hc *http.Client, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: status %d", endpoint, resp.StatusCode)
	}
	return resp, nil
}

// bearerAuth stamps Authorization on every request so pokerepro can talk to an
// authenticated wall, not just a local one.
type bearerAuth struct{ token string }

func (b bearerAuth) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(req)
}

func transportFor(token string) http.RoundTripper {
	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))
	if token == "" {
		return http.DefaultTransport
	}
	return bearerAuth{token: token}
}

// claudeToken reads the pokepilot MCP bearer out of ~/.claude.json, which is
// where it already lives for anyone triaging farm runs.
func claudeToken() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		MCPServers map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.MCPServers["pokepilot"].Headers["Authorization"]
}

func inspectSource(ctx context.Context, wall, runID string, transport http.RoundTripper) (planner, goal, profile string) {
	endpoint := strings.TrimRight(wall, "/") + "/v1/runs/" + url.PathEscape(runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", ""
	}
	client := &http.Client{Timeout: localFetchTimeout, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", ""
	}
	var envelope runInspectEnvelope
	if json.NewDecoder(resp.Body).Decode(&envelope) != nil {
		return "", "", ""
	}
	return envelope.Run.Planner, envelope.Run.Goal, envelope.Run.LLMProfile
}

// shellQuote makes one argument safe for the shell that make hands the recipe
// to. Goals contain spaces, so ARGS cannot be passed bare.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func safeLocal(s string) string {
	s = unsafeLocalName.ReplaceAllString(s, "_")
	if s == "" || s == "." || s == ".." {
		return "run"
	}
	return s
}
