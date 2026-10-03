package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func TestOpsEndpointRequiresTokenAndStoresSnapshot(t *testing.T) {
	b, _ := uiBot(t, nil)
	b.cfg.OpsToken = "tok"
	body, _ := json.Marshal(operatorapi.OpsSnapshot{At: 5})
	rec := httptest.NewRecorder()
	b.httpHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/ops", bytes.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/ops", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	b.httpHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || b.opsSnapshot() == nil || b.opsSnapshot().At != 5 {
		t.Fatalf("stored: %d %+v", rec.Code, b.opsSnapshot())
	}
}

func TestLocalChecksWallStallAndWatcherSilence(t *testing.T) {
	b, _ := uiBot(t, nil)
	now := time.Now()
	b.started = now.Add(-10 * time.Minute)
	b.runs["r1"] = runWatch{Status: "running", Frame: 10, LastProgress: now.Add(-20 * time.Minute)}
	got := map[string]operatorapi.CheckResult{}
	for _, c := range b.localChecks([]operatorapi.Run{{RunID: "r1", Status: "running", Frame: 10}}, nil, now) {
		got[c.Name] = c
	}
	if c := got["stall:r1"]; c.OK || c.RunID != "r1" {
		t.Fatalf("stall: %+v", c)
	}
	if c := got["watcher"]; c.OK {
		t.Fatalf("no push for 10m → watcher silent: %+v", c)
	}
	got = map[string]operatorapi.CheckResult{}
	for _, c := range b.localChecks(nil, errors.New("dial tcp: refused"), now) {
		got[c.Name] = c
	}
	if c := got["wall"]; c.OK || c.Grace != 2 {
		t.Fatalf("wall: %+v", c)
	}
}

func TestAlertCardOpenThenResolveEditsAndReplies(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	t0 := time.Now()
	b.applyAlerts(ctx, "watch", nil, t0)
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "disk:n1:/", Grace: 1, Message: "n1 / has 5G free"}}, t0)
	open := f.calls[len(f.calls)-1]
	if open["_method"] != "sendMessage" || !strings.Contains(open["text"].(string), "n1 / has 5G free") {
		t.Fatalf("open card: %+v", open)
	}
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "disk:n1:/", OK: true}}, t0.Add(34*time.Minute))
	var edited, replied bool
	for _, c := range f.calls {
		if c["_method"] == "editMessageText" && strings.Contains(c["text"].(string), "resolved after 34m") {
			edited = true
		}
		if rp, ok := c["reply_parameters"].(map[string]any); ok && rp["message_id"] == float64(42) {
			replied = true
		}
	}
	if !edited || !replied {
		t.Fatalf("resolve must edit the card and reply to it: %+v", f.calls)
	}
}

func TestRestartSummaryIsOneMessage(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	b.applyAlerts(context.Background(), "bot", nil, time.Now())
	b.applyAlerts(context.Background(), "watch", []operatorapi.CheckResult{
		{Name: "quorum", Grace: 2, Message: "2/3 managers reachable"},
		{Name: "paid-cap", Grace: 1, Message: "cap"},
	}, time.Now())
	if len(f.calls) != 1 || !strings.Contains(f.calls[0]["text"].(string), "2 checks failing") {
		t.Fatalf("one summary expected: %+v", f.calls)
	}
}

func TestStallAlertCarriesRunButtons(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	b.applyAlerts(ctx, "bot", nil, time.Now())
	b.applyAlerts(ctx, "bot", []operatorapi.CheckResult{{Name: "stall:r1", Grace: 1, RunID: "r1", Message: "r1 no frame progress for 20m"}}, time.Now())
	kb, _ := json.Marshal(f.calls[len(f.calls)-1]["reply_markup"])
	if !strings.Contains(string(kb), "act:flag:") || !strings.Contains(string(kb), "mute:") {
		t.Fatalf("stall alert buttons: %s", kb)
	}
	if b.runForMessage(1, 42) != "r1" {
		t.Fatal("alert message must support reply shortcuts")
	}
}

