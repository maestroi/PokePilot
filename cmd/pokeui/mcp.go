package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	// A looping endless run accumulates hundreds of activity events, each
	// planning event carrying a multi-KB STRATEGY dump (MEASURED 2026-09-23 on
	// run-22ahrk9pflcilu3jxq9xt37x6: 76 KB of activity, repeated as an 87 KB
	// timeline in the debug bundle), which overflows MCP clients' tool-result
	// token cap. MCP gets the newest events with clipped text; the operator UI
	// still serves the full history.
	mcpMaxEvents    = 40
	mcpMaxEventText = 400
)

var mcpRunSequence atomic.Uint64

// mcpControl is deliberately an operator client, not another orchestrator.
// It only speaks the same allowlisted wall API that pokeui exposes to a human;
// lease, heartbeat, finish, checkpoint, worker registration and Docker/Swarm
// controls are not reachable through MCP.
type mcpControl struct {
	wallBase string
	// artifactBase serves GET /v1/runs/{id}/artifacts/{name}/content. It is
	// pokereplay when configured (inline and S3-backed artifacts both
	// resolved server-side from pokewall's own artifact list) and falls back
	// to wallBase (inline only) otherwise — the same selection
	// mountRunInspectorRoutes makes for the browser's identical route.
	artifactBase string
	http         *http.Client
}

