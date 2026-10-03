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

func uiBot(t *testing.T, runs []operatorapi.Run) (*bot, *fakeTG) {
	t.Helper()
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/dashboard"):
			_ = json.NewEncoder(w).Encode(map[string]any{"runs": runs})
		case strings.HasSuffix(r.URL.Path, "/flag-stuck"):
			_ = json.NewEncoder(w).Encode(map[string]any{"flagged": true})
		case strings.HasPrefix(r.URL.Path, "/v1/runs/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/runs/")
			for _, run := range runs {
				if run.RunID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"run": run})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/v1/triage"):
			_, _ = w.Write([]byte(`[]`)) // GET /v1/triage returns a bare array
		}
	}))
	t.Cleanup(wall.Close)
	f := newFakeTG(t)
	b := newBot(config{
		AllowedUsers: map[int64]struct{}{1: {}}, AllowedChats: map[int64]struct{}{},
		AdminBaseURL: "https://admin", SpectatorBase: "https://spec", ControlSpacing: 0, StallAfter: 15 * time.Minute,
	}, f.client(), operatorapi.New(wall.URL, "", ""))
	return b, f
}

func TestRunsCardIsTappableAndNumbered(t *testing.T) {
	runs := []operatorapi.Run{
		{RunID: "red-very-long-run-id-0001-abcdef", Status: "running", Game: "red", Player: &operatorapi.Player{Badges: []string{"a", "b"}}, GameState: map[string]any{"map_name": "Route <12>"}},
		{RunID: "gold-0002", Status: "running", Game: "gold", Map: 7},
	}
	b, _ := uiBot(t, runs)
	c, err := b.runsCard(context.Background(), 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Text, "Route &lt;12&gt;") || !strings.Contains(c.Text, "map 7") {
		t.Fatalf("labels/escaping: %s", c.Text)
	}
	for _, row := range c.Keyboard.InlineKeyboard {
		for _, btn := range row {
			if len(btn.CallbackData) > 64 {
				t.Fatalf("callback too long: %q", btn.CallbackData)
			}
		}
	}
	if b.resolveRun(5, "2") != "gold-0002" || b.resolveRun(5, "gold-0002") != "gold-0002" || b.resolveRun(6, "2") != "2" {
		t.Fatal("number resolution is per chat and falls back to the literal")
	}
}

func TestReplyShortcutResolvesRunFromRepliedMessage(t *testing.T) {
	b, f := uiBot(t, []operatorapi.Run{{RunID: "r1", Status: "running", Game: "red"}})
	b.rememberRunMessage(1, 100, "r1")
	b.handleMessage(context.Background(), telegramMessage{
		MessageID: 101, From: telegramUser{ID: 1}, Chat: telegramChat{ID: 1}, Text: "flag",
		ReplyToMessage: &telegramMessage{MessageID: 100},
	})
	last := f.calls[len(f.calls)-1]
	if !strings.Contains(last["text"].(string), "Flag run") {
		t.Fatalf("reply 'flag' should open the flag confirmation: %+v", last)
	}
}

func TestFlagConfirmationAcceptsNoteByReply(t *testing.T) {
	b, f := uiBot(t, []operatorapi.Run{{RunID: "r1", Status: "running", Game: "red"}})
	text, kb, err := b.askConfirmation(context.Background(), 1, 1, "flag", "r1")
	if err != nil || !strings.Contains(text, "reply") || kb == nil {
		t.Fatalf("confirmation: %q %v", text, err)
	}
	b.rememberConfirmMessage(1, 300, kb)
	b.handleMessage(context.Background(), telegramMessage{
		MessageID: 301, From: telegramUser{ID: 1}, Chat: telegramChat{ID: 1}, Text: "stuck in the PC menu",
		ReplyToMessage: &telegramMessage{MessageID: 300},
	})
	last := f.calls[len(f.calls)-1]
	if !strings.Contains(last["text"].(string), "Flagged r1") {
		t.Fatalf("note reply should flag: %+v", last)
	}
	if _, ok := b.flags["r1"]; !ok {
		t.Fatal("flag should be watched for the issue link")
	}
}

