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

type mcpExperimentsInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum experiments to return, newest first; defaults to 20 and is capped at 100"`
}

type mcpExperimentInput struct {
	ID string `json:"id" jsonschema:"experiment id from pokepilot_list_experiments"`
}

// The arm metrics an agent compares; the full aggregate (replan reasons,
// token and TPS breakdowns) stays behind pokepilot_get_experiment.
var mcpExperimentArmFields = []string{"runs", "done", "goal_successes", "success_rate",
	"median_frames_to_goal", "median_rounds_to_goal", "avg_run_seconds", "blackouts", "objective_failures"}

func mcpSummaryExperiment(experiment map[string]any) map[string]any {
	out := mcpSelectedFields(experiment, "id", "name", "created_at", "total_pairs", "paired")
	if request, ok := experiment["request"].(map[string]any); ok {
		out["request"] = mcpSelectedFields(request, "game", "goal", "starter", "play_style", "arm_a", "arm_b")
	}
	for _, arm := range []string{"arm_a", "arm_b"} {
		if aggregate, ok := experiment[arm].(map[string]any); ok {
			out[arm] = mcpSelectedFields(aggregate, mcpExperimentArmFields...)
		}
	}
	return out
}

func (c *mcpControl) listExperiments(ctx context.Context, _ *mcp.CallToolRequest, in mcpExperimentsInput) (*mcp.CallToolResult, map[string]any, error) {
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > mcpMaxRuns {
		return nil, nil, fmt.Errorf("limit must be between 1 and %d", mcpMaxRuns)
	}
	var out struct {
		Experiments []map[string]any `json:"experiments"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/experiments", nil, &out); err != nil {
		return nil, nil, err
	}
	total := len(out.Experiments)
	if total > limit {
		out.Experiments = out.Experiments[:limit]
	}
	brief := make([]map[string]any, 0, len(out.Experiments))
	for _, experiment := range out.Experiments {
		brief = append(brief, mcpSummaryExperiment(experiment))
	}
	return nil, map[string]any{"experiments": brief, "total": total}, nil
}

func (c *mcpControl) getExperiment(ctx context.Context, _ *mcp.CallToolRequest, in mcpExperimentInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return nil, nil, fmt.Errorf("id is required")
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/experiments/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (c *mcpControl) startExperiment(ctx context.Context, _ *mcp.CallToolRequest, in farm.ExperimentRequest) (*mcp.CallToolResult, map[string]any, error) {
	// The wall owns validation (arms, goal, seeds, starter, recovery profile);
	// its 400 message is returned as the tool error.
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/experiments", in, &out); err != nil {
		return nil, nil, err
	}
	return nil, mcpSummaryExperiment(out), nil
}

// listModels returns the deployments an experiment arm or run can name,
// without host control URLs or token env names.
func (c *mcpControl) listModels(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out struct {
		Deployments []map[string]any `json:"deployments"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/models", nil, &out); err != nil {
		return nil, nil, err
	}
	deployments := make([]map[string]any, 0, len(out.Deployments))
	for _, d := range out.Deployments {
		deployments = append(deployments, mcpSelectedFields(d, "id", "label", "model_id", "compute",
			"quantization", "enabled", "default_for", "state", "loaded_deployment", "active_leases", "queued", "error"))
	}
	return nil, map[string]any{"deployments": deployments}, nil
}
