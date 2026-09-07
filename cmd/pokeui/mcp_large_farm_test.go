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
			if err != nil || limit < 1 {
				http.Error(res, "unnarrowed dashboard requested", http.StatusInsufficientStorage)
				return
			}
			status := req.URL.Query().Get("status")
			runs := []map[string]any{}
			for i := 0; i < totalRuns && len(runs) < limit; i++ {
				s := "done"
				if i%2 == 0 {
					s = "running"
				}
				if status != "" && s != status {
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
	// One run must not cost a whole-farm read.
	call("pokepilot_get_run", map[string]any{"run_id": "run-7"})
}