func TestDigestIncludesBadgeDeltaAndFixer(t *testing.T) {
	b, _ := uiBot(t, nil)
	b.ops = &operatorapi.OpsSnapshot{Merged24h: 4, FarmOpened: 2, FarmClosed: 3,
		Fixer: &operatorapi.FixerSummary{PaidStarts24h: 3, PaidCap: 20, FreeStarts24h: 5, Attempts24h: 8, PRs24h: 2}}
	b.digestBadges = map[string]int{"r1": 3}
	b.lastDash = []operatorapi.Run{{RunID: "r1", Status: "running", Game: "red", RecoveryBadges: 5}}
	text, kb := b.digestText(time.Now())
	for _, want := range []string{"PRs merged: 4", "opened 2", "closed 3", "qwen 5", "paid 3/20", "25%", "(+2)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("digest missing %q:\n%s", want, text)
		}
	}
	if kb == nil {
		t.Fatal("digest buttons")
	}
}

func TestBoardDroppedWhenMessageGone(t *testing.T) {
	b, f := uiBot(t, nil)
	b.boards[1] = 77
	f.reply = func(string, map[string]any) (int, string) {
		return 400, `{"ok":false,"description":"Bad Request: message to edit not found"}`
	}
	b.refreshBoards(context.Background())
	if _, ok := b.boards[1]; ok {
		t.Fatal("a deleted board must be forgotten, not retried forever")
	}
}

func TestWallBlipLeavesStallAlertUntouched(t *testing.T) {
	var down atomic.Bool
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/dashboard") {
			_ = json.NewEncoder(w).Encode(map[string]any{"runs": []operatorapi.Run{{RunID: "r1", Status: "running", Frame: 10}}})
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(wall.Close)
	f := newFakeTG(t)
	b := newBot(config{NotifyChats: []int64{1}, StallAfter: 15 * time.Minute}, f.client(), operatorapi.New(wall.URL, "", ""))
	ctx := context.Background()
	b.applyAlerts(ctx, "runs", nil, time.Now()) // seed
	b.runs["r1"] = runWatch{Status: "running", Frame: 10, LastProgress: time.Now().Add(-20 * time.Minute)}
	b.monitorOnce(ctx)
	if open := b.book.Open(); len(open) != 1 || open[0].Name != "stall:r1" {
		t.Fatalf("stall should be open: %+v", open)
	}
	down.Store(true)
	b.monitorOnce(ctx)
	down.Store(false)
	b.monitorOnce(ctx)
	for _, c := range f.calls {
		if c["_method"] == "editMessageText" && strings.Contains(c["text"].(string), "stall:r1") {
			t.Fatalf("wall blip resolved the stall card: %+v", c)
		}
	}
	n := 0
	for _, c := range f.calls {
		if c["_method"] == "sendMessage" && strings.Contains(c["text"].(string), "stall:r1") {
			n++
		}
	}
	if n != 1 || len(b.book.Open()) == 0 {
		t.Fatalf("stall card sent %d times, open=%d", n, len(b.book.Open()))
	}
}

func TestResolveWithCardGoneSendsFullText(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	t0 := time.Now()
	b.applyAlerts(ctx, "watch", nil, t0)
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "disk:n1:/", Grace: 1, Message: "low"}}, t0)
	errs := b.m.telegramErrs.Load()
	f.reply = func(m string, _ map[string]any) (int, string) {
		if m == "editMessageText" {
			return 400, `{"ok":false,"description":"Bad Request: message to edit not found"}`
		}
		return 200, `{"ok":true,"result":{"message_id":43}}`
	}
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "disk:n1:/", OK: true}}, t0.Add(time.Minute))
	last := f.calls[len(f.calls)-1]
	if last["_method"] != "sendMessage" || !strings.Contains(last["text"].(string), "disk:n1:/") || !strings.Contains(last["text"].(string), "resolved after") {
		t.Fatalf("full resolve text expected: %+v", last)
	}
	if b.m.telegramErrs.Load() != errs {
		t.Fatal("gone card is not a telegram error")
	}
}

func TestHealthzIsLivenessWhileWallDown(t *testing.T) {
	b, _ := uiBot(t, nil)
	b.m.wallHealthy.Store(0)
	rec := httptest.NewRecorder()
	b.httpHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body["wall_healthy"] != false || body["status"] != "degraded" {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body)
	}
}

