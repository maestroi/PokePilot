package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

const pageSize = 8

type card struct {
	Text     string // HTML
	Keyboard *inlineKeyboard
	RunID    string // set when the card is about one run (reply shortcuts)
}

type runSample struct {
	At    time.Time
	Frame uint64
	Maps  int
}

type flagWatch struct {
	Chat  int64
	Since time.Time
}

var botCommands = [][2]string{
	{"menu", "Open the menu"}, {"runs", "Active runs"}, {"health", "Swarm, disk and deploy health"},
	{"fixer", "Fixer budget, blocked keys and PRs"}, {"failures", "Failure groups"}, {"alerts", "Open alerts"},
	{"board", "Post a live board to pin"}, {"status", "One-line farm summary"}, {"help", "All commands"},
}

func btn(text, data string) inlineButton { return inlineButton{Text: text, CallbackData: data} }
func link(text, url string) inlineButton { return inlineButton{Text: text, URL: url} }

func backRow(refresh string) []inlineButton {
	return []inlineButton{btn("↻ Refresh", refresh), btn("« Menu", "nav:home")}
}

func kb(rows ...[]inlineButton) *inlineKeyboard { return &inlineKeyboard{InlineKeyboard: rows} }

// handle maps an arbitrary value (run ID, check name) to a short stable
// token for callback data. Bounded: the map resets past 5000 entries; a stale
// button then answers "expired, refresh".
func (b *bot) handle(value string) string {
	sum := sha1.Sum([]byte(value))
	hd := hex.EncodeToString(sum[:5])
	b.mu.Lock()
	if len(b.handles) > 5000 {
		b.handles = map[string]string{}
	}
	b.handles[hd] = value
	b.mu.Unlock()
	return hd
}

func (b *bot) unhandle(hd string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.handles[hd]
}

func (b *bot) resolveRun(chat int64, arg string) string {
	if n, err := strconv.Atoi(strings.TrimSpace(arg)); err == nil && n >= 1 {
		b.mu.Lock()
		list := b.lastList[chat]
		b.mu.Unlock()
		if n <= len(list) {
			return list[n-1]
		}
	}
	return strings.TrimSpace(arg)
}

func (b *bot) rememberRunMessage(chat, msgID int64, runID string) {
	if runID == "" || msgID == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.msgRuns[chat]
	if m == nil || len(m) > 500 {
		m = map[int64]string{}
		b.msgRuns[chat] = m
	}
	m[msgID] = runID
}

func (b *bot) runForMessage(chat, msgID int64) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.msgRuns[chat][msgID]
}

// rememberConfirmMessage records which confirmation token a sent confirmation
// message carries so a reply to it can complete it.
func (b *bot) rememberConfirmMessage(chat, msgID int64, k *inlineKeyboard) {
	if k == nil || msgID == 0 {
		return
	}
	for _, row := range k.InlineKeyboard {
		for _, bt := range row {
			if token, ok := strings.CutPrefix(bt.CallbackData, "confirm:"); ok {
				b.mu.Lock()
				m := b.confirmMsgs[chat]
				if m == nil || len(m) > 500 {
					m = map[int64]string{}
					b.confirmMsgs[chat] = m
				}
				m[msgID] = token
				b.mu.Unlock()
				return
			}
		}
	}
}

func (b *bot) watchFlag(runID string, chat int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flags[runID] = flagWatch{Chat: chat, Since: time.Now()}
}

func (b *bot) opsSnapshot() *operatorapi.OpsSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ops
}

func (b *bot) sendCard(ctx context.Context, chat int64, c card) {
	id, err := b.tg.send(ctx, chat, outgoing{Text: c.Text, HTML: true, Keyboard: c.Keyboard})
	if err != nil {
		b.m.telegramErrs.Add(1)
		log.Printf("poketelegram: send card: %v", err)
		return
	}
	b.rememberRunMessage(chat, id, c.RunID)
}

