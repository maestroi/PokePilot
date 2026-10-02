package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

type config struct {
	BotToken       string
	AllowedUsers   map[int64]struct{}
	AllowedChats   map[int64]struct{}
	NotifyChats    []int64
	OperatorURL    string
	ReplayURL      string
	Alertmanager   string
	AdminBaseURL   string
	SpectatorBase  string
	HTTPAddr       string
	EventInterval  time.Duration
	StallAfter     time.Duration
	UpdateTimeout  time.Duration
	ControlSpacing time.Duration
}

type telegramUser struct {
	ID int64 `json:"id"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramMessage struct {
	MessageID int64        `json:"message_id"`
	From      telegramUser `json:"from"`
	Chat      telegramChat `json:"chat"`
	Text      string       `json:"text"`
}

type callbackQuery struct {
	ID      string          `json:"id"`
	From    telegramUser    `json:"from"`
	Message telegramMessage `json:"message"`
	Data    string          `json:"data"`
}

type telegramUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *telegramMessage `json:"message,omitempty"`
	CallbackQuery *callbackQuery   `json:"callback_query,omitempty"`
}

type telegramResponse[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Result      T      `json:"result"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type telegramClient struct {
	token string
	http  *http.Client
}

type confirmation struct {
	ActorID int64
	ChatID  int64
	Action  string
	Target  string
	Expires time.Time
}

type runWatch struct {
	Status        string
	Reason        string
	Frame         uint64
	LastProgress  time.Time
	StallNotified bool
}

type alertWatch struct {
	State string
	Name  string
}

type mediaWatch struct {
	State string
	RunID string
	Mode  string
}

type metrics struct {
	updates       atomic.Uint64
	commands      atomic.Uint64
	unauthorized  atomic.Uint64
	telegramErrs  atomic.Uint64
	operatorErrs  atomic.Uint64
	notifications atomic.Uint64
	actions       atomic.Uint64
	actionErrs    atomic.Uint64
	wallHealthy   atomic.Int64
	lastPollUnix  atomic.Int64
}

type bot struct {
	cfg config
	tg  *telegramClient
	op  *operatorapi.Client
	m   metrics

	mu            sync.Mutex
	confirmations map[string]confirmation
	lastControl   map[int64]time.Time
	runs          map[string]runWatch
	alerts        map[string]alertWatch
	mediaJobs     map[string]mediaWatch
	wallSeeded    bool
	alertSeeded   bool
	mediaSeeded   bool
	wallDown      bool
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC | log.Lmicroseconds)
	var showConfig bool
	flag.BoolVar(&showConfig, "check-config", false, "validate configuration and exit")
	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if showConfig {
		log.Printf("poketelegram: configuration valid; authorized_users=%d authorized_chats=%d notification_chats=%d", len(cfg.AllowedUsers), len(cfg.AllowedChats), len(cfg.NotifyChats))
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	b := &bot{
		cfg:           cfg,
		tg:            &telegramClient{token: cfg.BotToken, http: &http.Client{Timeout: 65 * time.Second}},
		op:            operatorapi.New(cfg.OperatorURL, cfg.ReplayURL, cfg.Alertmanager),
		confirmations: make(map[string]confirmation),
		lastControl:   make(map[int64]time.Time),
		runs:          make(map[string]runWatch),
		alerts:        make(map[string]alertWatch),
		mediaJobs:     make(map[string]mediaWatch),
	}
	b.m.wallHealthy.Store(1)

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: b.httpHandler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("poketelegram: health/metrics listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("poketelegram: http server: %v", err)
			stop()
		}
	}()
	go b.monitor(ctx)

	if len(cfg.AllowedUsers) == 0 && len(cfg.AllowedChats) == 0 {
		log.Printf("poketelegram: warning: no allowlist configured; bot is default-deny")
	}
	if err := b.poll(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("poketelegram: poll loop stopped: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func loadConfig() (config, error) {
	cfg := config{
		BotToken:       secretEnv("TELEGRAM_BOT_TOKEN"),
		AllowedUsers:   parseIDSet(os.Getenv("TELEGRAM_ALLOWED_USER_IDS")),
		AllowedChats:   parseIDSet(os.Getenv("TELEGRAM_ALLOWED_CHAT_IDS")),
		NotifyChats:    parseIDList(os.Getenv("TELEGRAM_NOTIFY_CHAT_IDS")),
		OperatorURL:    envDefault("POKEPILOT_OPERATOR_URL", "http://wall:8080"),
		ReplayURL:      strings.TrimSpace(os.Getenv("POKEPILOT_REPLAY_URL")),
		Alertmanager:   strings.TrimSpace(os.Getenv("POKEPILOT_ALERTMANAGER_URL")),
		AdminBaseURL:   strings.TrimRight(envDefault("POKEPILOT_ADMIN_BASE_URL", "https://admin.rompilot.app"), "/"),
		SpectatorBase:  strings.TrimRight(envDefault("POKEPILOT_SPECTATOR_BASE_URL", "https://rompilot.app/runs"), "/"),
		HTTPAddr:       envDefault("POKETELEGRAM_HTTP_ADDR", ":8080"),
		EventInterval:  envDuration("POKETELEGRAM_EVENT_INTERVAL", 15*time.Second),
		StallAfter:     envDuration("POKETELEGRAM_STALL_AFTER", 15*time.Minute),
		UpdateTimeout:  envDuration("POKETELEGRAM_UPDATE_TIMEOUT", 50*time.Second),
		ControlSpacing: envDuration("POKETELEGRAM_CONTROL_SPACING", 2*time.Second),
	}
	if cfg.BotToken == "" {
		return config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	// Backward-compatible with the existing farm-watch deployment: a legacy
	// private chat is both authorized and a notification destination.
	if legacy := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")); legacy != "" {
		id, err := strconv.ParseInt(legacy, 10, 64)
		if err != nil {
			return config{}, fmt.Errorf("TELEGRAM_CHAT_ID: %w", err)
		}
		cfg.AllowedChats[id] = struct{}{}
		cfg.NotifyChats = appendUniqueID(cfg.NotifyChats, id)
	}
	if cfg.EventInterval < time.Second {
		return config{}, errors.New("POKETELEGRAM_EVENT_INTERVAL must be at least 1s")
	}
	if cfg.StallAfter < time.Minute {
		return config{}, errors.New("POKETELEGRAM_STALL_AFTER must be at least 1m")
	}
	if cfg.UpdateTimeout < 5*time.Second || cfg.UpdateTimeout > 60*time.Second {
		return config{}, errors.New("POKETELEGRAM_UPDATE_TIMEOUT must be between 5s and 60s")
	}
	return cfg, nil
}

func (b *bot) poll(ctx context.Context) error {
	var offset int64
	for ctx.Err() == nil {
		updates, err := b.tg.getUpdates(ctx, offset, b.cfg.UpdateTimeout)
		if err != nil {
			b.m.telegramErrs.Add(1)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("poketelegram: getUpdates: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, update := range updates {
			b.m.updates.Add(1)
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if update.Message != nil {
				b.handleMessage(ctx, *update.Message)
			}
			if update.CallbackQuery != nil {
				b.handleCallback(ctx, *update.CallbackQuery)
			}
		}
	}
	return ctx.Err()
}

func (b *bot) handleMessage(ctx context.Context, msg telegramMessage) {
	if !b.authorized(msg.From.ID, msg.Chat.ID) {
		b.m.unauthorized.Add(1)
		b.audit(msg.From.ID, msg.Chat.ID, "unauthorized", "", "denied", nil)
		return
	}
	cmd, arg := parseCommand(msg.Text)
	if cmd == "" {
		return
	}
	b.m.commands.Add(1)

	var text string
	var keyboard *inlineKeyboard
	var err error
	switch cmd {
	case "start", "help":
		text = helpText()
	case "status":
		text, err = b.statusText(ctx)
	case "runs":
		text, err = b.runsText(ctx)
	case "run":
		if arg == "" {
			text = "Usage: /run <run-id>"
			break
		}
		text, keyboard, err = b.runText(ctx, arg)
	case "failures":
		text, err = b.failuresText(ctx)
	case "alerts":
		text, err = b.alertsText(ctx)
	case "triage":
		if arg == "" {
			text = "Usage: /triage <run-id>"
			break
		}
		text, err = b.runTriage(ctx, msg.From.ID, msg.Chat.ID, arg)
	case "replay":
		if arg == "" {
			text = "Usage: /replay <run-id>"
			break
		}
		text, err = b.queueReplay(ctx, msg.From.ID, msg.Chat.ID, arg)
	case "stop", "restart":
		if arg == "" {
			text = "Usage: /" + cmd + " <run-id>"
			break
		}
		text, keyboard, err = b.askConfirmation(ctx, msg.From.ID, msg.Chat.ID, cmd, arg)
	default:
		text = "Unknown command. Use /help."
	}
	if err != nil {
		b.m.operatorErrs.Add(1)
		text = "Operator request failed: " + clip(err.Error(), 500)
	}
	if text != "" {
		if sendErr := b.tg.sendMessage(ctx, msg.Chat.ID, text, keyboard); sendErr != nil {
			b.m.telegramErrs.Add(1)
			log.Printf("poketelegram: sendMessage: %v", sendErr)
		}
	}
	if cmd == "run" && err == nil && arg != "" {
		b.sendRunFrame(ctx, msg.Chat.ID, arg)
	}
}

func (b *bot) handleCallback(ctx context.Context, cb callbackQuery) {
	_ = b.tg.answerCallback(ctx, cb.ID)
	if !b.authorized(cb.From.ID, cb.Message.Chat.ID) {
		b.m.unauthorized.Add(1)
		return
	}
	parts := strings.Split(cb.Data, ":")
	if len(parts) != 2 {
		return
	}
	if parts[0] == "cancel" {
		b.mu.Lock()
		delete(b.confirmations, parts[1])
		b.mu.Unlock()
		_ = b.tg.sendMessage(ctx, cb.Message.Chat.ID, "Action cancelled.", nil)
		return
	}
	if parts[0] != "confirm" {
		return
	}
	token := parts[1]
	b.mu.Lock()
	pending, ok := b.confirmations[token]
	if ok {
		delete(b.confirmations, token)
	}
	b.mu.Unlock()
	if !ok || pending.Expires.Before(time.Now()) || pending.ActorID != cb.From.ID || pending.ChatID != cb.Message.Chat.ID {
		_ = b.tg.sendMessage(ctx, cb.Message.Chat.ID, "That confirmation expired or belongs to another operator.", nil)
		return
	}
	if !b.controlAllowed(cb.From.ID) {
		_ = b.tg.sendMessage(ctx, cb.Message.Chat.ID, "Control action rate-limited; try again in a moment.", nil)
		return
	}

	b.m.actions.Add(1)
	var text string
	var err error
	switch pending.Action {
	case "stop":
		err = b.op.Stop(ctx, pending.Target)
		if err == nil {
			text = "Stop requested for " + pending.Target + "."
		}
	case "restart":
		var result operatorapi.RestartResult
		result, err = b.op.Restart(ctx, pending.Target)
		if err == nil {
			text = fmt.Sprintf("Restarted %s as %s (%s).", pending.Target, result.RunID, result.Method)
			if result.Checkpoint != "" {
				text += " Checkpoint: " + result.Checkpoint
			}
		}
	default:
		err = errors.New("unknown confirmation action")
	}
	if err != nil {
		b.m.actionErrs.Add(1)
		text = "Action failed: " + clip(err.Error(), 500)
		b.audit(cb.From.ID, cb.Message.Chat.ID, pending.Action, pending.Target, "error", err)
	} else {
		b.audit(cb.From.ID, cb.Message.Chat.ID, pending.Action, pending.Target, "ok", nil)
	}
	_ = b.tg.sendMessage(ctx, cb.Message.Chat.ID, text, nil)
}

func (b *bot) askConfirmation(ctx context.Context, actor, chat int64, action, runID string) (string, *inlineKeyboard, error) {
	inspection, err := b.op.Run(ctx, runID)
	if err != nil {
		return "", nil, err
	}
	token := randomToken()
	b.mu.Lock()
	b.pruneConfirmationsLocked(time.Now())
	b.confirmations[token] = confirmation{ActorID: actor, ChatID: chat, Action: action, Target: runID, Expires: time.Now().Add(2 * time.Minute)}
	b.mu.Unlock()
	verb := strings.ToUpper(action[:1]) + action[1:]
	text := fmt.Sprintf("%s run %s?\n%s · %s · frame %d\nGoal: %s", verb, runID, emptyDash(inspection.Run.Game), emptyDash(inspection.Run.Status), inspection.Run.Frame, emptyDash(runGoal(inspection.Run)))
	keyboard := &inlineKeyboard{InlineKeyboard: [][]inlineButton{{
		{Text: "Confirm " + action, CallbackData: "confirm:" + token},
		{Text: "Cancel", CallbackData: "cancel:" + token},
	}}}
	return text, keyboard, nil
}

func (b *bot) statusText(ctx context.Context) (string, error) {
	dash, err := b.op.Dashboard(ctx, false, 100)
	if err != nil {
		return "", err
	}
	active, queued, failed := 0, 0, 0
	for _, run := range dash.Runs {
		switch run.Status {
		case "running", "leased":
			active++
		case "queued":
			queued++
		case "done":
			if !successfulReason(run.Reason) && run.Reason != "" {
				failed++
			}
		}
	}
	triage, triageErr := b.op.Triage(ctx)
	if triageErr != nil {
		triage = nil
	}
	replay := "not configured"
	if health, healthErr := b.op.ReplayHealth(ctx); healthErr == nil {
		replay = fmt.Sprintf("%s, %d active render(s)", emptyDash(health.Status), health.ActiveRenders)
	} else if !errors.Is(healthErr, operatorapi.ErrNotConfigured) {
		replay = "unreachable"
	}
	return fmt.Sprintf("🟢 PokePilot operator\nWall: reachable%s\nRuns: %d active · %d queued · %d recent failures\nWorkers: %d\nTriage groups: %d\nReplay: %s", versionSuffix(dash.WallVersion), active, queued, failed, len(dash.Workers), len(triage), replay), nil
}

func (b *bot) runsText(ctx context.Context) (string, error) {
	dash, err := b.op.Dashboard(ctx, true, 50)
	if err != nil {
		return "", err
	}
	if len(dash.Runs) == 0 {
		return "No active runs.", nil
	}
	lines := []string{fmt.Sprintf("Active runs (%d):", len(dash.Runs))}
	for _, run := range dash.Runs {
		badges := 0
		if run.Player != nil {
			badges = len(run.Player.Badges)
		}
		lines = append(lines, fmt.Sprintf("• %s · %s · %s · seed %d · frame %d · %d badge(s)\n  %s", run.RunID, emptyDash(run.Game), run.Status, run.Seed, run.Frame, badges, clip(runGoal(run), 140)))
		if len(lines) >= 21 {
			lines = append(lines, "…more runs omitted")
			break
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (b *bot) runText(ctx context.Context, runID string) (string, *inlineKeyboard, error) {
	inspection, err := b.op.Run(ctx, runID)
	if err != nil {
		return "", nil, err
	}
	run := inspection.Run
	lines := []string{
		fmt.Sprintf("🎮 %s", run.RunID),
		fmt.Sprintf("%s · %s · seed %d · attempt %d", emptyDash(run.Game), emptyDash(run.Status), run.Seed, run.Attempts),
		fmt.Sprintf("Frame %d · map %d @ %d,%d · %d maps visited", run.Frame, run.Map, run.X, run.Y, run.MapsVisited),
		"Goal: " + emptyDash(runGoal(run)),
	}
	if run.Decision != "" {
		lines = append(lines, "Decision: "+clip(run.Decision, 320))
	} else if run.Question != "" {
		lines = append(lines, "State: thinking / waiting for planner")
	}
	if run.Player != nil {
		lines = append(lines, formatPlayer(run.Player))
	}
	if run.Reason != "" {
		lines = append(lines, "Result: "+run.Reason+detailSuffix(run.Detail))
	}
	if inspection.Finish != nil && len(inspection.Finish.Trace) > 0 {
		trace := inspection.Finish.Trace
		if len(trace) > 3 {
			trace = trace[len(trace)-3:]
		}
		lines = append(lines, "Recent trace:")
		for _, entry := range trace {
			lines = append(lines, "• "+clip(strings.TrimSpace(entry), 240))
		}
	} else if strings.TrimSpace(run.Trace) != "" {
		lines = append(lines, "Trace: "+clip(strings.TrimSpace(run.Trace), 320))
	}
	if group, groupErr := b.op.FindTriageForRun(ctx, runID); groupErr == nil && group != nil {
		lines = append(lines, fmt.Sprintf("Triage: %s (x%d, key %s)", clip(firstNonEmpty(group.Pattern, group.Example), 180), group.Count, group.Key))
	}
	keyboard := &inlineKeyboard{InlineKeyboard: [][]inlineButton{
		{{Text: "Open admin", URL: b.adminRunURL(runID)}, {Text: "Spectator", URL: b.spectatorRunURL(runID)}},
	}}
	return strings.Join(lines, "\n"), keyboard, nil
}

func (b *bot) failuresText(ctx context.Context) (string, error) {
	groups, err := b.op.Triage(ctx)
	if err != nil {
		return "", err
	}
	if len(groups) == 0 {
		return "No active failure groups.", nil
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
	lines := []string{fmt.Sprintf("Failure groups (%d):", len(groups))}
	for i, group := range groups {
		if i >= 12 {
			lines = append(lines, "…more groups omitted")
			break
		}
		run := ""
		if len(group.RunIDs) > 0 {
			run = " · " + group.RunIDs[0]
		}
		issue := ""
		if group.Issue != nil && group.Issue.IssueNumber > 0 {
			issue = fmt.Sprintf(" · issue #%d %s", group.Issue.IssueNumber, group.Issue.Status)
		}
		lines = append(lines, fmt.Sprintf("• x%d %s%s%s\n  key %s", group.Count, clip(firstNonEmpty(group.Pattern, group.Example), 180), run, issue, group.Key))
	}
	return strings.Join(lines, "\n"), nil
}

func (b *bot) alertsText(ctx context.Context) (string, error) {
	alerts, err := b.op.Alerts(ctx)
	if errors.Is(err, operatorapi.ErrNotConfigured) {
		return "Alertmanager is not configured for this bot.", nil
	}
	if err != nil {
		return "", err
	}
	active := make([]operatorapi.Alert, 0, len(alerts))
	for _, alert := range alerts {
		if strings.EqualFold(alert.Status.State, "active") {
			active = append(active, alert)
		}
	}
	if len(active) == 0 {
		return "No active Alertmanager alerts.", nil
	}
	lines := []string{fmt.Sprintf("Active alerts (%d):", len(active))}
	for i, alert := range active {
		if i >= 15 {
			lines = append(lines, "…more alerts omitted")
			break
		}
		lines = append(lines, "• "+formatAlert(alert))
	}
	return strings.Join(lines, "\n"), nil
}

func (b *bot) runTriage(ctx context.Context, actor, chat int64, runID string) (string, error) {
	group, err := b.op.FindTriageForRun(ctx, runID)
	if err != nil {
		b.audit(actor, chat, "triage", runID, "error", err)
		return "", err
	}
	if group == nil {
		return "No actionable triage group is linked to " + runID + ".", nil
	}
	b.m.actions.Add(1)
	if err := b.op.Investigate(ctx, group.Key); err != nil {
		b.m.actionErrs.Add(1)
		b.audit(actor, chat, "triage", runID, "error", err)
		return "", err
	}
	b.audit(actor, chat, "triage", runID, "ok", nil)
	return fmt.Sprintf("Triage queued for %s.\n%s (x%d, key %s)", runID, clip(firstNonEmpty(group.Pattern, group.Example), 250), group.Count, group.Key), nil
}

func (b *bot) queueReplay(ctx context.Context, actor, chat int64, runID string) (string, error) {
	b.m.actions.Add(1)
	if err := b.op.QueueReplay(ctx, runID); err != nil {
		b.m.actionErrs.Add(1)
		b.audit(actor, chat, "queue_replay", runID, "error", err)
		if errors.Is(err, operatorapi.ErrNotConfigured) {
			return "Replay service is not configured for this bot.", nil
		}
		return "", err
	}
	b.audit(actor, chat, "queue_replay", runID, "ok", nil)
	return "Replay render queued for " + runID + ".", nil
}

func (b *bot) sendRunFrame(ctx context.Context, chat int64, runID string) {
	frameCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, mediaType, err := b.op.Frame(frameCtx, runID)
	if err != nil || len(data) == 0 {
		return
	}
	if err := b.tg.sendPhoto(frameCtx, chat, data, mediaType, "Latest frame · "+runID); err != nil {
		b.m.telegramErrs.Add(1)
	}
}

func (b *bot) monitor(ctx context.Context) {
	ticker := time.NewTicker(b.cfg.EventInterval)
	defer ticker.Stop()
	b.monitorOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.monitorOnce(ctx)
		}
	}
}

func (b *bot) monitorOnce(ctx context.Context) {
	dash, err := b.op.Dashboard(ctx, false, 200)
	if err != nil {
		b.m.operatorErrs.Add(1)
		b.m.wallHealthy.Store(0)
		if !b.wallDown {
			b.wallDown = true
			b.notifyAll(ctx, "🔴 PokePilot wall/operator API is unreachable: "+clip(err.Error(), 300))
		}
	} else {
		b.m.wallHealthy.Store(1)
		b.m.lastPollUnix.Store(time.Now().Unix())
		if b.wallDown {
			b.wallDown = false
			b.notifyAll(ctx, "✅ PokePilot wall/operator API recovered")
		}
		b.observeRuns(ctx, dash.Runs)
		b.observeRenderJobs(ctx)
	}
	b.observeAlerts(ctx)
}

func (b *bot) observeRuns(ctx context.Context, runs []operatorapi.Run) {
	now := time.Now()
	current := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		current[run.RunID] = struct{}{}
		prev, seen := b.runs[run.RunID]
		active := isActive(run.Status)
		if !seen {
			prev = runWatch{Status: run.Status, Reason: run.Reason, Frame: run.Frame, LastProgress: now}
			b.runs[run.RunID] = prev
			continue
		}
		if run.Frame != prev.Frame {
			prev.Frame = run.Frame
			prev.LastProgress = now
			prev.StallNotified = false
		}
		if active && prev.LastProgress.IsZero() {
			prev.LastProgress = now
		}
		if active && !prev.StallNotified && now.Sub(prev.LastProgress) >= b.cfg.StallAfter {
			prev.StallNotified = true
			b.notifyRun(ctx, "🟠", "stalled", run, fmt.Sprintf("No frame progress for %s", durationShort(now.Sub(prev.LastProgress))))
		}
		if prev.Status != run.Status && run.Status == "done" {
			kind, icon := "finished", "✅"
			if !successfulReason(run.Reason) {
				kind, icon = "failed", "🔴"
			}
			b.notifyRun(ctx, icon, kind, run, run.Reason+detailSuffix(run.Detail))
		}
		prev.Status = run.Status
		prev.Reason = run.Reason
		b.runs[run.RunID] = prev
	}
	if !b.wallSeeded {
		b.wallSeeded = true
	}
	for id := range b.runs {
		if _, ok := current[id]; !ok {
			delete(b.runs, id)
		}
	}
}

func (b *bot) observeRenderJobs(ctx context.Context) {
	jobs, err := b.op.MediaRenderJobs(ctx, 100)
	if err != nil {
		b.m.operatorErrs.Add(1)
		return
	}
	current := make(map[string]mediaWatch, len(jobs.Jobs))
	for _, job := range jobs.Jobs {
		current[job.ID] = mediaWatch{State: job.State, RunID: job.RunID, Mode: job.Mode}
		prev, seen := b.mediaJobs[job.ID]
		if !b.mediaSeeded || (seen && prev.State == job.State) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(job.State)) {
		case "ready":
			extra := ""
			if job.ResultSize > 0 {
				extra = fmt.Sprintf(" · %.1f MiB", float64(job.ResultSize)/(1024*1024))
			}
			b.notifyAll(ctx, fmt.Sprintf("✅ Replay ready for %s (%s)%s\n%s", job.RunID, emptyDash(job.Mode), extra, b.adminRunURL(job.RunID)))
		case "failed":
			detail := firstNonEmpty(job.LastError, job.FailureClass, "render failed")
			b.notifyAll(ctx, fmt.Sprintf("🔴 Replay failed for %s (%s)\n%s\n%s", job.RunID, emptyDash(job.Mode), clip(detail, 400), b.adminRunURL(job.RunID)))
		}
	}
	b.mediaJobs = current
	b.mediaSeeded = true
}

func (b *bot) observeAlerts(ctx context.Context) {
	alerts, err := b.op.Alerts(ctx)
	if errors.Is(err, operatorapi.ErrNotConfigured) {
		return
	}
	if err != nil {
		b.m.operatorErrs.Add(1)
		return
	}
	current := make(map[string]alertWatch, len(alerts))
	for _, alert := range alerts {
		key := alert.Fingerprint
		if key == "" {
			key = alertKey(alert)
		}
		state := strings.ToLower(alert.Status.State)
		name := alert.Labels["alertname"]
		current[key] = alertWatch{State: state, Name: name}
		prev, seen := b.alerts[key]
		if b.alertSeeded && (!seen || prev.State != state) {
			if state == "active" {
				b.notifyAll(ctx, "🔴 Alertmanager: "+formatAlert(alert))
			} else if prev.State == "active" {
				b.notifyAll(ctx, "✅ Alert resolved: "+firstNonEmpty(name, key))
			}
		}
	}
	if b.alertSeeded {
		for key, prev := range b.alerts {
			if prev.State != "active" {
				continue
			}
			if _, ok := current[key]; !ok {
				b.notifyAll(ctx, "✅ Alert resolved: "+firstNonEmpty(prev.Name, key))
			}
		}
	}
	b.alerts = current
	b.alertSeeded = true
}

func (b *bot) notifyRun(ctx context.Context, icon, event string, run operatorapi.Run, extra string) {
	text := fmt.Sprintf("%s %s %s\n%s · seed %d · frame %d\nGoal: %s", icon, run.RunID, event, emptyDash(run.Game), run.Seed, run.Frame, emptyDash(runGoal(run)))
	if strings.TrimSpace(extra) != "" {
		text += "\n" + clip(strings.TrimSpace(extra), 400)
	}
	if run.Player != nil {
		text += fmt.Sprintf("\nProgress: %d badge(s), %d owned", len(run.Player.Badges), run.Player.DexOwned)
	}
	text += "\n" + b.adminRunURL(run.RunID)
	b.notifyAll(ctx, text)
}

func (b *bot) notifyAll(ctx context.Context, text string) {
	for _, chat := range b.cfg.NotifyChats {
		if err := b.tg.sendMessage(ctx, chat, text, nil); err != nil {
			b.m.telegramErrs.Add(1)
			log.Printf("poketelegram: notification to %d: %v", chat, err)
			continue
		}
		b.m.notifications.Add(1)
	}
}

func (b *bot) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := "ok"
		code := http.StatusOK
		if b.m.wallHealthy.Load() == 0 {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":             status,
			"wall_healthy":       b.m.wallHealthy.Load() == 1,
			"last_operator_poll": b.m.lastPollUnix.Load(),
			"authorized_users":   len(b.cfg.AllowedUsers),
			"authorized_chats":   len(b.cfg.AllowedChats),
			"notification_chats": len(b.cfg.NotifyChats),
		})
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		b.mu.Lock()
		pending := len(b.confirmations)
		b.mu.Unlock()
		fmt.Fprintf(w, "poketelegram_updates_total %d\n", b.m.updates.Load())
		fmt.Fprintf(w, "poketelegram_commands_total %d\n", b.m.commands.Load())
		fmt.Fprintf(w, "poketelegram_unauthorized_total %d\n", b.m.unauthorized.Load())
		fmt.Fprintf(w, "poketelegram_telegram_errors_total %d\n", b.m.telegramErrs.Load())
		fmt.Fprintf(w, "poketelegram_operator_errors_total %d\n", b.m.operatorErrs.Load())
		fmt.Fprintf(w, "poketelegram_notifications_total %d\n", b.m.notifications.Load())
		fmt.Fprintf(w, "poketelegram_actions_total %d\n", b.m.actions.Load())
		fmt.Fprintf(w, "poketelegram_action_errors_total %d\n", b.m.actionErrs.Load())
		fmt.Fprintf(w, "poketelegram_pending_confirmations %d\n", pending)
		fmt.Fprintf(w, "poketelegram_wall_healthy %d\n", b.m.wallHealthy.Load())
		fmt.Fprintf(w, "poketelegram_last_operator_poll_timestamp_seconds %d\n", b.m.lastPollUnix.Load())
	})
	return mux
}