func TestRestartSummaryCollectsAllSourcesIntoOneMessage(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	now := time.Now()
	b.started = now
	b.applyAlerts(ctx, "bot", []operatorapi.CheckResult{{Name: "wall", Grace: 2, Message: "wall down"}}, now)
	b.applyAlerts(ctx, "runs", []operatorapi.CheckResult{{Name: "stall:r1", Grace: 1, Message: "r1 stalled"}}, now)
	if len(f.calls) != 0 {
		t.Fatalf("summary must wait for the watcher or the window: %+v", f.calls)
	}
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "quorum", Grace: 2, Message: "2/3"}}, now)
	if len(f.calls) != 1 || !strings.Contains(f.calls[0]["text"].(string), "3 checks failing") {
		t.Fatalf("one summary for all sources: %+v", f.calls)
	}
	// A source first seen after the window pages its failures as cards.
	b.applyAlerts(ctx, "late", []operatorapi.CheckResult{{Name: "x", Grace: 1, Message: "late failure"}}, now)
	if len(f.calls) != 2 || !strings.Contains(f.calls[1]["text"].(string), "late failure") || f.calls[1]["reply_markup"] == nil {
		t.Fatalf("late adoption: %+v", f.calls)
	}
}

func TestRestartSummaryWindowAndEmpty(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	now := time.Now()
	b.started = now
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "quorum", Message: "2/3"}}, now)
	if len(f.calls) != 0 {
		t.Fatalf("a push before the first monitor pass must not flush: %+v", f.calls)
	}
	b.applyAlerts(ctx, "bot", nil, now)
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "quorum", OK: true}}, now)
	if len(f.calls) != 0 {
		t.Fatalf("nothing adopted: nothing posted: %+v", f.calls)
	}

	b2, f2 := uiBot(t, nil)
	b2.cfg.NotifyChats = []int64{1}
	b2.started = now
	b2.applyAlerts(ctx, "bot", []operatorapi.CheckResult{{Name: "wall", Message: "wall down"}}, now)
	b2.applyAlerts(ctx, "bot", []operatorapi.CheckResult{{Name: "wall", Message: "wall down"}}, now.Add(3*time.Minute))
	if len(f2.calls) != 1 || !strings.Contains(f2.calls[0]["text"].(string), "1 checks failing") {
		t.Fatalf("window expiry must flush the summary: %+v", f2.calls)
	}
}

func TestRestartSummaryIsCapped(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	var checks []operatorapi.CheckResult
	for i := 0; i < 30; i++ {
		checks = append(checks, operatorapi.CheckResult{Name: fmt.Sprintf("c%02d", i), Message: strings.Repeat("x", 300)})
	}
	b.applyAlerts(context.Background(), "bot", nil, time.Now())
	b.applyAlerts(context.Background(), "watch", checks, time.Now())
	text := f.calls[0]["text"].(string)
	if !strings.Contains(text, "…15 more") || strings.Count(text, "\n") > 17 {
		t.Fatalf("summary not capped (%d lines):\n%s", strings.Count(text, "\n"), text)
	}
}

func TestAlertsCardIsCapped(t *testing.T) {
	b, _ := uiBot(t, nil)
	var checks []operatorapi.CheckResult
	for i := 0; i < 30; i++ {
		checks = append(checks, operatorapi.CheckResult{Name: fmt.Sprintf("c%02d", i), Message: strings.Repeat("x", 300)})
	}
	b.alertMu.Lock()
	b.book.Observe("watch", checks, time.Now())
	b.alertMu.Unlock()
	c, _ := b.alertsCard(context.Background())
	if !strings.Contains(c.Text, "…15 more") || strings.Count(c.Text, "🔴") != 15 {
		t.Fatalf("alerts card not capped:\n%s", c.Text)
	}
}

func TestAdoptedAlertResolveNotifiesChats(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	t0 := time.Now()
	b.applyAlerts(ctx, "bot", nil, t0)
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "disk:n1:/", Message: "low"}}, t0) // adopted
	n := len(f.calls)
	b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "disk:n1:/", OK: true}}, t0.Add(time.Minute))
	if len(f.calls) != n+1 || !strings.Contains(f.calls[n]["text"].(string), "disk:n1:/") || !strings.Contains(f.calls[n]["text"].(string), "resolved after") {
		t.Fatalf("adopted resolve must notify: %+v", f.calls[n:])
	}
}