func (b *bot) editCard(ctx context.Context, chat, msgID int64, c card) {
	err := b.tg.edit(ctx, chat, msgID, outgoing{Text: c.Text, HTML: true, Keyboard: c.Keyboard})
	switch {
	case err == nil, errors.Is(err, errNotModified):
		b.rememberRunMessage(chat, msgID, c.RunID)
	case errors.Is(err, errMessageGone):
		b.sendCard(ctx, chat, c)
	default:
		b.m.telegramErrs.Add(1)
		log.Printf("poketelegram: edit card: %v", err)
	}
}

func badgeBar(n, total int) string {
	if total <= 0 {
		total = 8
	}
	if n > total {
		total = n
	}
	return fmt.Sprintf("🏅 %s%s %d/%d", strings.Repeat("●", n), strings.Repeat("○", total-n), n, total)
}

// placeName is game-agnostic: the runtime's generic map_name when the game
// adapter publishes one, otherwise the numeric map id.
func placeName(run operatorapi.Run) string {
	if name, ok := run.GameState["map_name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fmt.Sprintf("map %d", run.Map)
}

func badges(run operatorapi.Run) int {
	n := run.RecoveryBadges
	if run.Player != nil && len(run.Player.Badges) > n {
		n = len(run.Player.Badges)
	}
	return n
}

func statusDot(status string) string {
	switch status {
	case "running", "leased":
		return "🟢"
	case "queued":
		return "⏳"
	}
	return "⚪"
}

func (b *bot) homeCard() card {
	return card{Text: "<b>PokePilot</b>\nPick a view.", Keyboard: kb(
		[]inlineButton{btn("📊 Status", "nav:status"), btn("🎮 Runs", "nav:runs")},
		[]inlineButton{btn("🧯 Failures", "nav:failures"), btn("🩺 Health", "nav:health")},
		[]inlineButton{btn("🔧 Fixer", "nav:fixer"), btn("🚨 Alerts", "nav:alerts")},
	)}
}

func pager(prefix string, page, pages int) []inlineButton {
	var nav []inlineButton
	if page > 0 {
		nav = append(nav, btn("‹", fmt.Sprintf("pg:%s:%d", prefix, page-1)))
	}
	nav = append(nav, btn(fmt.Sprintf("%d/%d", page+1, pages), fmt.Sprintf("pg:%s:%d", prefix, page)))
	if page < pages-1 {
		nav = append(nav, btn("›", fmt.Sprintf("pg:%s:%d", prefix, page+1)))
	}
	return nav
}

func (b *bot) runsCard(ctx context.Context, chat int64, page int) (card, error) {
	dash, err := b.op.Dashboard(ctx, true, 50)
	if err != nil {
		return card{}, err
	}
	runs := dash.Runs
	ids := make([]string, len(runs))
	for i, r := range runs {
		ids[i] = r.RunID
	}
	b.mu.Lock()
	b.lastList[chat] = ids
	b.mu.Unlock()
	if len(runs) == 0 {
		return card{Text: "<b>Runs</b>\nNo active runs.", Keyboard: kb(backRow("nav:runs"))}, nil
	}
	pages := (len(runs) + pageSize - 1) / pageSize
	page = max(0, min(page, pages-1))
	lines := []string{fmt.Sprintf("<b>Runs</b> (%d)", len(runs))}
	var rows [][]inlineButton
	for i := page * pageSize; i < min(len(runs), (page+1)*pageSize); i++ {
		r := runs[i]
		label := fmt.Sprintf("%s %s · 🏅%d · %s", statusDot(r.Status), emptyDash(r.Game), badges(r), placeName(r))
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, h(label)))
		rows = append(rows, []inlineButton{btn(clip(fmt.Sprintf("%d. %s", i+1, label), 60), "run:"+b.handle(r.RunID))})
	}
	if pages > 1 {
		rows = append(rows, pager("runs", page, pages))
	}
	rows = append(rows, backRow(fmt.Sprintf("pg:runs:%d", page)))
	return card{Text: strings.Join(lines, "\n"), Keyboard: kb(rows...)}, nil
}

