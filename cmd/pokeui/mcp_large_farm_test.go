package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A farm with enough history that the unnarrowed dashboard is larger than
// mcpMaxResponseBytes. This is not hypothetical: the live wall measured
// 2,123,033 bytes across 378 runs on 2026-09-07, and every MCP run tool then
// failed at once — including pokepilot_list_runs with limit 1, because the
// limit was applied to a response the client could not finish reading.
//
// The fake wall REFUSES to serve an unnarrowed dashboard, so a client that
// still asks for one fails the test loudly rather than silently reading two
// megabytes it will throw away.
func TestMCPToolsSurviveAnOversizedDashboard(t *testing.T) {
	const totalRuns = 400
	padding := strings.Repeat("x", 6000) // per-run trace/question bulk

	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/dashboard":
			limit, err := strconv.Atoi(req.URL.Query().Get("limit"))
			if err != nil || limit < 1 || limit > mcpListPageSize {
				http.Error(res, "unnarrowed dashboard requested", http.StatusInsufficientStorage)
				return
			}
			offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
			status := req.URL.Query().Get("status")
			if req.URL.Query().Get("sort") != "" && (status != "done" || req.URL.Query().Get("success") != "1") {
				http.Error(res, "archive filters lost", http.StatusBadRequest)
				return
			}
			runs := []map[string]any{}
			skipped := 0
			for i := 0; i < totalRuns && len(runs) < limit; i++ {
				s := "done"
				if i%2 == 0 {
					s = "running"
				}
				if status != "" && s != status {
					continue
				}
				if skipped < offset {
					skipped++
					continue
				}
				runs = append(runs, map[string]any{
					"run_id": "run-" + strconv.Itoa(i), "status": s, "trace": padding,
				})
			}
			json.NewEncoder(res).Encode(map[string]any{"now": int64(1), "runs": runs, "workers": []any{}}) //nolint:errcheck
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/v1/runs/"):
			id := strings.TrimPrefix(req.URL.Path, "/v1/runs/")
			json.NewEncoder(res).Encode(map[string]any{ //nolint:errcheck
				"run": map[string]any{"run_id": id, "status": "done"},
			})
		default:
			http.NotFound(res, req)
		}
	}))
	t.Cleanup(wall.Close)

	ui := httptest.NewServer(handlerWithMCP(wall.URL, "secret"))
	t.Cleanup(ui.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "pokeui-test", Version: "1"}, nil)
	httpClient := &http.Client{Transport: bearerRoundTripper{token: "secret", base: http.DefaultTransport}}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: ui.URL + "/mcp", HTTPClient: httpClient,
	}, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	defer session.Close()

	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		out, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out.IsError {
			t.Fatalf("%s returned an error result: %+v", name, out.Content)
		}
		return out
	}

	// The default limit, an explicit small limit, and a status filter all have
	// to reach the wall as query parameters.
	call("pokepilot_list_runs", map[string]any{})
	call("pokepilot_list_runs", map[string]any{"limit": 1})
	call("pokepilot_list_runs", map[string]any{"limit": 5, "status": "done"})
	// The cap's own maximum must page rather than overflow (100 done runs are
	// > 2 MB in one read on the live farm).
	full := call("pokepilot_list_runs", map[string]any{"limit": mcpMaxRuns, "status": "done"})
	var listed struct {
		Runs []struct {
			RunID string `json:"run_id"`
		} `json:"runs"`
	}
	data, _ := json.Marshal(full.StructuredContent)
	if err := json.Unmarshal(data, &listed); err != nil || len(listed.Runs) != mcpMaxRuns {
		t.Fatalf("limit %d listed %d runs (err %v)", mcpMaxRuns, len(listed.Runs), err)
	}
	if listed.Runs[0].RunID != "run-1" || listed.Runs[mcpMaxRuns-1].RunID != "run-199" {
		t.Fatalf("pages not contiguous: first %s last %s", listed.Runs[0].RunID, listed.Runs[mcpMaxRuns-1].RunID)
	}
	call("pokepilot_list_runs", map[string]any{"status": "done", "success": true, "sort": "frames", "goal": "Earn the Boulder Badge.", "offset": 3})
	if out, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pokepilot_list_runs", Arguments: map[string]any{"sort": "frames"},
	}); err == nil && !out.IsError {
		t.Fatal("archive sort without status=done should be rejected")
	}
	// One run must not cost a whole-farm read.
	call("pokepilot_get_run", map[string]any{"run_id": "run-7"})
}
