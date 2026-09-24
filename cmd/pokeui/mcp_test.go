package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

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
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/v1/runs/") && strings.HasSuffix(req.URL.Path, "/debug"):
			id := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/v1/runs/"), "/debug")
			json.NewEncoder(res).Encode(map[string]any{ //nolint:errcheck
				"run":       map[string]any{"run_id": id, "status": "done"},
				"summary":   map[string]any{"progress_known": true, "progressed": false, "replay_available": true},
				"artifacts": []map[string]any{{"name": "run.gbrun", "replayable": true}},
			})
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/v1/runs/") && strings.HasSuffix(req.URL.Path, "/artifacts"):
			id := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/v1/runs/"), "/artifacts")
			json.NewEncoder(res).Encode(map[string]any{ //nolint:errcheck
				"run_id": id, "attempt": 1,
				"artifacts": []map[string]any{{"name": "run.gbrun", "store": "s3", "object_key": "runs/x/run.gbrun"}},
			})
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/artifacts/checkpoint.state/content"):
			res.Header().Set("Content-Type", "application/octet-stream")
			res.Write([]byte("checkpoint-bytes")) //nolint:errcheck
		case req.Method == http.MethodPost && strings.HasPrefix(req.URL.Path, "/v1/runs/") && strings.HasSuffix(req.URL.Path, "/cancel"):
			cancelled = strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/v1/runs/"), "/cancel")
			json.NewEncoder(res).Encode(map[string]bool{"cancel": true}) //nolint:errcheck
		case req.Method == http.MethodGet && req.URL.Path == "/v1/triage":
			json.NewEncoder(res).Encode([]map[string]any{{"key": "deadbeef", "pattern": "stuck", "count": 2}}) //nolint:errcheck
		case req.Method == http.MethodPost && req.URL.Path == "/v1/triage/deadbeef/investigate":
			json.NewEncoder(res).Encode(map[string]any{"issue_number": 42}) //nolint:errcheck
		case req.Method == http.MethodPost && req.URL.Path == "/v1/triage/deadbeef/solver-attempt":
			var attempt map[string]any
			if err := json.NewDecoder(req.Body).Decode(&attempt); err != nil {
				http.Error(res, err.Error(), http.StatusBadRequest)
				return
			}
			if attempt["model"] != "qwen3.8-27b/qwen3.8-27b" {
				http.Error(res, "wrong model", http.StatusBadRequest)
				return
			}
			json.NewEncoder(res).Encode(map[string]any{"attempt_count": 1}) //nolint:errcheck
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
		"pokepilot_get_run_artifact_content",
		"pokepilot_get_run_artifacts",
		"pokepilot_get_run_debug",
		"pokepilot_get_run_recovery_audit",
		"pokepilot_get_triage",
		"pokepilot_investigate_failure",
		"pokepilot_list_runs",
		"pokepilot_record_solver_attempt",
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
	if spec.Planner != "llm" || spec.Starter != "charmander" || spec.Goal.String() != "badges:1" || spec.MaxRounds != 40 {
		t.Fatalf("queued spec = %+v", spec)
	}
	if spec.Endless || spec.RandomSeed {
		t.Fatalf("MCP must queue finite runs only: %+v", spec)
	}

	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"pokepilot_get_run", map[string]any{"run_id": runID}},
		{"pokepilot_get_run_debug", map[string]any{"run_id": runID}},
		{"pokepilot_get_run_recovery_audit", map[string]any{"run_id": runID}},
		{"pokepilot_get_run_artifacts", map[string]any{"run_id": runID}},
		{"pokepilot_get_triage", map[string]any{}},
		{"pokepilot_investigate_failure", map[string]any{"key": "deadbeef"}},
		{"pokepilot_record_solver_attempt", map[string]any{
			"key": "deadbeef", "id": "attempt-1", "backend": "opencode",
			"model": "qwen3.8-27b/qwen3.8-27b", "state": "started", "run_id": runID,
		}},
		{"pokepilot_cancel_run", map[string]any{"run_id": runID}},
	} {
		if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: call.name, Arguments: call.args}); err != nil {
			t.Fatalf("%s: %v", call.name, err)
		}
	}
	if cancelled != runID {
		t.Fatalf("cancelled = %q, want %q", cancelled, runID)
	}

	content, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pokepilot_get_run_artifact_content",
		Arguments: map[string]any{"run_id": runID, "name": "checkpoint.state"},
	})
	if err != nil {
		t.Fatalf("get_run_artifact_content: %v", err)
	}
	raw, err := json.Marshal(content.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var artifact mcpArtifactContentOutput
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("decode artifact content result: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(artifact.ContentBase64)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if string(decoded) != "checkpoint-bytes" {
		t.Fatalf("artifact content = %q, want %q", decoded, "checkpoint-bytes")
	}
}