func (b *bot) runCard(ctx context.Context, runID string) (card, error) {
	inspection, err := b.op.Run(ctx, runID)
	if err != nil {
		return card{}, err
	}
	run := inspection.Run
	lines := []string{
		fmt.Sprintf("%s <b>%s</b> · <code>%s</code>", statusDot(run.Status), h(emptyDash(run.Game)), h(run.RunID)),
		badgeBar(badges(run), 8),
		fmt.Sprintf("📍 %s · %d maps visited%s", h(placeName(run)), run.MapsVisited, b.lastNewMapSuffix(run.RunID)),
		fmt.Sprintf("🎞 frame %d%s", run.Frame, b.rateSuffix(run.RunID)),
		fmt.Sprintf("🔁 attempt %d · lost %d · recoveries %d", run.Attempts, run.LossRecoveries, run.RecoveryEvents),
		"🧠 " + plannerState(run),
		"🎯 " + h(clip(emptyDash(runGoal(run)), 200)),
	}
	if run.Reason != "" {
		lines = append(lines, "Result: "+h(run.Reason+detailSuffix(run.Detail)))
	}
	if group, gErr := b.op.FindTriageForRun(ctx, runID); gErr == nil && group != nil {
		issue := ""
		if group.Issue != nil && group.Issue.IssueNumber > 0 {
			issue = fmt.Sprintf(" · issue #%d", group.Issue.IssueNumber)
		}
		lines = append(lines, fmt.Sprintf("🧯 x%d %s%s", group.Count, h(clip(firstNonEmpty(group.Pattern, group.Example), 160)), issue))
	}
	hd := b.handle(runID)
	return card{Text: strings.Join(lines, "\n"), RunID: runID, Keyboard: kb(
		[]inlineButton{btn("🖼 Frame", "act:frame:"+hd), btn("🎬 Replay", "act:replay:"+hd), btn("🧯 Triage", "act:triage:"+hd)},
		[]inlineButton{btn("🚩 Flag stuck", "act:flag:"+hd), btn("⏹ Stop", "act:stop:"+hd), btn("🔄 Restart", "act:restart:"+hd)},
		[]inlineButton{link("Admin", b.adminRunURL(runID)), link("Spectator", b.spectatorRunURL(runID))},
		[]inlineButton{btn("↻ Refresh", "run:"+hd), btn("« Runs", "nav:runs")},
	)}, nil
}

func plannerState(run operatorapi.Run) string {
	switch {
	case !isActive(run.Status):
		return h(emptyDash(run.Status))
	case run.Question != "" && run.Decision == "":
		return "thinking"
	case run.Decision != "":
		return "acting: " + h(clip(run.Decision, 120))
	}
	return "idle"
}

// recordSample keeps 15 minutes of (frame, maps) per active run from the
// monitor tick, for frames/min and "last new map".
func (b *bot) recordSample(run operatorapi.Run, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.samples == nil {
		b.samples = map[string][]runSample{}
	}
	if b.lastNewMap == nil {
		b.lastNewMap = map[string]time.Time{}
	}
	s := append(b.samples[run.RunID], runSample{At: now, Frame: run.Frame, Maps: run.MapsVisited})
	for len(s) > 0 && now.Sub(s[0].At) > 15*time.Minute {
		s = s[1:]
	}
	b.samples[run.RunID] = s
	if len(s) >= 2 && s[len(s)-1].Maps > s[len(s)-2].Maps {
		b.lastNewMap[run.RunID] = now
	}
}

func (b *bot) rateSuffix(runID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.samples[runID]
	if len(s) < 2 || s[len(s)-1].Frame < s[0].Frame {
		return ""
	}
	span := s[len(s)-1].At.Sub(s[0].At).Minutes()
	if span <= 0 {
		return ""
	}
	return fmt.Sprintf(" · %.0f/min", float64(s[len(s)-1].Frame-s[0].Frame)/span)
}

func (b *bot) lastNewMapSuffix(runID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if at, ok := b.lastNewMap[runID]; ok {
		return " · last new map " + durationShort(time.Since(at)) + " ago"
	}
	return ""
}

