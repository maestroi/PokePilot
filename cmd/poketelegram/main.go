package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
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
	GitHubRepo     string
	OpsToken       string
	DigestHour     int
}

type confirmation struct {
	ActorID int64
	ChatID  int64
	Action  string
	Target  string
	Note    string
	Expires time.Time
}

type runWatch struct {
	Status       string
	Reason       string
	Frame        uint64
	LastProgress time.Time
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

	handles     map[string]string
	lastList    map[int64][]string
	msgRuns     map[int64]map[int64]string
	confirmMsgs map[int64]map[int64]string // chat → confirmation message id → token
	samples     map[string][]runSample
	lastNewMap  map[string]time.Time
	flags       map[string]flagWatch
	ops         *operatorapi.OpsSnapshot
	opsAt       time.Time
	started     time.Time
	lastDash    []operatorapi.Run
	boards      map[int64]int64
	boardAt     time.Time

	digestBadges map[string]int
	digestDay    string

	alertMu sync.Mutex
	book    *alertBook
}

func newBot(cfg config, tg *telegramClient, op *operatorapi.Client) *bot {
	return &bot{
		cfg:           cfg,
		tg:            tg,
		op:            op,
		confirmations: make(map[string]confirmation),
		lastControl:   make(map[int64]time.Time),
		runs:          make(map[string]runWatch),
		alerts:        make(map[string]alertWatch),
		mediaJobs:     make(map[string]mediaWatch),
		handles:       make(map[string]string),
		lastList:      make(map[int64][]string),
		msgRuns:       make(map[int64]map[int64]string),
		confirmMsgs:   make(map[int64]map[int64]string),
		samples:       make(map[string][]runSample),
		lastNewMap:    make(map[string]time.Time),
		flags:         make(map[string]flagWatch),
		boards:        make(map[int64]int64),
		started:       time.Now(),
		book:          newAlertBook(12 * time.Hour),
	}
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

	b := newBot(cfg, &telegramClient{token: cfg.BotToken, http: &http.Client{Timeout: 65 * time.Second}},
		operatorapi.New(cfg.OperatorURL, cfg.ReplayURL, cfg.Alertmanager))
	if err := b.tg.setCommands(ctx, botCommands); err != nil {
		log.Printf("poketelegram: setMyCommands: %v", err)
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
		GitHubRepo:     envDefault("POKEPILOT_GITHUB_REPO", "maestroi/PokePilot"),
		OpsToken:       operatorapi.ReadSecretFile(os.Getenv("POKEPILOT_OPS_TOKEN_FILE")),
	}
	cfg.DigestHour, _ = strconv.Atoi(envDefault("POKEPILOT_WATCH_DIGEST_HOUR", "9"))
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
	chat := msg.Chat.ID
	text := msg.Text
	if r := msg.ReplyToMessage; r != nil {
		b.mu.Lock()
		token := b.confirmMsgs[chat][r.MessageID]
		pending, pendingOK := b.confirmations[token]
		b.mu.Unlock()
		if pendingOK && pending.Action == "flag" && strings.TrimSpace(text) != "" && !strings.HasPrefix(strings.TrimSpace(text), "/") {
			b.m.commands.Add(1)
			b.reply(ctx, chat, b.confirm(ctx, msg.From.ID, chat, token, strings.TrimSpace(text)), nil)
			return
		}
		if runID := b.runForMessage(chat, r.MessageID); runID != "" {
			switch word := strings.ToLower(strings.TrimSpace(text)); word {
			case "flag", "stop", "restart", "frame", "run", "replay", "triage":
				text = "/" + word + " " + runID
			}
		}
	}
	cmd, arg := parseCommand(text)
	if cmd == "" {
		return
	}
	b.m.commands.Add(1)

	var out string
	var keyboard *inlineKeyboard
	var c *card
	var err error
	switch cmd {
	case "start", "menu":
		hc := b.homeCard()
		c = &hc
	case "help":
		out = helpText()
	case "status":
		out, err = b.statusText(ctx)
	case "runs":
		var rc card
		if rc, err = b.runsCard(ctx, chat, 0); err == nil {
			c = &rc
		}
	case "run", "flag", "stop", "restart", "triage", "replay", "frame":
		if arg == "" {
			out = "Usage: /" + cmd + " <run-id or list number>"
			break
		}
		note := ""
		if cmd == "flag" {
			arg, note, _ = strings.Cut(arg, " ")
			note = strings.TrimSpace(note)
		}
		arg = b.resolveRun(chat, arg)
		switch cmd {
		case "run":
			out, keyboard, err = b.runText(ctx, arg)
		case "frame":
			b.sendRunFrame(ctx, chat, arg)
		case "triage":
			out, err = b.runTriage(ctx, msg.From.ID, chat, arg)
		case "replay":
			out, err = b.queueReplay(ctx, msg.From.ID, chat, arg)
		default:
			out, keyboard, err = b.askConfirmation(ctx, msg.From.ID, chat, cmd, arg)
			if err == nil && note != "" {
				b.setConfirmNote(keyboard, note)
			}
		}
	case "failures":
		out, err = b.failuresText(ctx)
	case "alerts":
		out, err = b.alertsText(ctx)
	case "health":
		hc := b.healthCard()
		c = &hc
	case "fixer":
		fc := b.fixerCard(ctx)
		c = &fc
	case "board":
		b.startBoard(ctx, chat)
		return
	default:
		out = "Unknown command. Use /help."
	}
	if err != nil {
		b.m.operatorErrs.Add(1)
		c, out, keyboard = nil, "Operator request failed: "+clip(err.Error(), 500), nil
	}
	switch {
	case c != nil:
		b.sendCard(ctx, chat, *c)
	case out != "":
		id := b.reply(ctx, chat, out, keyboard)
		b.rememberConfirmMessage(chat, id, keyboard)
		if cmd == "run" && err == nil {
			b.rememberRunMessage(chat, id, arg)
		}
	}
	if cmd == "run" && err == nil && arg != "" {
		b.sendRunFrame(ctx, chat, arg)
	}
}