type mcpStartRunInput struct {
	Planner    string `json:"planner,omitempty" jsonschema:"planner mode: llm or scripted; defaults to llm"`
	Game       string `json:"game,omitempty" jsonschema:"game to play, e.g. pokemon-red or pokemon-blue; empty lets the runner pick its mounted cartridge"`
	Starter    string `json:"starter,omitempty" jsonschema:"starter Pokemon: squirtle, charmander, or bulbasaur; defaults to squirtle"`
	Dest       string `json:"dest,omitempty" jsonschema:"destination for scripted mode"`
	Goal       string `json:"goal,omitempty" jsonschema:"task statement for llm mode; defaults to earning the Boulder Badge"`
	Seed       int64  `json:"seed,omitempty" jsonschema:"deterministic run seed; zero is the bit-identical baseline"`
	FPS        int    `json:"fps,omitempty" jsonschema:"emulation pace; zero runs flat out"`
	MaxRounds  int    `json:"max_rounds,omitempty" jsonschema:"optional emergency/experiment LLM objective cap; zero means no hard round cap"`
	MaxFrames  int    `json:"max_frames,omitempty" jsonschema:"emulated frame budget; zero uses the runner default"`
	LLMProfile string `json:"llm_profile,omitempty" jsonschema:"llm endpoint routing: default, gpu, or auto (GPU primary with LAN fallback)"`
	// ReasoningEffort overrides the strategist's reasoning_effort: low,
	// medium, or high. Empty/auto means the endpoint's configured default.
	// Omitting the field entirely is a distinct broken path on this
	// server's llama.cpp build (see agent.LLMPlanner.ReasoningEffort) —
	// this option lets an operator dial it without redeploying.
	ReasoningEffort string `json:"reasoning_effort,omitempty" jsonschema:"strategist reasoning effort: low, medium, high, off (thinking disabled outright), or auto (endpoint default)"`
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

type mcpTriageInput struct {
	IncludeResolved bool `json:"include_resolved,omitempty" jsonschema:"include historical failure groups whose linked issue is already resolved; defaults to false"`
}

type mcpInvestigateInput struct {
	Key string `json:"key" jsonschema:"triage failure key returned by pokepilot_get_triage"`
}

type mcpSolverAttemptInput struct {
	Key       string `json:"key" jsonschema:"triage failure key returned by pokepilot_get_triage"`
	ID        string `json:"id" jsonschema:"stable id for this solver attempt; updates with the same id replace that attempt"`
	Backend   string `json:"backend" jsonschema:"coding agent runtime, for example opencode, cursor, codex, or claude"`
	Model     string `json:"model,omitempty" jsonschema:"actual requested coding model id when known"`
	State     string `json:"state" jsonschema:"attempt state such as started, agent_failed, no_pr, pr_opened, or pr_updated"`
	RunID     string `json:"run_id,omitempty" jsonschema:"representative failing PokePilot run id"`
	Branch    string `json:"branch,omitempty" jsonschema:"repair branch produced by the coding agent"`
	PRNumber  int64  `json:"pr_number,omitempty" jsonschema:"GitHub pull request number produced by the attempt"`
	PRURL     string `json:"pr_url,omitempty" jsonschema:"GitHub pull request URL produced by the attempt"`
	ExitCode  int    `json:"exit_code,omitempty" jsonschema:"coding agent process exit code when non-zero"`
	Note      string `json:"note,omitempty" jsonschema:"short machine/operator note about the attempt outcome"`
	StartedAt int64  `json:"started_at,omitempty" jsonschema:"optional Unix start timestamp; wall fills it for a new attempt when omitted"`
}

type mcpArtifactContentInput struct {
	RunID string `json:"run_id" jsonschema:"PokePilot run id"`
	Name  string `json:"name" jsonschema:"artifact name, exactly as pokepilot_get_run_artifacts listed it"`
}

type mcpArtifactContentOutput struct {
	RunID         string `json:"run_id"`
	Name          string `json:"name"`
	MediaType     string `json:"media_type,omitempty"`
	Size          int    `json:"size"`
	ContentBase64 string `json:"content_base64"`
}

type mcpRunView struct {
	RunID           string         `json:"run_id"`
	Status          string         `json:"status"`
	Planner         string         `json:"planner,omitempty"`
	Starter         string         `json:"starter,omitempty"`
	Dest            string         `json:"dest,omitempty"`
	Goal            string         `json:"goal,omitempty"`
	LLMProfile      string         `json:"llm_profile,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	Seed            int64          `json:"seed"`
	FPS             int            `json:"fps"`
	MaxRounds       int            `json:"max_rounds"`
	MaxFrames       int            `json:"max_frames"`
	QueuedAt        int64          `json:"queued_at,omitempty"`
	EndedAt         int64          `json:"ended_at,omitempty"`
	Attempts        int            `json:"attempts"`
	Frame           uint64         `json:"frame"`
	Map             uint8          `json:"map"`
	X               uint8          `json:"x"`
	Y               uint8          `json:"y"`
	Trace           string         `json:"trace,omitempty"`
	Question        string         `json:"question,omitempty"`
	Decision        string         `json:"decision,omitempty"`
	StopSoFar       string         `json:"stop_so_far,omitempty"`
	Stats           *farm.LLMStats `json:"stats,omitempty"`
	Player          *farm.Player   `json:"player,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	Detail          string         `json:"detail,omitempty"`
	Issue           map[string]any `json:"issue,omitempty"`
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

func newMCPHandler(wallBase, replayBase, token string) http.Handler {
	artifactBase := strings.TrimRight(strings.TrimSpace(replayBase), "/")
	if artifactBase == "" {
		artifactBase = strings.TrimRight(wallBase, "/")
	}
	control := &mcpControl{
		wallBase:     strings.TrimRight(wallBase, "/"),
		artifactBase: artifactBase,
		http:         &http.Client{Timeout: mcpWallTimeout},
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
		Description: "List recent PokePilot runs, optionally filtered by lifecycle status. Finished failures include their linked issue status/resolution when known; this is historical evidence, not an actionable work queue.",
	}, control.listRuns)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_get_run",
		Description: "Get the live or finished state of one PokePilot run, including planner state, party, location and LLM statistics when available.",
	}, control.getRun)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_get_run_debug",
		Description: "Get the compact persisted debug bundle for one run: finish reason, trace tail, progress deltas, latest planner decision, timeline markers and artifact references. Large artifact bytes are never embedded.",
	}, control.getRunDebug)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_get_run_recovery_audit",
		Description: "Get a recovery-focused audit packet for one run. Returns the wall's full bounded recovery/failure activity history without the normal 40-event MCP timeline truncation, annotates recovery events with attempt runner revisions when available, and includes related triage groups including resolved history for stale/duplicate/regression analysis.",
	}, control.getRunRecoveryAudit)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_get_run_artifacts",
		Description: "List one run's artifacts and durable storage references without downloading artifact bytes. Use this to discover run.gbrun recordings and diagnostic evidence.",
	}, control.getRunArtifacts)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_get_run_artifact_content",
		Description: "Fetch one small artifact's bytes (a .state checkpoint, .ram snapshot, or knowledge/failure JSON) as base64, for local reproduction, whether pokewall still holds it inline or it has since been durabilized to S3. Bounded by the MCP response cap, so a large recording such as run.gbrun still will not fit; use the operator UI/replay service for that.",
	}, control.getRunArtifactContent)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_cancel_run",
		Description: "Request cooperative cancellation of one queued or active PokePilot run.",
	}, control.cancelRun)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_get_triage",
		Description: "Get the authoritative actionable failure groups. Linked issues that are already resolved are hidden by default so historical fixes are not investigated again; pass include_resolved=true only when auditing history.",
	}, control.getTriage)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_investigate_failure",
		Description: "Trigger the existing PokePilot investigation handoff for one actionable triage failure key.",
	}, control.investigateFailure)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pokepilot_record_solver_attempt",
		Description: "Record or update which coding agent/model attempted a triage failure and whether it produced a PR. Issue resolution and PokePilot verification determine whether that repair ultimately succeeded.",
	}, control.recordSolverAttempt)

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
	reasoningEffort := strings.ToLower(strings.TrimSpace(in.ReasoningEffort))
	switch reasoningEffort {
	case "", "auto", "low", "medium", "high", "off":
	default:
		return nil, mcpStartRunOutput{}, fmt.Errorf("reasoning_effort must be low, medium, high, off, or auto")
	}
	if reasoningEffort == "auto" {
		reasoningEffort = ""
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
		RunID:           runID,
		Seed:            in.Seed,
		Game:            strings.ToLower(strings.TrimSpace(in.Game)),
		Planner:         planner,
		Starter:         starter,
		Dest:            dest,
		Goal:            farm.GoalFrom(goal),
		LLMProfile:      llmProfile,
		ReasoningEffort: reasoningEffort,
		FPS:             in.FPS,
		MaxRounds:       in.MaxRounds,
		MaxFrames:       in.MaxFrames,
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

	// Narrow at the wall, not here. The whole dashboard is megabytes once a
	// farm has a few hundred runs — more than mcpMaxResponseBytes — so a
	// client-side limit is applied to a response that already failed to read.
	dashboard, err := c.dashboardNarrowed(ctx, status, limit)
	if err != nil {
		return nil, nil, err
	}

	runs := dashboard.Runs
	if len(runs) > limit {
		runs = runs[:limit]
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
	// One run comes from the wall's own single-run route, not from scanning
	// the whole dashboard: the dashboard carries every run's question, trace,
	// stats and sprite trail, so it outgrows mcpMaxResponseBytes as a farm
	// accumulates runs (MEASURED 2026-09-07: 2,123,033 bytes at 378 runs) and
	// this tool would start failing for every id at once, including ids whose
	// own record is a few hundred bytes.
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, nil, err
	}
	if out == nil {
		return nil, nil, fmt.Errorf("run %q not found", id)
	}
	if run, ok := out["run"].(map[string]any); ok {
		compactEvents(run, "activity")
	}
	return nil, out, nil
}