func (b *bot) authorized(userID, chatID int64) bool {
	if _, ok := b.cfg.AllowedUsers[userID]; ok {
		return true
	}
	_, ok := b.cfg.AllowedChats[chatID]
	return ok
}

func (b *bot) controlAllowed(actor int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if last := b.lastControl[actor]; !last.IsZero() && now.Sub(last) < b.cfg.ControlSpacing {
		return false
	}
	b.lastControl[actor] = now
	return true
}

func (b *bot) pruneConfirmationsLocked(now time.Time) {
	for token, pending := range b.confirmations {
		if pending.Expires.Before(now) {
			delete(b.confirmations, token)
		}
	}
}

func (b *bot) audit(actor, chat int64, action, target, outcome string, err error) {
	entry := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"actor_id":  actor,
		"chat_id":   chat,
		"action":    action,
		"target":    target,
		"outcome":   outcome,
	}
	if err != nil {
		entry["error"] = clip(err.Error(), 500)
	}
	data, _ := json.Marshal(entry)
	log.Printf("audit %s", data)
}

func (b *bot) adminRunURL(runID string) string {
	return b.cfg.AdminBaseURL + "/runs/" + url.PathEscape(runID)
}

func (b *bot) spectatorRunURL(runID string) string {
	return b.cfg.SpectatorBase + "/" + url.PathEscape(runID)
}

