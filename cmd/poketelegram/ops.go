package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

const (
	watcherSilentAfter = 5 * time.Minute
	// restartWindow bounds how long adopted alerts are collected into the
	// single "Bot restarted" summary when the watcher does not push first.
	restartWindow = 2 * time.Minute
	// maxListLines caps alert lists so a message stays under Telegram's
	// 4096-character limit.
	maxListLines = 15
)

func (b *bot) handleOps(w http.ResponseWriter, r *http.Request) {
	if !operatorapi.OpsAuthorized(r, b.cfg.OpsToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var snap operatorapi.OpsSnapshot
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&snap); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	b.ops = &snap // replace, never mutate: cards read the old pointer unlocked
	b.opsAt = time.Now()
	b.mu.Unlock()
	// A cold watcher (just restarted) may not know every check yet; absent
	// ones are unknown, not resolved.
	b.applyAlertList(context.WithoutCancel(r.Context()), "watch", snap.Checks, time.Now(), snap.Warm)
}

// localChecks are the checks the bot computes itself each monitor tick.
func (b *bot) localChecks(runs []operatorapi.Run, wallErr error, now time.Time) []operatorapi.CheckResult {
	var out []operatorapi.CheckResult
	if wallErr != nil {
		out = append(out, operatorapi.CheckResult{Name: "wall", Grace: 2, Message: "wall/operator API unreachable: " + clip(wallErr.Error(), 200)})
	} else {
		out = append(out, operatorapi.CheckResult{Name: "wall", OK: true, Grace: 2})
		for _, run := range runs {
			if !isActive(run.Status) {
				continue
			}
			b.mu.Lock()
			w, seen := b.runs[run.RunID]
			b.mu.Unlock()
			stalled := seen && !w.LastProgress.IsZero() && now.Sub(w.LastProgress) >= b.cfg.StallAfter
			c := operatorapi.CheckResult{Name: "stall:" + run.RunID, Grace: 1, RunID: run.RunID, OK: !stalled}
			if stalled {
				c.Message = fmt.Sprintf("%s (%s) no frame progress for %s", run.RunID, emptyDash(run.Game), durationShort(now.Sub(w.LastProgress)))
			}
			out = append(out, c)
		}
		out = append(out, b.plannerChecks(runs)...)
	}
	b.mu.Lock()
	opsAt := b.opsAt
	b.mu.Unlock()
	silent := (opsAt.IsZero() && now.Sub(b.started) >= watcherSilentAfter) || (!opsAt.IsZero() && now.Sub(opsAt) >= watcherSilentAfter)
	c := operatorapi.CheckResult{Name: "watcher", Grace: 1, OK: !silent}
	if silent {
		c.Message = "no report from pokewatch for 5m+: Swarm, disk and fixer checks are blind"
	}
	return append(out, c)
}

// plannerChecks probes each live run's planner endpoint /health, as the
// retired farm-watch.sh did. Probes run at most once a minute (monitor
// goroutine only); in between the last results are reused so the "runs"
// source stays complete.
func (b *bot) plannerChecks(runs []operatorapi.Run) []operatorapi.CheckResult {
	if !b.plannerAt.IsZero() && time.Since(b.plannerAt) < time.Minute {
		return b.plannerLast
	}
	b.plannerAt = time.Now()
	b.plannerLast = b.probePlanners(runs)
	return b.plannerLast
}

func (b *bot) probePlanners(runs []operatorapi.Run) []operatorapi.CheckResult {
	seen := map[string]bool{}
	var out []operatorapi.CheckResult
	client := &http.Client{Timeout: 10 * time.Second}
	for _, run := range runs {
		if run.Stats == nil || !isActive(run.Status) {
			continue
		}
		ep := strings.TrimRight(run.Stats.Endpoint, "/")
		if !strings.HasPrefix(ep, "http") || seen[ep] {
			continue
		}
		seen[ep] = true
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ep+"/health", nil)
		res, err := client.Do(req)
		healthy := err == nil && res.StatusCode < 500
		if res != nil {
			res.Body.Close()
		}
		c := operatorapi.CheckResult{Name: "planner:" + ep, Grace: 2, OK: healthy}
		if !healthy {
			c.Message = "planner endpoint unreachable: " + ep + " (runs stall until it returns)"
		}
		out = append(out, c)
	}
	return out
}

// applyAlerts is called from the monitor loop and from the /v1/ops handler,
// so b.alertMu serializes all alertBook access and alert-card bookkeeping.
func (b *bot) applyAlerts(ctx context.Context, source string, results []operatorapi.CheckResult, now time.Time) {
	b.applyAlertList(ctx, source, results, now, true)
}