func TestFailedAlertCardIsRetried(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	ctx := context.Background()
	t0 := time.Now()
	b.applyAlerts(ctx, "watch", nil, t0)
	f.reply = func(string, map[string]any) (int, string) { return 502, `{"ok":false,"description":"Bad Gateway"}` }
	fail := []operatorapi.CheckResult{{Name: "disk:n1:/", Grace: 1, Message: "low"}}
	b.applyAlerts(ctx, "watch", fail, t0)
	f.reply = func(string, map[string]any) (int, string) { return 200, `{"ok":true,"result":{"message_id":42}}` }
	b.applyAlerts(ctx, "watch", fail, t0.Add(time.Minute))
	b.alertMu.Lock()
	cards := len(b.book.states["disk:n1:/"].Cards)
	b.alertMu.Unlock()
	if cards != 1 {
		t.Fatalf("failed page must be re-sent next tick; calls=%+v", f.calls)
	}
}

func TestColdWatchSnapshotDoesNotResolveAbsentChecks(t *testing.T) {
	b, f := uiBot(t, nil)
	b.cfg.NotifyChats = []int64{1}
	b.cfg.OpsToken = "tok"
	post := func(s operatorapi.OpsSnapshot) {
		body, _ := json.Marshal(s)
		req := httptest.NewRequest(http.MethodPost, "/v1/ops", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer tok")
		b.httpHandler().ServeHTTP(httptest.NewRecorder(), req)
	}
	post(operatorapi.OpsSnapshot{Warm: true})
	post(operatorapi.OpsSnapshot{Warm: true, Checks: []operatorapi.CheckResult{{Name: "paid-cap", Grace: 1, Message: "cap"}}})
	post(operatorapi.OpsSnapshot{Warm: false}) // restarted watcher, no fixer report yet
	for _, c := range f.calls {
		if c["_method"] == "editMessageText" {
			t.Fatalf("cold snapshot resolved paid-cap: %+v", c)
		}
	}
	if len(b.book.Open()) != 1 {
		t.Fatal("paid-cap must stay open")
	}
}

func TestBoardTextConcurrentWithAlerts(t *testing.T) {
	b, _ := uiBot(t, nil)
	ctx := context.Background()
	b.applyAlerts(ctx, "watch", nil, time.Now())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			b.applyAlerts(ctx, "watch", []operatorapi.CheckResult{{Name: "a", Grace: 1, Message: fmt.Sprint("m", i)}}, time.Now())
		}
	}()
	for i := 0; i < 200; i++ {
		_ = b.boardText()
	}
	<-done
}

func TestFlagFollowUpNeedsOperatorFlaggedGroup(t *testing.T) {
	var triage atomic.Value
	triage.Store(`[{"key":"old","pattern":"stuck in menu","example":"older failure","run_ids":["r1"],"issue":{"issue_number":7}}]`)
	wall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/triage") {
			_, _ = w.Write([]byte(triage.Load().(string)))
		}
	}))
	t.Cleanup(wall.Close)
	f := newFakeTG(t)
	b := newBot(config{GitHubRepo: "o/r"}, f.client(), operatorapi.New(wall.URL, "", ""))
	now := time.Now()
	b.flags["r1"] = flagWatch{Chat: 1, Since: now}
	b.observeFlags(context.Background(), now)
	if len(f.calls) != 0 {
		t.Fatalf("older group for the same run must not be posted: %+v", f.calls)
	}
	triage.Store(`[{"key":"old","pattern":"stuck in menu","run_ids":["r1"],"issue":{"issue_number":7}},` +
		`{"key":"new","pattern":"stuck","example":"operator flagged: looping","run_ids":["r1"],"issue":{"issue_number":9}}]`)
	b.observeFlags(context.Background(), now)
	if len(f.calls) != 1 || !strings.Contains(f.calls[0]["text"].(string), "issue #9") {
		t.Fatalf("matching group must be posted: %+v", f.calls)
	}
}