// reply sends plain text and returns the Telegram message id (0 on failure).
func (b *bot) reply(ctx context.Context, chat int64, text string, keyboard *inlineKeyboard) int64 {
	id, err := b.tg.send(ctx, chat, outgoing{Text: text, Keyboard: keyboard})
	if err != nil {
		b.m.telegramErrs.Add(1)
		log.Printf("poketelegram: sendMessage: %v", err)
	}
	return id
}

var errUnknownView = errors.New("unknown view")

// render builds the card for a nav:/ref:/pg: callback.
func (b *bot) render(ctx context.Context, chat int64, kind string, page int) (card, error) {
	switch kind {
	case "home":
		return b.homeCard(), nil
	case "runs":
		return b.runsCard(ctx, chat, page)
	case "failures", "fail":
		return b.failuresCard(ctx, page)
	case "health":
		return b.healthCard(), nil
	case "fixer":
		return b.fixerCard(ctx), nil
	case "alerts":
		return b.alertsCard(ctx)
	case "status":
		text, err := b.statusText(ctx)
		return card{Text: h(text), Keyboard: kb(backRow("nav:status"))}, err
	}
	return card{}, fmt.Errorf("%w %q", errUnknownView, kind)
}

func (b *bot) handleCallback(ctx context.Context, cb callbackQuery) {
	_ = b.tg.answerCallback(ctx, cb.ID)
	chat, msgID := cb.Message.Chat.ID, cb.Message.MessageID
	if !b.authorized(cb.From.ID, chat) {
		b.m.unauthorized.Add(1)
		return
	}
	parts := strings.Split(cb.Data, ":")
	expired := card{Text: "That button expired. Tap ↻ Refresh.", Keyboard: kb(backRow("nav:home"))}
	switch parts[0] {
	case "cancel":
		if len(parts) != 2 {
			return
		}
		b.mu.Lock()
		delete(b.confirmations, parts[1])
		b.mu.Unlock()
		b.reply(ctx, chat, "Action cancelled.", nil)
	case "confirm":
		if len(parts) != 2 {
			return
		}
		b.reply(ctx, chat, b.confirm(ctx, cb.From.ID, chat, parts[1], ""), nil)
	case "nav", "pg", "ref":
		kind, page := "", 0
		switch {
		case parts[0] == "pg" && len(parts) == 3:
			kind = parts[1]
			page, _ = strconv.Atoi(parts[2])
		case parts[0] == "ref" && len(parts) == 3 && parts[1] == "run":
			b.editRun(ctx, chat, msgID, b.unhandle(parts[2]), expired)
			return
		case len(parts) >= 2:
			kind = parts[1]
		default:
			return
		}
		c, err := b.render(ctx, chat, kind, page)
		if errors.Is(err, errUnknownView) {
			b.editCard(ctx, chat, msgID, expired)
			return
		}
		if err != nil {
			b.m.operatorErrs.Add(1)
			c = card{Text: "Operator request failed: " + h(clip(err.Error(), 300)), Keyboard: kb(backRow("nav:" + kind))}
		}
		b.editCard(ctx, chat, msgID, c)
	case "run":
		if len(parts) != 2 {
			return
		}
		b.editRun(ctx, chat, msgID, b.unhandle(parts[1]), expired)
	case "act":
		if len(parts) != 3 {
			return
		}
		runID := b.unhandle(parts[2])
		if runID == "" {
			b.editCard(ctx, chat, msgID, expired)
			return
		}
		b.runAction(ctx, cb.From.ID, chat, parts[1], runID)
	case "mute":
		if len(parts) != 2 {
			return
		}
		name := b.unhandle(parts[1])
		until := time.Now().Add(12 * time.Hour)
		b.alertMu.Lock()
		ok := name != "" && b.book.Mute(name, until)
		b.alertMu.Unlock()
		if !ok {
			b.editCard(ctx, chat, msgID, expired)
			return
		}
		b.editCard(ctx, chat, msgID, card{Text: h(cb.Message.Text) + "\n🔕 muted until " + until.UTC().Format("15:04") + " UTC", Keyboard: cb.Message.ReplyMarkup})
	}
}