func TestNavigationEditsInPlace(t *testing.T) {
	b, f := uiBot(t, []operatorapi.Run{{RunID: "r1", Status: "running"}})
	b.handleCallback(context.Background(), callbackQuery{ID: "q", From: telegramUser{ID: 1}, Data: "nav:runs",
		Message: telegramMessage{MessageID: 55, Chat: telegramChat{ID: 1}}})
	var edited bool
	for _, c := range f.calls {
		if c["_method"] == "editMessageText" && c["message_id"] == float64(55) {
			edited = true
		}
	}
	if !edited {
		t.Fatalf("nav must edit the tapped message: %+v", f.calls)
	}
}

func TestBadgeBar(t *testing.T) {
	// No game-specific total: just the count.
	if got := badgeBar(5); got != "🏅 5" {
		t.Fatalf("%q", got)
	}
}

func TestMuteCallbackMutesAlert(t *testing.T) {
	b, f := uiBot(t, nil)
	b.alertMu.Lock()
	b.book.Observe("watch", []operatorapi.CheckResult{{Name: "disk", OK: false, Message: "low"}}, time.Now())
	b.alertMu.Unlock()
	b.handleCallback(context.Background(), callbackQuery{ID: "q", From: telegramUser{ID: 1}, Data: "mute:" + b.handle("disk"),
		Message: telegramMessage{MessageID: 9, Chat: telegramChat{ID: 1}, Text: "disk low"}})
	b.alertMu.Lock()
	muted := b.book.Open()[0].MutedUntil
	b.alertMu.Unlock()
	if !muted.After(time.Now()) {
		t.Fatal("alert not muted")
	}
	last := f.calls[len(f.calls)-1]
	if !strings.Contains(last["text"].(string), "muted until") {
		t.Fatalf("%+v", last)
	}
}

func TestHealthAndFixerCardsWithoutData(t *testing.T) {
	b, _ := uiBot(t, nil)
	if !strings.Contains(b.healthCard().Text, "No watcher data yet.") {
		t.Fatal("health")
	}
	if !strings.Contains(b.fixerCard(context.Background()).Text, "No fixer report yet.") {
		t.Fatal("fixer")
	}
}

func TestHelpTextListsCommandsAndShortcuts(t *testing.T) {
	for _, want := range []string{"/menu", "/flag N [note]", "/frame N", "/health", "/fixer", "/board", "Reply with text to a flag confirmation"} {
		if !strings.Contains(helpText(), want) {
			t.Fatalf("help missing %q", want)
		}
	}
}

func TestOtherUserReplyDoesNotConsumeFlagConfirmation(t *testing.T) {
	b, _ := uiBot(t, []operatorapi.Run{{RunID: "r1", Status: "running", Game: "red"}})
	b.cfg.AllowedUsers[2] = struct{}{}
	_, kb, err := b.askConfirmation(context.Background(), 1, 1, "flag", "r1")
	if err != nil {
		t.Fatal(err)
	}
	b.rememberConfirmMessage(1, 300, kb)
	b.handleMessage(context.Background(), telegramMessage{
		MessageID: 301, From: telegramUser{ID: 2}, Chat: telegramChat{ID: 1}, Text: "me too",
		ReplyToMessage: &telegramMessage{MessageID: 300},
	})
	if _, ok := b.flags["r1"]; ok {
		t.Fatal("other user flagged the run")
	}
	token := strings.TrimPrefix(kb.InlineKeyboard[0][0].CallbackData, "confirm:")
	if got := b.confirm(context.Background(), 1, 1, token, ""); !strings.Contains(got, "Flagged r1") {
		t.Fatalf("owner confirmation lost: %q", got)
	}
}

func TestUnknownNavViewIsExpiredNotError(t *testing.T) {
	b, f := uiBot(t, nil)
	b.handleCallback(context.Background(), callbackQuery{ID: "q", From: telegramUser{ID: 1}, Data: "nav:bogus",
		Message: telegramMessage{MessageID: 5, Chat: telegramChat{ID: 1}}})
	if b.m.operatorErrs.Load() != 0 || !strings.Contains(f.calls[len(f.calls)-1]["text"].(string), "expired") {
		t.Fatalf("%+v", f.calls)
	}
}