func (c *mcpControl) getRunDebug(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(id)+"/debug", nil, &out); err != nil {
		return nil, nil, err
	}
	// timeline already carries every activity event; drop the duplicate copy.
	if run, ok := out["run"].(map[string]any); ok {
		delete(run, "activity")
	}
	compactEvents(out, "timeline")
	return nil, out, nil
}

func (c *mcpControl) getRunRecoveryAudit(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}

	// Fetch the wall's raw debug bundle directly. Do not call getRunDebug here:
	// that MCP tool intentionally keeps only the newest mcpMaxEvents timeline
	// entries, while a recovery audit must be able to see an older recovery that
	// is still present in pokewall's bounded activity history.
	var debug map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(id)+"/debug", nil, &debug); err != nil {
		return nil, nil, err
	}
	recoveries, kindCounts, recoveryAttempts := recoveryAuditEvents(debug)

	// Triage is the durable failure/issue view. Include resolved groups here on
	// purpose: a recovery audit needs them to decide that an old event is stale
	// or duplicate rather than creating the same repair again.
	var triage []map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/triage", nil, &triage); err != nil {
		return nil, nil, err
	}
	related := make([]map[string]any, 0)
	for _, group := range triage {
		if !triageGroupMentionsRun(group, id) {
			continue
		}
		triageGroupActionable(group)
		related = append(related, group)
	}

	out := map[string]any{
		"run_id":                 id,
		"recovery_events":        recoveries,
		"recovery_event_count":   len(recoveries),
		"recovery_attempt_count": recoveryAttempts,
		"recovery_kinds":         kindCounts,
		"related_triage":         related,
		"related_triage_count":   len(related),
	}
	if timeline, ok := debug["timeline"].([]any); ok {
		out["source_timeline_event_count"] = len(timeline)
	}
	if run, ok := debug["run"].(map[string]any); ok {
		out["run"] = compactRecoveryAuditRecord(run, []string{
			"run_id", "status", "planner", "starter", "goal", "play_style", "risk_tolerance",
			"wild_encounters", "llm_profile", "reasoning_effort", "seed", "attempts",
			"error_attempts", "loss_recoveries", "recovery_profile", "recovery_attempts",
			"recovery_badges", "recovery_events", "recovery_maps", "reason", "detail",
			"frame", "map", "x", "y", "queued_at", "ended_at", "resume_from_run_id",
		})
	}
	if finish, ok := debug["finish"].(map[string]any); ok {
		out["finish"] = compactRecoveryAuditRecord(finish, []string{
			"attempt", "reason", "detail", "runner_version", "seed_burn", "progress_early", "progress_final",
		})
	}
	if summary, ok := debug["summary"]; ok {
		out["summary"] = summary
	}
	return nil, out, nil
}

