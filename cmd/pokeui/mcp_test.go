package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/maestroi/pokepilot/farm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

func TestMCPDisabledWithoutToken(t *testing.T) {
	ui := httptest.NewServer(handler("http://127.0.0.1:1"))
	t.Cleanup(ui.Close)

	req, err := http.NewRequest(http.MethodPost, ui.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("MCP without token = %d, want 404", res.StatusCode)
	}
}

func TestMCPRequiresBearerToken(t *testing.T) {
	ui := httptest.NewServer(handlerWithMCP("http://127.0.0.1:1", "secret"))
	t.Cleanup(ui.Close)

	req, err := http.NewRequest(http.MethodPost, ui.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("MCP without bearer = %d, want 401", res.StatusCode)
	}
	if got := res.Header.Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
		t.Fatalf("WWW-Authenticate = %q, want Bearer challenge", got)
	}
}

func TestMCPToolsDriveOnlyOperatorAPI(t *testing.T) {
	var mu sync.Mutex
	var queued farm.Spec
	var cancelled string
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/v1/specs":
			var spec farm.Spec
			if err := json.NewDecoder(req.Body).Decode(&spec); err != nil {
				http.Error(res, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			queued = spec
			mu.Unlock()
			json.NewEncoder(res).Encode(map[string]string{"status": "queued"}) //nolint:errcheck
		case req.Method == http.MethodGet && req.URL.Path == "/v1/dashboard":
			mu.Lock()
			spec := queued
			mu.Unlock()
			runs := []map[string]any{}
			if spec.RunID != "" {
				runs = append(runs, map[string]any{
					"run_id": spec.RunID, "status": "queued", "planner": spec.Planner,
					"starter": spec.Starter, "goal": spec.Goal, "seed": spec.Seed,
					"fps": spec.FPS, "max_rounds": spec.MaxRounds, "max_frames": spec.MaxFrames,
				})
			}
			json.NewEncoder(res).Encode(map[string]any{"now": int64(123), "runs": runs, "workers": []any{}}) //nolint:errcheck
		case req.Method == http.MethodPost && strings.HasPrefix(req.URL.Path, "/v1/runs/") && strings.HasSuffix(req.URL.Path, "/cancel"):
			cancelled = strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/v1/runs/"), "/cancel")
			json.NewEncoder(res).Encode(map[string]bool{"cancel": true}) //nolint:errcheck
		case req.Method == http.MethodGet && req.URL.Path == "/v1/triage":
			json.NewEncoder(res).Encode([]map[string]any{{"key": "deadbeef", "pattern": "stuck", "count": 2}}) //nolint:errcheck
		case req.Method == http.MethodPost && req.URL.Path == "/v1/triage/deadbeef/investigate":
			json.NewEncoder(res).Encode(map[string]any{"issue_number": 42}) //nolint:errcheck
		case req.URL.Path == "/v1/lease" || strings.Contains(req.URL.Path, "/heartbeat") || strings.Contains(req.URL.Path, "/finish"):
			http.Error(res, "runner-only route reached", http.StatusInternalServerError)
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
		Endpoint:   ui.URL + "/mcp",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	want := []string{
		"pokepilot_cancel_run",
		"pokepilot_get_run",
		"pokepilot_get_triage",
		"pokepilot_investigate_failure",
		"pokepilot_list_runs",
		"pokepilot_start_run",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools = %v, want %v", names, want)
	}

	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pokepilot_start_run",
		Arguments: map[string]any{
			"starter":    "charmander",
			"goal":       "badges:1",
			"max_rounds": 40,
		},
	}); err != nil {
		t.Fatalf("start run: %v", err)
	}

	mu.Lock()
	runID := queued.RunID
	spec := queued
	mu.Unlock()
	if runID == "" || !strings.HasPrefix(runID, "mcp-") {
		t.Fatalf("generated run id = %q", runID)
	}
	if spec.Planner != "llm" || spec.Starter != "charmander" || spec.Goal != "badges:1" || spec.MaxRounds != 40 {
		t.Fatalf("queued spec = %+v", spec)
	}
	if spec.Endless || spec.RandomSeed {
		t.Fatalf("MCP must queue finite runs only: %+v", spec)
	}

	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pokepilot_get_run",
		Arguments: map[string]any{"run_id": runID},
	}); err != nil {
		t.Fatalf("get run: %v", err)
	}
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pokepilot_get_triage",
		Arguments: map[string]any{},
	}); err != nil {
		t.Fatalf("get triage: %v", err)
	}
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pokepilot_investigate_failure",
		Arguments: map[string]any{"key": "deadbeef"},
	}); err != nil {
		t.Fatalf("investigate failure: %v", err)
	}
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pokepilot_cancel_run",
		Arguments: map[string]any{"run_id": runID},
	}); err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled != runID {
		t.Fatalf("cancelled = %q, want %q", cancelled, runID)
	}
}