func (b *bot) applyAlertList(ctx context.Context, source string, results []operatorapi.CheckResult, now time.Time, resolveAbsent bool) {
	b.alertMu.Lock()
	defer b.alertMu.Unlock()
	observe := b.book.Observe
	if !resolveAbsent {
		observe = b.book.ObserveCold
	}
	actions, adopted := observe(source, results, now)
	// Before the restart summary went out, adopted alerts were never
	// announced, so their resolution is not either.
	announced := b.restartSent
	if b.restartSent {
		// A source first seen after the restart window pages normally.
		for _, st := range adopted {
			b.sendAlertCard(ctx, st, now)
		}
	} else {
		// One summary per restart: collect every source's adoptions until
		// the watcher pushes (after the first monitor pass, so a push racing
		// it cannot split the summary) or the window closes.
		b.restartAdopted = append(b.restartAdopted, adopted...)
		if (source == "watch" && b.book.seeded["bot"]) || now.Sub(b.started) >= restartWindow {
			b.restartSent = true
			b.sendRestartSummary(ctx)
			b.restartAdopted = nil
		}
	}
	for _, a := range actions {
		switch a.Kind {
		case alertOpen:
			a.State.Cards = map[int64]int64{} // a new episode never edits an old card
			b.sendAlertCard(ctx, a.State, now)
		case alertRemind:
			for chat, msg := range a.State.Cards {
				b.notifyOne(ctx, chat, outgoing{Text: "🔴 still failing (" + durationShort(now.Sub(a.State.Since)) + "): " + h(clip(a.State.Message, 300)), HTML: true, ReplyTo: msg})
			}
			if len(a.State.Cards) == 0 {
				b.sendAlertCard(ctx, a.State, now)
			}
		case alertResolve:
			text := fmt.Sprintf("✅ <b>%s</b> resolved after %s\n<s>%s</s>", h(a.State.Name), durationShort(now.Sub(a.State.Since)), h(clip(a.State.Message, 300)))
			if len(a.State.Cards) == 0 && announced {
				// Adopted (or never delivered): no card to edit, say it plainly.
				b.notifyHTML(ctx, outgoing{Text: text, HTML: true})
			}
			for chat, msg := range a.State.Cards {
				err := b.tg.edit(ctx, chat, msg, outgoing{Text: text, HTML: true})
				if errors.Is(err, errMessageGone) {
					b.notifyOne(ctx, chat, outgoing{Text: text, HTML: true})
					continue
				}
				if err != nil && !errors.Is(err, errNotModified) {
					b.m.telegramErrs.Add(1)
				}
				b.notifyOne(ctx, chat, outgoing{Text: "✅ resolved", ReplyTo: msg, Silent: true})
			}
		}
	}
}

// sendRestartSummary posts the alerts adopted since start that are still
// open, or nothing. Caller holds b.alertMu.
func (b *bot) sendRestartSummary(ctx context.Context) {
	var open []*alertState
	for _, st := range b.restartAdopted {
		if st.Open {
			open = append(open, st)
		}
	}
	if len(open) == 0 {
		return
	}
	lines := []string{fmt.Sprintf("🔁 <b>Bot restarted</b>: %d checks failing", len(open))}
	for i, st := range open {
		if i == maxListLines {
			lines = append(lines, fmt.Sprintf("…%d more", len(open)-maxListLines))
			break
		}
		lines = append(lines, "🔴 "+h(clip(firstNonEmpty(st.Message, st.Name), 200)))
	}
	b.notifyHTML(ctx, outgoing{Text: strings.Join(lines, "\n"), HTML: true, Keyboard: &inlineKeyboard{InlineKeyboard: [][]inlineButton{{btn("🚨 Alerts", "nav:alerts"), btn("🩺 Health", "nav:health")}}}})
}

func (b *bot) sendAlertCard(ctx context.Context, st *alertState, now time.Time) {
	hd := b.handle(st.Name)
	rows := [][]inlineButton{}
	if st.RunID != "" {
		rh := b.handle(st.RunID)
		rows = append(rows, []inlineButton{btn("🚩 Flag stuck", "act:flag:"+rh), btn("🎮 Open run", "run:"+rh), btn("🖼 Frame", "act:frame:"+rh)})
	}
	row := []inlineButton{btn("🔕 Mute 12h", "mute:"+hd), btn("ℹ️ Details", "nav:alerts")}
	if st.Link != "" {
		row = append(row, link("Open", st.Link))
	}
	rows = append(rows, row)
	text := fmt.Sprintf("🔴 <b>%s</b>\n%s\nfailing for %s", h(st.Name), h(clip(st.Message, 500)), durationShort(now.Sub(st.Since)))
	for _, chat := range b.cfg.NotifyChats {
		id, err := b.tg.send(ctx, chat, outgoing{Text: text, HTML: true, Keyboard: &inlineKeyboard{InlineKeyboard: rows}})
		if err != nil {
			b.m.telegramErrs.Add(1)
			log.Printf("poketelegram: alert %s to %d: %v", st.Name, chat, err)
			continue
		}
		b.m.notifications.Add(1)
		st.Cards[chat] = id // caller holds b.alertMu
		b.rememberRunMessage(chat, id, st.RunID)
	}
	if len(st.Cards) == 0 {
		st.Notified = time.Time{} // nothing delivered: the remind branch retries next tick
	}
}

