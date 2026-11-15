package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/maestroi/pokepilot/farm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpReproInput struct {
	RunID      string `json:"run_id" jsonschema:"llm run to reproduce"`
	Attempt    int    `json:"attempt,omitempty" jsonschema:"attempt whose checkpoint to resume; 0 uses the run's latest attempt"`
	Checkpoint string `json:"checkpoint,omitempty" jsonschema:"checkpoint name from pokepilot_list_checkpoints; empty picks the attempt's latest replayable checkpoint"`
}

type mcpFlagStuckInput struct {
	RunID string `json:"run_id" jsonschema:"active run to stop as stuck"`
	Note  string `json:"note,omitempty" jsonschema:"why the run is stuck; kept on the failure (max 500 characters)"`
}

// cloneRun queues a fresh run with the source's exact spec. The wall copies
// endless/random_seed too, so an endless source is refused: MCP only ever
// queues finite runs.
func (c *mcpControl) cloneRun(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	var source struct {
		Run map[string]any `json:"run"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(id), nil, &source); err != nil {
		return nil, nil, err
	}
	if source.Run == nil {
		return nil, nil, fmt.Errorf("run %q not found", id)
	}
	if endless, _ := source.Run["endless"].(bool); endless {
		return nil, nil, fmt.Errorf("run %q is endless; MCP queues finite runs only, use pokepilot_start_run or pokepilot_queue_repro", id)
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/clone", nil, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (c *mcpControl) queueRepro(ctx context.Context, _ *mcp.CallToolRequest, in mcpReproInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	if in.Attempt < 0 {
		return nil, nil, fmt.Errorf("attempt must be non-negative")
	}
	var out map[string]any
	body := farm.ReplayRequest{Attempt: in.Attempt, Checkpoint: strings.TrimSpace(in.Checkpoint)}
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/repro", body, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (c *mcpControl) flagStuck(ctx context.Context, _ *mcp.CallToolRequest, in mcpFlagStuckInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	var out map[string]any
	body := map[string]string{"note": strings.TrimSpace(in.Note)}
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/flag-stuck", body, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