func (b *bot) editRun(ctx context.Context, chat, msgID int64, runID string, expired card) {
	if runID == "" {
		b.editCard(ctx, chat, msgID, expired)
		return
	}
	c, err := b.runCard(ctx, runID)
	if err != nil {
		b.m.operatorErrs.Add(1)
		c = card{Text: "Operator request failed: " + h(clip(err.Error(), 300)), Keyboard: kb(backRow("nav:runs"))}
	}
	b.editCard(ctx, chat, msgID, c)
}

func (b *bot) runAction(ctx context.Context, actor, chat int64, action, runID string) {
	var out string
	var keyboard *inlineKeyboard
	var err error
	switch action {
	case "frame":
		b.sendRunFrame(ctx, chat, runID)
		return
	case "replay":
		out, err = b.queueReplay(ctx, actor, chat, runID)
	case "triage":
		out, err = b.runTriage(ctx, actor, chat, runID)
	case "flag", "stop", "restart":
		out, keyboard, err = b.askConfirmation(ctx, actor, chat, action, runID)
	default:
		return
	}
	if err != nil {
		b.m.operatorErrs.Add(1)
		out, keyboard = "Operator request failed: "+clip(err.Error(), 500), nil
	}
	id := b.reply(ctx, chat, out, keyboard)
	b.rememberConfirmMessage(chat, id, keyboard)
}