func (b *bot) notifyOne(ctx context.Context, chat int64, m outgoing) {
	if _, err := b.tg.send(ctx, chat, m); err != nil {
		b.m.telegramErrs.Add(1)
		return
	}
	b.m.notifications.Add(1)
}

func (b *bot) notifyHTML(ctx context.Context, m outgoing) {
	for _, chat := range b.cfg.NotifyChats {
		b.notifyOne(ctx, chat, m)
	}
}

// observeFlags posts the issue link once triage files the flagged failure,
// or gives up after an hour with a link to the run.
func (b *bot) observeFlags(ctx context.Context, now time.Time) {
	b.mu.Lock()
	pending := make(map[string]flagWatch, len(b.flags))
	for k, v := range b.flags {
		pending[k] = v
	}
	b.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	groups, err := b.op.Triage(ctx) // once per tick for every pending flag
	for runID, f := range pending {
		var group *operatorapi.TriageGroup
		if err == nil {
			group = flaggedGroup(groups, runID)
		}
		done := false
		if group != nil && group.Issue != nil && group.Issue.IssueNumber > 0 {
			b.notifyOne(ctx, f.Chat, outgoing{HTML: true, Text: fmt.Sprintf(`🧯 Flagged run <code>%s</code> is filed as <a href="https://github.com/%s/issues/%d">issue #%d</a> (triage key <code>%s</code>).`,
				h(runID), h(b.cfg.GitHubRepo), group.Issue.IssueNumber, group.Issue.IssueNumber, h(group.Key))})
			done = true
		} else if now.Sub(f.Since) > time.Hour {
			b.notifyOne(ctx, f.Chat, outgoing{Text: "No issue was filed for flagged run " + runID + " within an hour. If the runner never stopped, the wall settled it without a dump; check " + b.adminRunURL(runID)})
			done = true
		}
		if done {
			b.mu.Lock()
			delete(b.flags, runID)
			b.mu.Unlock()
		}
	}
}

// flaggedGroup is the triage group filed for the operator flag on runID. The
// run may also belong to older groups (earlier attempts); only the one whose
// failure the wall rewrote as "operator flagged: <note>" is the flag's issue.
func flaggedGroup(groups []operatorapi.TriageGroup, runID string) *operatorapi.TriageGroup {
	for i := range groups {
		g := &groups[i]
		if !strings.Contains(g.Pattern, "operator flagged") && !strings.Contains(g.Example, "operator flagged") {
			continue
		}
		for _, id := range g.RunIDs {
			if id == runID {
				return g
			}
		}
	}
	return nil
}

func (b *bot) boardText() string {
	lines := []string{"📌 <b>PokePilot live</b> · " + time.Now().UTC().Format("15:04 UTC")}
	b.mu.Lock()
	runs := append([]operatorapi.Run(nil), b.lastDash...)
	snap := b.ops
	b.mu.Unlock()
	b.alertMu.Lock()
	var open []string // copied under alertMu: Observe mutates states
	for _, st := range b.book.Open() {
		open = append(open, firstNonEmpty(st.Message, st.Name))
	}
	b.alertMu.Unlock()
	active := 0
	for _, r := range runs {
		if !isActive(r.Status) {
			continue
		}
		active++
		if active <= 8 {
			lines = append(lines, fmt.Sprintf("%s %s 🏅%d %s%s", statusDot(r.Status), h(emptyDash(r.Game)), badges(r), h(placeName(r)), b.rateSuffix(r.RunID)))
		}
	}
	lines = append(lines, fmt.Sprintf("Runs: %d active", active))
	if len(open) == 0 {
		lines = append(lines, "✅ no open alerts")
	} else {
		lines = append(lines, fmt.Sprintf("🔴 %d open alerts", len(open)))
		for i, msg := range open {
			if i == 5 {
				break
			}
			lines = append(lines, "• "+h(clip(msg, 120)))
		}
	}
	if snap != nil && snap.Fixer != nil {
		lines = append(lines, fmt.Sprintf("🔧 paid %d/%d · qwen %d · PRs %d", snap.Fixer.PaidStarts24h, snap.Fixer.PaidCap, snap.Fixer.FreeStarts24h, snap.Fixer.PRs24h))
	}
	if snap != nil && snap.FrozenUntil > time.Now().Unix() {
		lines = append(lines, "🧊 deploys frozen")
	}
	return strings.Join(lines, "\n")
}