func (b *bot) failuresCard(ctx context.Context, page int) (card, error) {
	groups, err := b.op.Triage(ctx)
	if err != nil {
		return card{}, err
	}
	if len(groups) == 0 {
		return card{Text: "<b>Failures</b>\nNo active failure groups.", Keyboard: kb(backRow("nav:failures"))}, nil
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
	pages := (len(groups) + pageSize - 1) / pageSize
	page = max(0, min(page, pages-1))
	lines := []string{fmt.Sprintf("<b>Failure groups</b> (%d)", len(groups))}
	for _, g := range groups[page*pageSize : min(len(groups), (page+1)*pageSize)] {
		run := ""
		if len(g.RunIDs) > 0 {
			run = " · <code>" + h(g.RunIDs[0]) + "</code>"
		}
		issue := ""
		if g.Issue != nil && g.Issue.IssueNumber > 0 {
			issue = fmt.Sprintf(" · issue #%d %s", g.Issue.IssueNumber, h(g.Issue.Status))
		}
		lines = append(lines, fmt.Sprintf("• x%d %s%s%s", g.Count, h(clip(firstNonEmpty(g.Pattern, g.Example), 160)), run, issue))
	}
	var rows [][]inlineButton
	if pages > 1 {
		rows = append(rows, pager("fail", page, pages))
	}
	rows = append(rows, backRow(fmt.Sprintf("pg:fail:%d", page)))
	return card{Text: strings.Join(lines, "\n"), Keyboard: kb(rows...)}, nil
}

func (b *bot) alertsCard(ctx context.Context) (card, error) {
	b.alertMu.Lock()
	open := b.book.Open()
	type row struct{ name, msg string }
	snap := make([]row, len(open))
	for i, st := range open {
		snap[i] = row{st.Name, st.Message}
	}
	b.alertMu.Unlock()

	lines := []string{"<b>Alerts</b>"}
	var rows [][]inlineButton
	if len(snap) == 0 {
		lines = append(lines, "No open watcher alerts.")
	}
	for i, a := range snap {
		lines = append(lines, "🔴 <b>"+h(a.name)+"</b>: "+h(clip(a.msg, 200)))
		if i < 8 {
			rows = append(rows, []inlineButton{btn("🔕 Mute 12h · "+clip(a.name, 30), "mute:"+b.handle(a.name))})
		}
	}
	if am, err := b.alertsText(ctx); err == nil {
		lines = append(lines, "", h(am))
	} else {
		lines = append(lines, "", "Alertmanager unavailable: "+h(clip(err.Error(), 120)))
	}
	rows = append(rows, backRow("nav:alerts"))
	return card{Text: strings.Join(lines, "\n"), Keyboard: kb(rows...)}, nil
}

func (b *bot) healthCard() card {
	s := b.opsSnapshot()
	rows := [][]inlineButton{backRow("nav:health")}
	if s == nil {
		return card{Text: "<b>Health</b>\nNo watcher data yet.", Keyboard: kb(rows...)}
	}
	lines := []string{fmt.Sprintf("<b>Health</b> · %s ago", durationShort(time.Since(time.Unix(s.At, 0))))}
	managers, up := 0, 0
	var nodeLines []string
	for _, n := range s.Nodes {
		icon := "🟢"
		if n.Status != "ready" || n.ManagerStatus == "unreachable" {
			icon = "🔴"
		}
		role := ""
		if n.Manager {
			managers++
			if icon == "🟢" {
				up++
			}
			role = " (" + n.ManagerStatus + ")"
		}
		nodeLines = append(nodeLines, fmt.Sprintf("%s %s%s", icon, h(n.Hostname), h(role)))
	}
	lines = append(lines, fmt.Sprintf("<b>Nodes</b> · managers %d/%d", up, managers))
	lines = append(lines, nodeLines...)
	lines = append(lines, "<b>Services</b>")
	for _, svc := range s.Services {
		icon := "🟢"
		if svc.Running < svc.Desired || strings.HasPrefix(svc.UpdateState, "rollback") {
			icon = "🔴"
		}
		lines = append(lines, fmt.Sprintf("%s %s %d/%d%s", icon, h(svc.Name), svc.Running, svc.Desired, h(suffixIf(svc.UpdateState))))
	}
	if s.FrozenUntil > time.Now().Unix() {
		lines = append(lines, "🧊 deploys frozen until "+time.Unix(s.FrozenUntil, 0).UTC().Format("Jan 2 15:04 UTC"))
	}
	type disk struct {
		node, mount string
		free        int
	}
	var disks []disk
	for _, r := range s.Disks {
		for _, d := range r.Disks {
			disks = append(disks, disk{r.Node, d.Mount, d.FreeGB})
		}
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].free < disks[j].free })
	lines = append(lines, "<b>Disk free</b> (lowest first)")
	for i, d := range disks {
		if i == 6 {
			break
		}
		lines = append(lines, fmt.Sprintf("%dG %s %s", d.free, h(d.node), h(d.mount)))
	}
	return card{Text: strings.Join(lines, "\n"), Keyboard: kb(rows...)}
}

