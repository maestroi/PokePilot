package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		in      string
		command string
		arg     string
	}{
		{"/status", "status", ""},
		{" /run@PokePilotBot run-123 ", "run", "run-123"},
		{"hello", "", ""},
		{"/restart run-1", "restart", "run-1"},
	}
	for _, tt := range tests {
		cmd, arg := parseCommand(tt.in)
		if cmd != tt.command || arg != tt.arg {
			t.Fatalf("parseCommand(%q) = %q %q, want %q %q", tt.in, cmd, arg, tt.command, tt.arg)
		}
	}
}

func TestLoadConfigLegacyChatIsAuthorizedAndNotified(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("TELEGRAM_CHAT_ID", "12345")
	t.Setenv("TELEGRAM_ALLOWED_CHAT_IDS", "")
	t.Setenv("TELEGRAM_NOTIFY_CHAT_IDS", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.AllowedChats[12345]; !ok {
		t.Fatal("legacy chat not authorized")
	}
	if len(cfg.NotifyChats) != 1 || cfg.NotifyChats[0] != 12345 {
		t.Fatalf("notify chats = %v", cfg.NotifyChats)
	}
}

func TestLoadConfigDigestHourRange(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	for in, want := range map[string]int{"25": 9, "-1": 9, "x": 9, "0": 0, "23": 23} {
		t.Setenv("POKEPILOT_WATCH_DIGEST_HOUR", in)
		cfg, err := loadConfig()
		if err != nil || cfg.DigestHour != want {
			t.Fatalf("digest hour %q: %d %v", in, cfg.DigestHour, err)
		}
	}
}

func TestStatusAndRunTextUseOperatorAPI(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/dashboard":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"wall_version": "abc123",
				"workers":      []map[string]any{{"addr": "runner:8099"}},
				"runs": []map[string]any{
					{"run_id": "run-live", "status": "running", "game": "pokemon-gold", "seed": 7, "frame": 123, "goal": "Earn Zephyr Badge"},
					{"run_id": "run-fail", "status": "done", "reason": "error"},
				},
			})
		case "/v1/triage":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"key": "deadbeef", "pattern": "field blockage", "count": 2, "run_ids": []string{"run-live"}}})
		case "/v1/runs/run-live":
			_ = json.NewEncoder(w).Encode(map[string]any{"run": map[string]any{
				"run_id": "run-live", "status": "running", "game": "pokemon-gold", "seed": 7, "frame": 123,
				"goal": "Earn Zephyr Badge", "decision": "Reach Violet Gym",
				"player": map[string]any{"badges": []string{"zephyr"}, "party": []map[string]any{{"name": "totodile", "level": 14, "hp": 32, "max_hp": 40}}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer wall.Close()

	b := &bot{
		cfg: config{AdminBaseURL: "https://admin.example", SpectatorBase: "https://public.example/runs"},
		op:  operatorapi.New(wall.URL, "", ""),
	}
	status, err := b.statusText(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1 active", "1 recent failures", "Workers: 1", "Triage groups: 1"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status missing %q: %s", want, status)
		}
	}
	run, keyboard, err := b.runText(context.Background(), "run-live")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pokemon-gold", "Earn Zephyr Badge", "Reach Violet Gym", "totodile Lv14", "field blockage"} {
		if !strings.Contains(run, want) {
			t.Fatalf("run text missing %q: %s", want, run)
		}
	}
	if keyboard == nil || len(keyboard.InlineKeyboard) != 1 || keyboard.InlineKeyboard[0][0].URL != "https://admin.example/runs/run-live" {
		t.Fatalf("keyboard = %#v", keyboard)
	}
}

func TestAuthorizationDefaultDeny(t *testing.T) {
	b := &bot{cfg: config{AllowedUsers: map[int64]struct{}{}, AllowedChats: map[int64]struct{}{}}}
	if b.authorized(1, 2) {
		t.Fatal("empty allowlist authorized a caller")
	}
	b.cfg.AllowedUsers[1] = struct{}{}
	if !b.authorized(1, 999) {
		t.Fatal("allowed user denied")
	}
	b.cfg.AllowedUsers = map[int64]struct{}{}
	b.cfg.AllowedChats[2] = struct{}{}
	if !b.authorized(999, 2) {
		t.Fatal("allowed chat denied")
	}
}

func TestSuccessfulReason(t *testing.T) {
	for _, value := range []string{"goal", "goal_complete", "done", "completed"} {
		if !successfulReason(value) {
			t.Fatalf("%q should be successful", value)
		}
	}
	for _, value := range []string{"error", "lost", "cancelled", "budget"} {
		if successfulReason(value) {
			t.Fatalf("%q should not be successful", value)
		}
	}
}

func TestControlRateLimit(t *testing.T) {
	b := &bot{cfg: config{ControlSpacing: time.Minute}, lastControl: map[int64]time.Time{}}
	if !b.controlAllowed(42) {
		t.Fatal("first action denied")
	}
	if b.controlAllowed(42) {
		t.Fatal("second action was not rate-limited")
	}
}

// captureTransport answers Telegram API calls locally and records the message
// text, so notification content is asserted without network access or a token.
type captureTransport struct {
	mu    sync.Mutex
	texts []string
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if text, ok := payload["text"].(string); ok {
			c.mu.Lock()
			c.texts = append(c.texts, text)
			c.mu.Unlock()
		}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":1}}`)),
		Request:    req,
	}, nil
}