func TestMCPRunRecoveryAuditKeepsOlderRecoveryAndResolvedTriage(t *testing.T) {
	timeline := make([]map[string]any, 0, mcpMaxEvents+12)
	timeline = append(timeline, map[string]any{
		"type": "activity", "source": "system", "kind": "attempt_start", "attempt": 1,
		"detail": "runner old-revision", "message": "Attempt 1 started",
	})
	timeline = append(timeline, map[string]any{
		"type": "activity", "source": "recovery", "kind": "retry", "attempt": 1,
		"recovery_attempt": 1, "message": "Old recovery still matters",
	})
	for i := 0; i < mcpMaxEvents+5; i++ {
		timeline = append(timeline, map[string]any{
			"type": "activity", "source": "llm", "kind": "decision", "attempt": 1,
			"message": fmt.Sprintf("decision-%d", i),
		})
	}
	timeline = append(timeline, map[string]any{
		"type": "activity", "source": "recovery", "kind": "circuit", "attempt": 1,
		"recovery_attempt": 2, "message": "Newest recovery",
	})

	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/v1/runs/run-audit/debug":
			json.NewEncoder(res).Encode(map[string]any{ //nolint:errcheck
				"run": map[string]any{
					"run_id": "run-audit", "status": "done", "attempts": 1,
					"recovery_attempts": 2, "question": strings.Repeat("q", 5000),
				},
				"finish": map[string]any{
					"attempt": 1, "reason": "done", "runner_version": "old-revision",
				},
				"summary": map[string]any{"progress_known": true, "progressed": true},
				"timeline": timeline,
			})
		case "/v1/triage":
			json.NewEncoder(res).Encode([]map[string]any{{ //nolint:errcheck
				"key": "fixed-key", "fingerprint": "sha256:abc", "run_ids": []string{"run-audit"},
				"issue": map[string]any{
					"issue_number": 77, "status": "closed", "resolution": "fixed", "fixed_revision": "fix-revision",
				},
			}})
		default:
			http.NotFound(res, req)
		}
	}))
	t.Cleanup(wall.Close)

	control := &mcpControl{wallBase: wall.URL, artifactBase: wall.URL, http: wall.Client()}
	_, got, err := control.getRunRecoveryAudit(context.Background(), nil, mcpRunInput{RunID: "run-audit"})
	if err != nil {
		t.Fatalf("getRunRecoveryAudit: %v", err)
	}
	if got["recovery_event_count"] != 2 {
		t.Fatalf("recovery_event_count = %#v, want 2", got["recovery_event_count"])
	}
	events, ok := got["recovery_events"].([]map[string]any)
	if !ok || len(events) != 2 {
		t.Fatalf("recovery_events = %#v", got["recovery_events"])
	}
	if events[0]["message"] != "Old recovery still matters" {
		t.Fatalf("old recovery was lost: %#v", events)
	}
	if events[0]["runner_version"] != "old-revision" {
		t.Fatalf("runner revision annotation = %#v", events[0]["runner_version"])
	}
	related, ok := got["related_triage"].([]map[string]any)
	if !ok || len(related) != 1 {
		t.Fatalf("related_triage = %#v", got["related_triage"])
	}
	if related[0]["actionable"] != false || related[0]["fixed_revision"] != "fix-revision" {
		t.Fatalf("resolved triage metadata = %#v", related[0])
	}
	run, ok := got["run"].(map[string]any)
	if !ok {
		t.Fatalf("run = %#v", got["run"])
	}
	if _, leaked := run["question"]; leaked {
		t.Fatalf("audit packet leaked large planner question: %#v", run)
	}
}

// TestMCPArtifactContentUsesReplayForDurabilizedArtifacts guards against the
// regression this tool was built to fix: pokewall durabilizes finish
// artifacts to S3 shortly after a run ends, at which point its own inline
// content route 409s. The browser's identical route already falls back to
// pokereplay for that case (mountRunInspectorRoutes); the MCP tool must make
// the same choice instead of only ever asking pokewall.
func TestMCPArtifactContentUsesReplayForDurabilizedArtifacts(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/content") {
			t.Fatalf("MCP must not ask pokewall directly once replay is configured: %s", req.URL.Path)
		}
		http.NotFound(res, req)
	}))
	t.Cleanup(wall.Close)

	replay := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/artifacts/durable.state/content") {
			res.Header().Set("Content-Type", "application/octet-stream")
			res.Write([]byte("durabilized-bytes")) //nolint:errcheck
			return
		}
		http.NotFound(res, req)
	}))
	t.Cleanup(replay.Close)

	ui := httptest.NewServer(handlerWithServices(wall.URL, replay.URL, "secret"))
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

	content, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pokepilot_get_run_artifact_content",
		Arguments: map[string]any{"run_id": "run-1", "name": "durable.state"},
	})
	if err != nil {
		t.Fatalf("get_run_artifact_content: %v", err)
	}
	raw, err := json.Marshal(content.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var artifact mcpArtifactContentOutput
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("decode artifact content result: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(artifact.ContentBase64)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if string(decoded) != "durabilized-bytes" {
		t.Fatalf("artifact content = %q, want %q", decoded, "durabilized-bytes")
	}
}

func TestCompactEventsKeepsNewestClippedEvents(t *testing.T) {
	var events []any
	for i := 0; i < mcpMaxEvents+5; i++ {
		events = append(events, map[string]any{"at": i, "detail": strings.Repeat("é", mcpMaxEventText)})
	}
	m := map[string]any{"activity": events}
	compactEvents(m, "activity")

	got := m["activity"].([]any)
	if len(got) != mcpMaxEvents || m["activity_omitted"] != 5 {
		t.Fatalf("kept %d omitted %v, want %d and 5", len(got), m["activity_omitted"], mcpMaxEvents)
	}
	first := got[0].(map[string]any)
	if first["at"] != 5 {
		t.Fatalf("first kept event at=%v, want the newest window starting at 5", first["at"])
	}
	detail := first["detail"].(string)
	if len(detail) > mcpMaxEventText+len("…") || !utf8.ValidString(detail) {
		t.Fatalf("detail not clipped to valid UTF-8: %d bytes", len(detail))
	}
}
