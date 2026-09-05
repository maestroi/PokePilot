package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/maestroi/pokepilot/farm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpWallTimeout      = 5 * time.Second
	mcpMaxResponseBytes = 2 << 20
	mcpMaxRuns          = 100
)

var mcpRunSequence atomic.Uint64

// mcpControl is deliberately an operator client, not another orchestrator.
// It only speaks the same allowlisted wall API that pokeui exposes to a human;
// lease, heartbeat, finish, checkpoint, worker registration and Docker/Swarm
// controls are not reachable through MCP.
type mcpControl struct {
	wallBase string
	http     *http.Client
}

type mcpStartRunInput struct {
	Planner    string `json:"planner,omitempty" jsonschema:"planner mode: llm or scripted; defaults to llm"`
	Starter    string `json:"starter,omitempty" jsonschema:"starter Pokemon: squirtle, charmander, or bulbasaur; defaults to squirtle"`
	Dest       string `json:"dest,omitempty" jsonschema:"destination for scripted mode"`
	Goal       string `json:"goal,omitempty" jsonschema:"task statement for llm mode; defaults to earning the Boulder Badge"`
	Seed       int64  `json:"seed,omitempty" jsonschema:"deterministic run seed; zero is the bit-identical baseline"`
	FPS        int    `json:"fps,omitempty" jsonschema:"emulation pace; zero runs flat out"`
	MaxRounds  int    `json:"max_rounds,omitempty" jsonschema:"optional emergency/experiment LLM objective cap; zero means no hard round cap"`
	MaxFrames  int    `json:"max_frames,omitempty" jsonschema:"emulated frame budget; zero uses the runner default"`
	LLMProfile string `json:"llm_profile,omitempty" jsonschema:"llm endpoint routing: default, gpu, or auto (GPU primary with LAN fallback)"`
}

type mcpStartRunOutput struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type mcpListRunsInput struct {
	Status string `json:"status,omitempty" jsonschema:"optional status filter: queued, leased, running, or done"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum runs to return; defaults to 20 and is capped at 100"`
}

type mcpRunInput struct {
	RunID string `json:"run_id" jsonschema:"PokePilot run id"`
}

type mcpInvestigateInput struct {
	Key string `json:"key" jsonschema:"triage failure key returned by pokepilot_get_triage"`
}

