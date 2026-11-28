package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpCheckpointsInput struct {
	RunID   string `json:"run_id" jsonschema:"PokePilot run id"`
	Attempt int    `json:"attempt,omitempty" jsonschema:"attempt whose checkpoints to list; 0 uses the run's latest attempt"`
}

// getRunAttempts lists every finished attempt with the triage problems it hit,
// so a multi-attempt run can be debugged at the attempt that actually failed
// rather than the latest one.
func (c *mcpControl) getRunAttempts(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	var attempts []map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(id)+"/attempts", nil, &attempts); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"run_id": id, "attempts": attempts}, nil
}

func (c *mcpControl) listCheckpoints(ctx context.Context, _ *mcp.CallToolRequest, in mcpCheckpointsInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	if in.Attempt < 0 {
		return nil, nil, fmt.Errorf("attempt must be non-negative")
	}
	path := "/v1/runs/" + url.PathEscape(id) + "/checkpoints"
	if in.Attempt > 0 {
		path += "?attempt=" + strconv.Itoa(in.Attempt)
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, nil, err
	}
	// Each checkpoint repeats the planner's question/decision dump; clip them
	// like the run summaries do so a long attempt stays readable.
	if checkpoints, ok := out["checkpoints"].([]any); ok {
		for _, raw := range checkpoints {
			if cp, ok := raw.(map[string]any); ok {
				for _, key := range []string{"question", "decision"} {
					if value, ok := cp[key].(string); ok {
						cp[key] = mcpClipString(value, 240)
					}
				}
			}
		}
	}
	return nil, out, nil
}