func recoveryAuditEvents(debug map[string]any) ([]map[string]any, map[string]int, int) {
	timeline, _ := debug["timeline"].([]any)
	fallbackVersion := ""
	if finish, ok := debug["finish"].(map[string]any); ok {
		fallbackVersion, _ = finish["runner_version"].(string)
	}

	attemptVersions := map[int]string{}
	currentVersion := ""
	kindCounts := map[string]int{}
	attempts := map[int]struct{}{}
	out := make([]map[string]any, 0)
	for _, raw := range timeline {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		attempt := mcpJSONInt(event["attempt"])
		source, _ := event["source"].(string)
		kind, _ := event["kind"].(string)
		source = strings.ToLower(strings.TrimSpace(source))
		kind = strings.ToLower(strings.TrimSpace(kind))

		if source == "system" && kind == "attempt_start" {
			detail, _ := event["detail"].(string)
			version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(detail), "runner "))
			if version != "" {
				currentVersion = version
				if attempt > 0 {
					attemptVersions[attempt] = version
				}
			}
		}

		isRecovery := source == "recovery" || strings.Contains(kind, "recovery") || kind == "failure" || kind == "retry" || kind == "circuit"
		if !isRecovery {
			continue
		}
		copy := make(map[string]any, len(event)+1)
		for key, value := range event {
			copy[key] = value
		}
		version := attemptVersions[attempt]
		if version == "" {
			version = currentVersion
		}
		if version == "" {
			version = fallbackVersion
		}
		if version != "" {
			copy["runner_version"] = version
		}
		out = append(out, copy)
		if kind == "" {
			kind = "event"
		}
		kindCounts[kind]++
		if attempt > 0 {
			attempts[attempt] = struct{}{}
		}
	}
	return out, kindCounts, len(attempts)
}