type mcpRunView struct {
	RunID      string         `json:"run_id"`
	Status     string         `json:"status"`
	Planner    string         `json:"planner,omitempty"`
	Starter    string         `json:"starter,omitempty"`
	Dest       string         `json:"dest,omitempty"`
	Goal       string         `json:"goal,omitempty"`
	LLMProfile string         `json:"llm_profile,omitempty"`
	Seed       int64          `json:"seed"`
	FPS        int            `json:"fps"`
	MaxRounds  int            `json:"max_rounds"`
	MaxFrames  int            `json:"max_frames"`
	QueuedAt   int64          `json:"queued_at,omitempty"`
	EndedAt    int64          `json:"ended_at,omitempty"`
	Attempts   int            `json:"attempts"`
	Frame      uint64         `json:"frame"`
	Map        uint8          `json:"map"`
	X          uint8          `json:"x"`
	Y          uint8          `json:"y"`
	Trace      string         `json:"trace,omitempty"`
	Question   string         `json:"question,omitempty"`
	Decision   string         `json:"decision,omitempty"`
	StopSoFar  string         `json:"stop_so_far,omitempty"`
	Stats      *farm.LLMStats `json:"stats,omitempty"`
	Player     *farm.Player   `json:"player,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Detail     string         `json:"detail,omitempty"`
}

type mcpWorkerView struct {
	Addr    string `json:"addr"`
	RunID   string `json:"run_id,omitempty"`
	SeenAgo string `json:"seen_ago,omitempty"`
	Version string `json:"version,omitempty"`
}

type mcpDashboard struct {
	Now     int64           `json:"now"`
	Runs    []mcpRunView    `json:"runs"`
	Workers []mcpWorkerView `json:"workers"`
}

func newMCPHandler(wallBase, token string) http.Handler {
	control := &mcpControl{
		wallBase: strings.TrimRight(wallBase, "/"),
		http:     &http.Client{Timeout: mcpWallTimeout},
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "pokepilot",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_start_run",
		Description: "Queue one goal-driven PokePilot run and return its generated run id. Defaults to an LLM Squirtle run for the Boulder Badge.",
	}, control.startRun)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_list_runs",
		Description: "List recent PokePilot runs, optionally filtered by lifecycle status.",
	}, control.listRuns)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_get_run",
		Description: "Get the live or finished state of one PokePilot run, including planner state, party, location and LLM statistics when available.",
	}, control.getRun)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_cancel_run",
		Description: "Request cooperative cancellation of one queued or active PokePilot run.",
	}, control.cancelRun)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_get_triage",
		Description: "Get grouped run failures that PokePilot considers useful for investigation.",
	}, control.getTriage)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_investigate_failure",
		Description: "Trigger the existing PokePilot investigation handoff for one triage failure key.",
	}, control.investigateFailure)

	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		PropagateRequestCancellation: true,
		MaxRequestBodyBytes:          1 << 20,
	})

	// MCP's Streamable HTTP security guidance requires Origin validation for
	// remote HTTP servers. Non-browser MCP clients normally send no Origin;
	// browser cross-site requests are rejected before they can reach a tool.
	originProtection := http.NewCrossOriginProtection()
	return mcpBearerAuth(strings.TrimSpace(token), originProtection.Handler(streamable))
}

func mcpBearerAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		want := "Bearer " + token
		got := req.Header.Get("Authorization")
		if token == "" || len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			res.Header().Set("WWW-Authenticate", `Bearer realm="pokepilot-mcp"`)
			res.Header().Set("Cache-Control", "no-store")
			http.Error(res, "unauthorized", http.StatusUnauthorized)
			return
		}
		res.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(res, req)
	})
}

func (c *mcpControl) startRun(ctx context.Context, _ *mcp.CallToolRequest, in mcpStartRunInput) (*mcp.CallToolResult, mcpStartRunOutput, error) {
	planner := strings.ToLower(strings.TrimSpace(in.Planner))
	if planner == "" {
		planner = "llm"
	}
	if planner != "llm" && planner != "scripted" {
		return nil, mcpStartRunOutput{}, fmt.Errorf("planner must be llm or scripted")
	}
	starter := strings.ToLower(strings.TrimSpace(in.Starter))
	if starter == "" {
		starter = "squirtle"
	}
	switch starter {
	case "squirtle", "charmander", "bulbasaur":
	default:
		return nil, mcpStartRunOutput{}, fmt.Errorf("starter must be squirtle, charmander, or bulbasaur")
	}
	if in.FPS < 0 || in.FPS > 240 {
		return nil, mcpStartRunOutput{}, fmt.Errorf("fps must be between 0 and 240")
	}
	if in.MaxRounds < 0 {
		return nil, mcpStartRunOutput{}, fmt.Errorf("max_rounds must be zero (uncapped) or positive")
	}
	if in.MaxFrames < 0 || in.MaxFrames > 50_000_000 {
		return nil, mcpStartRunOutput{}, fmt.Errorf("max_frames must be between 0 and 50000000")
	}
	llmProfile := strings.ToLower(strings.TrimSpace(in.LLMProfile))
	switch llmProfile {
	case "", "default", "gpu", "auto":
	default:
		return nil, mcpStartRunOutput{}, fmt.Errorf("llm_profile must be default, gpu, or auto")
	}

	dest := strings.TrimSpace(in.Dest)
	goal := strings.TrimSpace(in.Goal)
	if planner == "scripted" && dest == "" {
		return nil, mcpStartRunOutput{}, fmt.Errorf("dest is required for scripted runs")
	}
	if planner == "llm" && goal == "" {
		goal = "Earn the Boulder Badge."
	}

	runID := fmt.Sprintf("mcp-%s-%04x", time.Now().UTC().Format("20060102-150405"), mcpRunSequence.Add(1)&0xffff)
	spec := farm.Spec{
		RunID:      runID,
		Seed:       in.Seed,
		Planner:    planner,
		Starter:    starter,
		Dest:       dest,
		Goal:       goal,
		LLMProfile: llmProfile,
		FPS:        in.FPS,
		MaxRounds:  in.MaxRounds,
		MaxFrames:  in.MaxFrames,
	}
	var upstream map[string]any
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/specs", spec, &upstream); err != nil {
		return nil, mcpStartRunOutput{}, err
	}
	status, _ := upstream["status"].(string)
	if status == "" {
		status = "queued"
	}
	return nil, mcpStartRunOutput{RunID: runID, Status: status}, nil
}

func (c *mcpControl) listRuns(ctx context.Context, _ *mcp.CallToolRequest, in mcpListRunsInput) (*mcp.CallToolResult, map[string]any, error) {
	dashboard, err := c.dashboard(ctx)
	if err != nil {
		return nil, nil, err
	}
	status := strings.ToLower(strings.TrimSpace(in.Status))
	if status != "" && status != "queued" && status != "leased" && status != "running" && status != "done" {
		return nil, nil, fmt.Errorf("status must be queued, leased, running, or done")
	}
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > mcpMaxRuns {
		return nil, nil, fmt.Errorf("limit must be between 1 and %d", mcpMaxRuns)
	}

	runs := make([]mcpRunView, 0, min(limit, len(dashboard.Runs)))
	for _, run := range dashboard.Runs {
		if status != "" && run.Status != status {
			continue
		}
		runs = append(runs, run)
		if len(runs) == limit {
			break
		}
	}
	return nil, map[string]any{
		"now":     dashboard.Now,
		"runs":    runs,
		"workers": dashboard.Workers,
	}, nil
}

func (c *mcpControl) getRun(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	dashboard, err := c.dashboard(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, run := range dashboard.Runs {
		if run.RunID == id {
			return nil, map[string]any{"run": run, "now": dashboard.Now}, nil
		}
	}
	return nil, nil, fmt.Errorf("run %q not found", id)
}

func (c *mcpControl) cancelRun(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/cancel", nil, &out); err != nil {
		return nil, nil, err
	}
	out["run_id"] = id
	return nil, out, nil
}

func (c *mcpControl) getTriage(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var groups []map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/triage", nil, &groups); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"groups": groups}, nil
}

func (c *mcpControl) investigateFailure(ctx context.Context, _ *mcp.CallToolRequest, in mcpInvestigateInput) (*mcp.CallToolResult, map[string]any, error) {
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return nil, nil, fmt.Errorf("key is required")
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/triage/"+url.PathEscape(key)+"/investigate", nil, &out); err != nil {
		return nil, nil, err
	}
	out["key"] = key
	return nil, out, nil
}

func (c *mcpControl) dashboard(ctx context.Context) (mcpDashboard, error) {
	var dashboard mcpDashboard
	err := c.requestJSON(ctx, http.MethodGet, "/v1/dashboard", nil, &dashboard)
	return dashboard, err
}

func (c *mcpControl) requestJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode wall request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.wallBase+path, body)
	if err != nil {
		return fmt.Errorf("build wall request: %w", err)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wall unreachable: %w", err)
	}
	defer res.Body.Close()

	data, err := io.ReadAll(io.LimitReader(res.Body, mcpMaxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read wall response: %w", err)
	}
	if len(data) > mcpMaxResponseBytes {
		return fmt.Errorf("wall response exceeds %d bytes", mcpMaxResponseBytes)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = res.Status
		}
		return fmt.Errorf("wall returned %s: %s", res.Status, msg)
	}
	if output == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode wall response: %w", err)
	}
	return nil
}