func suffixIf(s string) string {
	if s == "" || s == "completed" {
		return ""
	}
	return " · " + s
}

func (b *bot) fixerCard(ctx context.Context) card {
	s := b.opsSnapshot()
	rows := [][]inlineButton{backRow("nav:fixer")}
	if s == nil || s.Fixer == nil {
		return card{Text: "<b>Fixer</b>\nNo fixer report yet.", Keyboard: kb(rows...)}
	}
	f := s.Fixer
	rate := "—"
	if f.Attempts24h > 0 {
		rate = fmt.Sprintf("%d%%", 100*f.PRs24h/f.Attempts24h)
	}
	lines := []string{
		"<b>Fixer</b> (24h)",
		fmt.Sprintf("💸 paid %d/%d · 🆓 qwen %d", f.PaidStarts24h, f.PaidCap, f.FreeStarts24h),
		fmt.Sprintf("🛠 %d attempts → %d PRs (%s)", f.Attempts24h, f.PRs24h, rate),
		fmt.Sprintf("✅ merged PRs: %s", countText(s.Merged24h)),
	}
	if len(f.BlockedKeys) > 0 {
		issues := map[string]int64{}
		if groups, err := b.op.Triage(ctx); err == nil {
			for _, g := range groups {
				if g.Issue != nil && g.Issue.IssueNumber > 0 {
					issues[g.Key] = g.Issue.IssueNumber
				}
			}
		}
		lines = append(lines, "<b>Blocked keys</b>")
		for i, k := range f.BlockedKeys {
			if i == 8 {
				lines = append(lines, fmt.Sprintf("…%d more", len(f.BlockedKeys)-8))
				break
			}
			item := "<code>" + h(k) + "</code>"
			if n := issues[k]; n > 0 {
				item = fmt.Sprintf(`<a href="https://github.com/%s/issues/%d">%s</a>`, h(b.cfg.GitHubRepo), n, item)
			}
			lines = append(lines, "• "+item)
		}
	}
	if len(s.TriagePRs) > 0 {
		lines = append(lines, "<b>Open fixer PRs</b>")
		for i, pr := range s.TriagePRs {
			if i == 8 {
				break
			}
			lines = append(lines, fmt.Sprintf(`• <a href="%s">#%d</a> %s old`, h(pr.URL), pr.Number, durationShort(time.Since(time.Unix(pr.CreatedAt, 0)))))
		}
	}
	return card{Text: strings.Join(lines, "\n"), Keyboard: kb(rows...)}
}

func countText(n int) string {
	if n < 0 {
		return "?"
	}
	return strconv.Itoa(n)
}

// setConfirmNote attaches a note to the pending confirmation behind kb.
func (b *bot) setConfirmNote(k *inlineKeyboard, note string) {
	if k == nil {
		return
	}
	for _, row := range k.InlineKeyboard {
		for _, bt := range row {
			if token, ok := strings.CutPrefix(bt.CallbackData, "confirm:"); ok {
				b.mu.Lock()
				if p, ok := b.confirmations[token]; ok {
					p.Note = note
					b.confirmations[token] = p
				}
				b.mu.Unlock()
				return
			}
		}
	}
}