func (t *telegramClient) getUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]telegramUpdate, error) {
	form := url.Values{}
	form.Set("offset", strconv.FormatInt(offset, 10))
	form.Set("timeout", strconv.Itoa(int(timeout.Seconds())))
	form.Set("allowed_updates", `["message","callback_query"]`)
	var out telegramResponse[[]telegramUpdate]
	if err := t.callForm(ctx, "getUpdates", form, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, errors.New(out.Description)
	}
	return out.Result, nil
}

func (t *telegramClient) sendMessage(ctx context.Context, chatID int64, text string, keyboard *inlineKeyboard) error {
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     clip(text, 3900),
		"disable_web_page_preview": true,
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
	var out telegramResponse[json.RawMessage]
	if err := t.callJSON(ctx, "sendMessage", payload, &out); err != nil {
		return err
	}
	if !out.OK {
		return errors.New(out.Description)
	}
	return nil
}

func (t *telegramClient) sendPhoto(ctx context.Context, chatID int64, data []byte, mediaType, caption string) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	_ = mw.WriteField("caption", clip(caption, 900))
	part, err := mw.CreateFormFile("photo", "frame.png")
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint("sendPhoto"), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if mediaType != "" {
		req.Header.Set("X-PokePilot-Source-Media-Type", mediaType)
	}
	res, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var out telegramResponse[json.RawMessage]
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || !out.OK {
		return fmt.Errorf("telegram sendPhoto %s: %s", res.Status, out.Description)
	}
	return nil
}