func compactRecoveryAuditRecord(in map[string]any, keys []string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := in[key]; ok {
			out[key] = value
		}
	}
	return out
}

func mcpJSONInt(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		n, _ := strconv.Atoi(v.String())
		return n
	default:
		return 0
	}
}

func triageGroupMentionsRun(group map[string]any, runID string) bool {
	for _, key := range []string{"run_ids", "runs"} {
		switch values := group[key].(type) {
		case []any:
			for _, value := range values {
				if s, ok := value.(string); ok && s == runID {
					return true
				}
			}
		case []string:
			for _, value := range values {
				if value == runID {
					return true
				}
			}
		}
	}
	for _, key := range []string{"run_id", "latest_run_id"} {
		if value, _ := group[key].(string); value == runID {
			return true
		}
	}
	return false
}

// compactEvents keeps the newest mcpMaxEvents entries of the event list at
// m[key] (the wall returns them oldest first), records how many were dropped
// under key+"_omitted", and clips each event's long text fields.
func compactEvents(m map[string]any, key string) {
	events, ok := m[key].([]any)
	if !ok {
		return
	}
	if n := len(events) - mcpMaxEvents; n > 0 {
		events = events[n:]
		m[key+"_omitted"] = n
	}
	for _, e := range events {
		ev, ok := e.(map[string]any)
		if !ok {
			continue
		}
		for _, f := range []string{"detail", "message", "summary"} {
			if s, ok := ev[f].(string); ok && len(s) > mcpMaxEventText {
				ev[f] = strings.ToValidUTF8(s[:mcpMaxEventText], "") + "…"
			}
		}
	}
	m[key] = events
}