// confirm completes a pending confirmation (button tap or flag note reply)
// and returns the text to post.
func (b *bot) confirm(ctx context.Context, actor, chat int64, token, note string) string {
	b.mu.Lock()
	pending, ok := b.confirmations[token]
	expired := ok && pending.Expires.Before(time.Now())
	valid := ok && !expired && pending.ActorID == actor && pending.ChatID == chat
	if valid || expired {
		delete(b.confirmations, token)
	}
	b.mu.Unlock()
	if !valid {
		return "That confirmation expired or belongs to another operator."
	}
	if !b.controlAllowed(actor) {
		return "Control action rate-limited; try again in a moment."
	}
	if note != "" {
		pending.Note = note
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
	case "flag":
		err = b.op.FlagStuck(ctx, pending.Target, pending.Note)
		if err == nil {
			text = "🚩 Flagged " + pending.Target + " as stuck. I'll post the issue link when triage files it."
			b.watchFlag(pending.Target, chat)
		}
	default:
		err = errors.New("unknown confirmation action")
	}
	if err != nil {
		b.m.actionErrs.Add(1)
		text = "Action failed: " + clip(err.Error(), 500)
		b.audit(actor, chat, pending.Action, pending.Target, "error", err)
	} else {
		b.audit(actor, chat, pending.Action, pending.Target, "ok", nil)
	}
	return text
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
	if action == "flag" {
		text = fmt.Sprintf("🚩 Flag run %s as stuck?\n%s · %s · frame %d\nTo add a note, reply to this message with it. Otherwise tap Confirm.", runID, emptyDash(inspection.Run.Game), emptyDash(inspection.Run.Status), inspection.Run.Frame)
	}
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
	// Alertmanager leaves resolved alerts out of the default query, so a
	// recent recovery is invisible unless it is requested explicitly. A
	// failure here must not hide the active alerts, so it degrades to "none".
	resolved, resolvedErr := b.op.ResolvedAlerts(ctx)
	if resolvedErr != nil {
		resolved = nil
	}

	lines := make([]string, 0, 6)
	if len(active) == 0 {
		lines = append(lines, "No active Alertmanager alerts.")
	} else {
		lines = append(lines, fmt.Sprintf("Active alerts (%d):", len(active)))
		for i, alert := range active {
			if i >= 15 {
				lines = append(lines, "…more alerts omitted")
				break
			}
			lines = append(lines, "• "+formatAlert(alert))
		}
	}
	if len(resolved) > 0 {
		lines = append(lines, "", fmt.Sprintf("Recently resolved (%d):", len(resolved)))
		for i, alert := range resolved {
			if i >= 5 {
				lines = append(lines, "…more resolved alerts omitted")
				break
			}
			lines = append(lines, "• "+formatAlert(alert))
		}
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
	now := time.Now()
	dash, err := b.op.Dashboard(ctx, false, 200)
	if err != nil {
		b.m.operatorErrs.Add(1)
		b.m.wallHealthy.Store(0)
	} else {
		b.m.wallHealthy.Store(1)
		b.m.lastPollUnix.Store(now.Unix())
		b.mu.Lock()
		b.lastDash = dash.Runs
		b.mu.Unlock()
		b.observeRuns(ctx, dash.Runs)
		b.observeRenderJobs(ctx)
		b.observeFlags(ctx, now)
	}
	b.applyAlerts(ctx, "bot", b.localChecks(dash.Runs, err, now), now)
	b.observeAlerts(ctx)
	if now.Sub(b.boardAt) >= time.Minute {
		b.boardAt = now
		b.refreshBoards(ctx)
	}
	b.maybeDigest(ctx, now)
}

func (b *bot) observeRuns(ctx context.Context, runs []operatorapi.Run) {
	now := time.Now()
	current := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		current[run.RunID] = struct{}{}
		if isActive(run.Status) {
			b.recordSample(run, now)
		}
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
		}
		if active && prev.LastProgress.IsZero() {
			prev.LastProgress = now
		}
		if prev.Status != run.Status && run.Status == "done" {
			kind, icon, extra := "finished", "✅", run.Reason+detailSuffix(run.Detail)
			if !successfulReason(run.Reason) {
				kind, icon = "failed", "🔴"
			} else if stats := b.finishedGoalStats(ctx, run.RunID, run.Stats); completedProgressGoal(stats) {
				// The run finished the progression track it was given, so the
				// agent will not advance further until more content is
				// supported. The experimental Gen-II preset points a progress
				// goal at its supported frontier, which is exactly this case.
				kind, icon, extra = "progression goal complete", "🏁", frontierDetail(stats)
			}
			b.notifyRun(ctx, icon, kind, run, extra)
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
	b.mu.Lock()
	for id := range b.samples {
		if _, ok := current[id]; !ok {
			delete(b.samples, id)
			delete(b.lastNewMap, id)
		}
	}
	b.mu.Unlock()
}

// completedProgressGoal reports whether a finished run completed the
// progression goal it was given. Progress goals are the generic
// "progress:<id>" form; the experimental Gen-II preset points one at its
// supported frontier, so completion means the agent has reached the end of
// that progression track and will not advance until more content is
// supported. This reads the structured goal identity and never the goal's
// prose summary.
func completedProgressGoal(stats *operatorapi.RunStats) bool {
	return stats != nil &&
		stats.GoalComplete &&
		strings.EqualFold(strings.TrimSpace(stats.GoalKind), "progress")
}

// finishedGoalStats returns a completed run's goal statistics. The dashboard
// list does not always carry stats, so fall back to the single-run endpoint:
// one extra call on a completion transition, never once per poll.
func (b *bot) finishedGoalStats(ctx context.Context, runID string, stats *operatorapi.RunStats) *operatorapi.RunStats {
	if stats != nil {
		return stats
	}
	detail, err := b.op.Run(ctx, runID)
	if err != nil {
		b.m.operatorErrs.Add(1)
		return nil
	}
	return detail.Run.Stats
}

func frontierDetail(stats *operatorapi.RunStats) string {
	id := ""
	if stats != nil {
		id = strings.TrimSpace(stats.GoalID)
	}
	if id == "" {
		return "The agent completed a declared progression goal and will not advance past it until more content is supported."
	}
	return fmt.Sprintf("Progression goal %s reached. The agent will not advance past this frontier until more content is supported.", id)
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
	mux.HandleFunc("POST /v1/ops", b.handleOps)
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
		"/menu — tap-driven menu",
		"/runs — active runs (numbered; use the number as N below)",
		"/run N (or id) — run state, progress, party and latest frame",
		"/flag N [note] — flag a run as stuck so triage files an issue",
		"/stop N — confirmed cooperative stop",
		"/restart N — confirmed restart from latest replayable checkpoint, else fresh clone",
		"/frame N — latest frame",
		"/triage N — queue the existing investigation for a run",
		"/replay N — queue a replay render",
		"/health — swarm, disk and deploy health",
		"/fixer — fixer budget, blocked keys and PRs",
		"/failures — active failure/triage groups",
		"/alerts — open alerts",
		"/board — post a live board to pin",
		"/status — farm, queue, worker and replay summary",
		"Shortcuts: reply flag, stop, restart, frame, run, replay or triage to any bot message about a run. Reply with text to a flag confirmation to add a note.",
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