func (t *telegramClient) answerCallback(ctx context.Context, id string) error {
	var out telegramResponse[bool]
	return t.callJSON(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id}, &out)
}

func (t *telegramClient) callForm(ctx context.Context, method string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint(method), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return t.do(req, out)
}

func (t *telegramClient) callJSON(ctx context.Context, method string, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint(method), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return t.do(req, out)
}

func (t *telegramClient) do(req *http.Request, out any) error {
	res, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 32<<10))
		return fmt.Errorf("telegram API %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	return json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out)
}

func (t *telegramClient) endpoint(method string) string {
	return "https://api.telegram.org/bot" + t.token + "/" + method
}

func parseCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}
	cmd := strings.TrimPrefix(fields[0], "/")
	if at := strings.IndexByte(cmd, '@'); at >= 0 {
		cmd = cmd[:at]
	}
	arg := ""
	if len(fields) > 1 {
		arg = strings.TrimSpace(strings.Join(fields[1:], " "))
	}
	return strings.ToLower(cmd), arg
}

func helpText() string {
	return strings.Join([]string{
		"PokePilot remote operator",
		"/status — farm, queue, worker and replay summary",
		"/runs — active runs",
		"/run <id> — run state, progress, party and latest frame",
		"/failures — active failure/triage groups",
		"/alerts — active Alertmanager alerts",
		"/triage <id> — queue the existing investigation for a run",
		"/replay <id> — queue a replay render",
		"/stop <id> — confirmed cooperative stop",
		"/restart <id> — confirmed restart from latest replayable checkpoint, else fresh clone",
	}, "\n")
}