func (c *mcpControl) getRunArtifacts(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(in.RunID)
	if id == "" {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(id)+"/artifacts", nil, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (c *mcpControl) getRunArtifactContent(ctx context.Context, _ *mcp.CallToolRequest, in mcpArtifactContentInput) (*mcp.CallToolResult, mcpArtifactContentOutput, error) {
	id := strings.TrimSpace(in.RunID)
	name := strings.TrimSpace(in.Name)
	if id == "" {
		return nil, mcpArtifactContentOutput{}, fmt.Errorf("run_id is required")
	}
	if name == "" {
		return nil, mcpArtifactContentOutput{}, fmt.Errorf("name is required")
	}
	path := "/v1/runs/" + url.PathEscape(id) + "/artifacts/" + url.PathEscape(name) + "/content"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.artifactBase+path, nil)
	if err != nil {
		return nil, mcpArtifactContentOutput{}, fmt.Errorf("build wall request: %w", err)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, mcpArtifactContentOutput{}, fmt.Errorf("wall unreachable: %w", err)
	}
	defer res.Body.Close()

	data, err := io.ReadAll(io.LimitReader(res.Body, mcpMaxResponseBytes+1))
	if err != nil {
		return nil, mcpArtifactContentOutput{}, fmt.Errorf("read wall response: %w", err)
	}
	if len(data) > mcpMaxResponseBytes {
		return nil, mcpArtifactContentOutput{}, fmt.Errorf("artifact %q exceeds %d bytes; fetch it through the operator UI instead", name, mcpMaxResponseBytes)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = res.Status
		}
		return nil, mcpArtifactContentOutput{}, fmt.Errorf("wall returned %s: %s", res.Status, msg)
	}
	return nil, mcpArtifactContentOutput{
		RunID:         id,
		Name:          name,
		MediaType:     res.Header.Get("Content-Type"),
		Size:          len(data),
		ContentBase64: base64.StdEncoding.EncodeToString(data),
	}, nil
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

// triageGroupActionable annotates a wall triage group and reports whether it
// should be offered as new work. Agent Orchestrator is the durable resolution
// source already synced into IssueLink by pokewall. Active/reopened statuses
// deliberately win over an old resolution value so a regression becomes
// actionable again as soon as the issue is reopened.
func triageGroupActionable(group map[string]any) bool {
	issue, _ := group["issue"].(map[string]any)
	if issue == nil {
		group["actionable"] = true
		return true
	}
	status, _ := issue["status"].(string)
	resolution, _ := issue["resolution"].(string)
	status = strings.ToLower(strings.TrimSpace(status))
	resolution = strings.ToLower(strings.TrimSpace(resolution))

	switch status {
	case "open", "reopened", "investigating", "in_progress", "in-progress", "todo", "backlog":
		group["actionable"] = true
		return true
	}

	resolved := resolution != ""
	if !resolved {
		switch status {
		case "resolved", "closed", "fixed", "done", "completed":
			resolved = true
		}
	}
	group["actionable"] = !resolved
	if resolved {
		state := resolution
		if state == "" {
			state = status
		}
		group["resolution_state"] = state
		if rev, _ := issue["fixed_revision"].(string); strings.TrimSpace(rev) != "" {
			group["fixed_revision"] = rev
		}
	}
	return !resolved
}

func (c *mcpControl) getTriage(ctx context.Context, _ *mcp.CallToolRequest, in mcpTriageInput) (*mcp.CallToolResult, map[string]any, error) {
	var groups []map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/triage", nil, &groups); err != nil {
		return nil, nil, err
	}
	actionable := make([]map[string]any, 0, len(groups))
	resolvedHidden := 0
	for _, group := range groups {
		isActionable := triageGroupActionable(group)
		if isActionable || in.IncludeResolved {
			actionable = append(actionable, group)
		} else {
			resolvedHidden++
		}
	}
	return nil, map[string]any{
		"groups":           actionable,
		"resolved_hidden":  resolvedHidden,
		"include_resolved": in.IncludeResolved,
	}, nil
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

func (c *mcpControl) recordSolverAttempt(ctx context.Context, _ *mcp.CallToolRequest, in mcpSolverAttemptInput) (*mcp.CallToolResult, map[string]any, error) {
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return nil, nil, fmt.Errorf("key is required")
	}
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Backend) == "" || strings.TrimSpace(in.State) == "" {
		return nil, nil, fmt.Errorf("id, backend, and state are required")
	}
	payload := map[string]any{
		"id":         in.ID,
		"backend":    in.Backend,
		"model":      in.Model,
		"state":      in.State,
		"run_id":     in.RunID,
		"branch":     in.Branch,
		"pr_number":  in.PRNumber,
		"pr_url":     in.PRURL,
		"exit_code":  in.ExitCode,
		"note":       in.Note,
		"started_at": in.StartedAt,
	}
	var out map[string]any
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/triage/"+url.PathEscape(key)+"/solver-attempt", payload, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (c *mcpControl) dashboard(ctx context.Context) (mcpDashboard, error) {
	return c.dashboardNarrowed(ctx, "", 0)
}

// dashboardNarrowed fetches the dashboard with the wall doing the filtering.
// An empty status and a limit of 0 mean unnarrowed, which is what the
// whole-farm callers (getRun, the stats views) still ask for.
func (c *mcpControl) dashboardNarrowed(ctx context.Context, status string, limit int) (mcpDashboard, error) {
	path := "/v1/dashboard"
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var dashboard mcpDashboard
	err := c.requestJSON(ctx, http.MethodGet, path, nil, &dashboard)
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