func (c *captureTransport) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.texts...)
}

func newCaptureBot(op *operatorapi.Client) (*bot, *captureTransport) {
	capture := &captureTransport{}
	return &bot{
		cfg: config{
			NotifyChats:    []int64{7},
			AdminBaseURL:   "https://admin.example",
			StallAfter:     time.Hour,
			ControlSpacing: time.Minute,
		},
		tg:   &telegramClient{token: "test", http: &http.Client{Transport: capture}},
		op:   op,
		runs: make(map[string]runWatch),
	}, capture
}

// A run that completes a declared progression goal has reached the end of that
// progression track. This is the Gen-II supported-frontier case the operator
// needs, and it is decided from goal_kind/goal_id rather than goal prose.
func TestCompletedProgressGoalNotifiesFrontier(t *testing.T) {
	b, capture := newCaptureBot(operatorapi.New("", "", ""))
	ctx := context.Background()
	b.observeRuns(ctx, []operatorapi.Run{{RunID: "run-frontier", Status: "running", Game: "pokemon-gold", Seed: 7, Frame: 10}})
	b.observeRuns(ctx, []operatorapi.Run{{
		RunID: "run-frontier", Status: "done", Reason: "goal", Game: "pokemon-gold", Seed: 7, Frame: 99,
		Stats: &operatorapi.RunStats{
			GoalKind: "progress", GoalID: "gs_supported_frontier", GoalComplete: true,
			GoalSummary: "progress gs_supported_frontier",
		},
	}})

	texts := capture.all()
	if len(texts) != 1 {
		t.Fatalf("notifications = %d, want 1: %v", len(texts), texts)
	}
	for _, want := range []string{"run-frontier", "progression goal complete", "gs_supported_frontier", "frontier"} {
		if !strings.Contains(texts[0], want) {
			t.Fatalf("frontier notification missing %q: %s", want, texts[0])
		}
	}
}

func TestOrdinaryCompletionIsNotAFrontierNotification(t *testing.T) {
	b, capture := newCaptureBot(operatorapi.New("", "", ""))
	ctx := context.Background()
	b.observeRuns(ctx, []operatorapi.Run{{RunID: "run-badges", Status: "running", Frame: 10}})
	b.observeRuns(ctx, []operatorapi.Run{{
		RunID: "run-badges", Status: "done", Reason: "goal", Frame: 99,
		Stats: &operatorapi.RunStats{GoalKind: "badges", GoalComplete: true, GoalSummary: "badges 1/1"},
	}})

	texts := capture.all()
	if len(texts) != 1 || !strings.Contains(texts[0], "finished") {
		t.Fatalf("notifications = %v, want a plain finished notice", texts)
	}
	if strings.Contains(texts[0], "frontier") {
		t.Fatalf("badge goal misreported as a progression frontier: %s", texts[0])
	}
}

// The dashboard list does not always carry stats, so a completion transition
// must still find the structured goal identity on the single-run endpoint.
func TestFrontierDetectionFallsBackToRunDetail(t *testing.T) {
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run-frontier" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"run": map[string]any{
			"run_id": "run-frontier", "status": "done",
			"stats": map[string]any{"goal_kind": "progress", "goal_id": "gs_supported_frontier", "goal_complete": true},
		}})
	}))
	defer wall.Close()

	b, capture := newCaptureBot(operatorapi.New(wall.URL, "", ""))
	ctx := context.Background()
	b.observeRuns(ctx, []operatorapi.Run{{RunID: "run-frontier", Status: "running", Frame: 10}})
	b.observeRuns(ctx, []operatorapi.Run{{RunID: "run-frontier", Status: "done", Reason: "goal", Frame: 99}})

	texts := capture.all()
	if len(texts) != 1 || !strings.Contains(texts[0], "gs_supported_frontier") {
		t.Fatalf("notifications = %v, want the frontier notice from run detail", texts)
	}
}

func TestAlertsTextIncludesRecentlyResolved(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	alertmanager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "active=false") {
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"status":      map[string]any{"state": "suppressed"},
				"labels":      map[string]string{"alertname": "FarmDown"},
				"annotations": map[string]string{"summary": "farm recovered"},
			}})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"status":      map[string]any{"state": "active"},
			"labels":      map[string]string{"alertname": "DiskFull"},
			"annotations": map[string]string{"summary": "disk almost full"},
		}})
	}))
	defer alertmanager.Close()

	b, _ := newCaptureBot(operatorapi.New("", "", alertmanager.URL))
	text, err := b.alertsText(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Active alerts (1)", "DiskFull", "Recently resolved (1)", "FarmDown"} {
		if !strings.Contains(text, want) {
			t.Fatalf("alerts text missing %q: %s", want, text)
		}
	}
}

func TestAlertsTextWhenAlertmanagerIsNotConfigured(t *testing.T) {
	b, _ := newCaptureBot(operatorapi.New("", "", ""))
	text, err := b.alertsText(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "not configured") {
		t.Fatalf("alerts text = %q", text)
	}
}

func TestFrontierDetailNamesTheGoal(t *testing.T) {
	if got := frontierDetail(&operatorapi.RunStats{GoalID: "gs_supported_frontier"}); !strings.Contains(got, "gs_supported_frontier") {
		t.Fatalf("frontier detail = %q", got)
	}
	if got := frontierDetail(nil); strings.TrimSpace(got) == "" {
		t.Fatal("frontier detail for a goal without an id is empty")
	}
}