var boardKeyboard = &inlineKeyboard{InlineKeyboard: [][]inlineButton{{btn("🎮 Runs", "nav:runs"), btn("🩺 Health", "nav:health"), btn("🔧 Fixer", "nav:fixer")}}}

func (b *bot) startBoard(ctx context.Context, chat int64) {
	id, err := b.tg.send(ctx, chat, outgoing{Text: b.boardText(), HTML: true, Keyboard: boardKeyboard, Silent: true})
	if err != nil {
		b.m.telegramErrs.Add(1)
		return
	}
	b.mu.Lock()
	b.boards[chat] = id
	b.mu.Unlock()
	b.notifyOne(ctx, chat, outgoing{Text: "Pin the message above to keep it handy; it refreshes every minute without notifying.", ReplyTo: id, Silent: true})
}

func (b *bot) refreshBoards(ctx context.Context) {
	b.mu.Lock()
	boards := make(map[int64]int64, len(b.boards))
	for k, v := range b.boards {
		boards[k] = v
	}
	b.mu.Unlock()
	text := b.boardText()
	for chat, id := range boards {
		err := b.tg.edit(ctx, chat, id, outgoing{Text: text, HTML: true, Keyboard: boardKeyboard})
		if errors.Is(err, errMessageGone) {
			b.mu.Lock()
			delete(b.boards, chat)
			b.mu.Unlock()
		} else if err != nil && !errors.Is(err, errNotModified) {
			b.m.telegramErrs.Add(1)
		}
	}
}

func (b *bot) digestText(now time.Time) (string, *inlineKeyboard) {
	b.mu.Lock()
	snap := b.ops
	runs := append([]operatorapi.Run(nil), b.lastDash...)
	prev := b.digestBadges
	b.mu.Unlock()
	lines := []string{"📊 <b>PokePilot daily</b>"}
	if snap != nil {
		lines = append(lines, fmt.Sprintf("PRs merged: %s · farm issues opened %s, closed %s", countText(snap.Merged24h), countText(snap.FarmOpened), countText(snap.FarmClosed)))
		if f := snap.Fixer; f != nil {
			rate := "—"
			if f.Attempts24h > 0 {
				rate = fmt.Sprintf("%d%%", 100*f.PRs24h/f.Attempts24h)
			}
			lines = append(lines, fmt.Sprintf("Fixer: qwen %d, paid %d/%d · %d attempts → %d PRs (%s)", f.FreeStarts24h, f.PaidStarts24h, f.PaidCap, f.Attempts24h, f.PRs24h, rate))
		}
		if snap.FrozenUntil > now.Unix() {
			lines = append(lines, "🧊 deploys frozen")
		}
	} else {
		lines = append(lines, "No watcher data.")
	}
	cur := map[string]int{}
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })
	for _, r := range runs {
		if r.Status == "done" {
			continue
		}
		n := badges(r)
		cur[r.RunID] = n
		delta := ""
		if p, ok := prev[r.RunID]; ok {
			if n == p {
				delta = " (no change)"
			} else {
				delta = fmt.Sprintf(" (%+d)", n-p)
			}
		}
		lines = append(lines, fmt.Sprintf("• %s %s: 🏅%d%s · attempt %d, lost %d", h(emptyDash(r.Game)), h(r.PlayStyle), n, delta, r.Attempts, r.LossRecoveries))
	}
	var muted []string
	b.alertMu.Lock()
	for _, st := range b.book.Open() {
		if st.MutedUntil.After(now) {
			muted = append(muted, st.Name)
		}
	}
	b.alertMu.Unlock()
	b.mu.Lock()
	b.digestBadges = cur
	b.mu.Unlock()
	if len(muted) > 0 {
		lines = append(lines, "🔕 muted: "+h(strings.Join(muted, ", ")))
	}
	return strings.Join(lines, "\n"), boardKeyboard
}

func (b *bot) maybeDigest(ctx context.Context, now time.Time) {
	day := now.UTC().Format("2006-01-02")
	if now.UTC().Hour() < b.cfg.DigestHour || b.digestDay == day {
		return
	}
	b.digestDay = day
	text, kb := b.digestText(now)
	b.notifyHTML(ctx, outgoing{Text: text, HTML: true, Keyboard: kb})
}
