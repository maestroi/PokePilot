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

type mcpMediaJobsInput struct {
	RunID string `json:"run_id,omitempty" jsonschema:"only jobs for this run"`
	State string `json:"state,omitempty" jsonschema:"only jobs in this state, queued, preparing, rendering, assembling, uploading, ready, failed or cancelled"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum jobs to return, newest first; defaults to 20 and is capped at 200"`
}

type mcpMediaJobInput struct {
	ID string `json:"id" jsonschema:"media render job id from pokepilot_list_media_jobs"`
}

func (c *mcpControl) listMediaJobs(ctx context.Context, _ *mcp.CallToolRequest, in mcpMediaJobsInput) (*mcp.CallToolResult, map[string]any, error) {
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 200 {
		return nil, nil, fmt.Errorf("limit must be between 1 and 200")
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if id := strings.TrimSpace(in.RunID); id != "" {
		q.Set("run_id", id)
	}
	if state := strings.TrimSpace(in.State); state != "" {
		q.Set("state", state)
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/media/render-jobs?"+q.Encode(), nil, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (c *mcpControl) getMediaJob(ctx context.Context, _ *mcp.CallToolRequest, in mcpMediaJobInput) (*mcp.CallToolResult, map[string]any, error) {
	return c.mediaJobRequest(ctx, http.MethodGet, in.ID, "")
}

func (c *mcpControl) retryMediaJob(ctx context.Context, _ *mcp.CallToolRequest, in mcpMediaJobInput) (*mcp.CallToolResult, map[string]any, error) {
	return c.mediaJobRequest(ctx, http.MethodPost, in.ID, "/retry")
}

func (c *mcpControl) cancelMediaJob(ctx context.Context, _ *mcp.CallToolRequest, in mcpMediaJobInput) (*mcp.CallToolResult, map[string]any, error) {
	return c.mediaJobRequest(ctx, http.MethodPost, in.ID, "/cancel")
}

func (c *mcpControl) mediaJobRequest(ctx context.Context, method, id, action string) (*mcp.CallToolResult, map[string]any, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, fmt.Errorf("id is required")
	}
	var out map[string]any
	if err := c.requestJSON(ctx, method, "/v1/media/render-jobs/"+url.PathEscape(id)+action, nil, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
