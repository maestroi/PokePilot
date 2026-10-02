package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