func runGoal(run operatorapi.Run) string {
	if run.Stats != nil && strings.TrimSpace(run.Stats.GoalSummary) != "" {
		return strings.TrimSpace(run.Stats.GoalSummary)
	}
	return strings.TrimSpace(run.Goal)
}

func formatPlayer(player *operatorapi.Player) string {
	if player == nil {
		return ""
	}
	parts := make([]string, 0, len(player.Party))
	for _, mon := range player.Party {
		item := fmt.Sprintf("%s Lv%d", mon.Name, mon.Level)
		if mon.MaxHP > 0 {
			item += fmt.Sprintf(" %d/%dHP", mon.HP, mon.MaxHP)
		}
		parts = append(parts, item)
	}
	line := fmt.Sprintf("Progress: %d badge(s) · Pokédex %d owned/%d seen", len(player.Badges), player.DexOwned, player.DexSeen)
	if len(parts) > 0 {
		line += "\nParty: " + strings.Join(parts, ", ")
	}
	return line
}

func formatAlert(alert operatorapi.Alert) string {
	name := firstNonEmpty(alert.Labels["alertname"], "unnamed alert")
	severity := alert.Labels["severity"]
	summary := firstNonEmpty(alert.Annotations["summary"], alert.Annotations["description"])
	out := name
	if severity != "" {
		out += " [" + severity + "]"
	}
	if summary != "" {
		out += ": " + clip(summary, 260)
	}
	return out
}

func alertKey(alert operatorapi.Alert) string {
	keys := make([]string, 0, len(alert.Labels))
	for key := range alert.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(alert.Labels[key])
		b.WriteByte(';')
	}
	return b.String()
}

func successfulReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "goal", "goal_complete", "goal-complete", "done", "completed", "success":
		return true
	default:
		return false
	}
}

func isActive(status string) bool {
	return status == "running" || status == "leased" || status == "queued"
}

func detailSuffix(detail string) string {
	if strings.TrimSpace(detail) == "" {
		return ""
	}
	return " · " + clip(strings.TrimSpace(detail), 280)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return strings.TrimSpace(value)
}

func versionSuffix(version string) string {
	if strings.TrimSpace(version) == "" {
		return ""
	}
	return " (" + clip(version, 16) + ")"
}

func durationShort(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return strings.TrimSpace(s[:max-1]) + "…"
}

func randomToken() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf[:])
}

func parseIDSet(raw string) map[int64]struct{} {
	out := make(map[int64]struct{})
	for _, id := range parseIDList(raw) {
		out[id] = struct{}{}
	}
	return out
}

func parseIDList(raw string) []int64 {
	var out []int64
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' }) {
		id, err := strconv.ParseInt(strings.TrimSpace(field), 10, 64)
		if err == nil {
			out = appendUniqueID(out, id)
		}
	}
	return out
}

func appendUniqueID(ids []int64, id int64) []int64 {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func secretEnv(name string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	path := strings.TrimSpace(os.Getenv(name + "_FILE"))
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if value, err := time.ParseDuration(raw); err == nil {
			return value
		}
	}
	return fallback
}
