package deploy

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// LadderTier is one solver backend and how many attempts it gets on a key
// before the key escalates to the next tier. Free tiers (the local qwen
// model) are tried first; paid tiers share one rolling daily start cap.
type LadderTier struct {
	Backend string
	Budget  int
	Paid    bool
}

// LedgerRow is one line of the fixer's local attempt ledger:
// "<unix>\t<key>\t<backend>\t<started|pr|parked>[\t<count>]". A start that
// never reaches "pr" (agent error, red gate, 50m timeout kill) is a failed
// attempt. "parked" is a paid tier's verdict that the failure is not
// actionable as seen (already fixed, cannot reproduce, not a defect); Count is
// the group's occurrence count at that moment.
type LedgerRow struct {
	At      time.Time
	Key     string
	Backend string
	Event   string
	Count   int
}

// ParseLadder reads "opencode:2,cursor:2,claude:2". opencode is the free
// local model; every other backend is paid.
func ParseLadder(spec string) ([]LadderTier, error) {
	var tiers []LadderTier
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, budget, ok := strings.Cut(part, ":")
		n, err := strconv.Atoi(budget)
		if !ok || err != nil || n < 1 || name == "" {
			return nil, fmt.Errorf("ladder tier %q: want backend:attempts", part)
		}
		tiers = append(tiers, LadderTier{Backend: name, Budget: n, Paid: name != "opencode"})
	}
	if len(tiers) == 0 {
		return nil, fmt.Errorf("empty ladder %q", spec)
	}
	return tiers, nil
}

// ReadLedger skips malformed lines: a torn append must not stop the fixer.
func ReadLedger(r io.Reader) []LedgerRow {
	var rows []LedgerRow
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 4 && len(f) != 5 {
			continue
		}
		at, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil {
			continue
		}
		row := LedgerRow{At: time.Unix(at, 0), Key: f[1], Backend: f[2], Event: f[3]}
		if len(f) == 5 {
			row.Count, _ = strconv.Atoi(f[4])
		}
		rows = append(rows, row)
	}
	return rows
}

// NextBackend picks the backend for key's next attempt. Attempts count per
// backend since the key's last opened/updated PR or park, so a merged fix that
// later regresses starts again at the free tier. A parked key stays parked
// until its occurrence count (count; 0 = unknown) grows past the parked count:
// only new evidence earns another attempt. "" means nothing may run on this
// key now: parked, every available tier spent (needs a human), or the next
// tier is paid and the rolling 24h paid cap is reached.
func NextBackend(rows []LedgerRow, tiers []LadderTier, key string, count int, available map[string]bool, paidDailyCap int, now time.Time) string {
	used := map[string]int{}
	parkedAt := -1
	paidToday := 0
	paid := map[string]bool{}
	for _, t := range tiers {
		paid[t.Backend] = t.Paid
	}
	for _, r := range rows {
		if r.Event == "started" && paid[r.Backend] && now.Sub(r.At) < 24*time.Hour {
			paidToday++
		}
		if r.Key != key {
			continue
		}
		switch r.Event {
		case "pr":
			used = map[string]int{}
			parkedAt = -1
		case "parked":
			used = map[string]int{}
			parkedAt = r.Count
		case "started":
			used[r.Backend]++
		}
	}
	if parkedAt >= 0 && count <= parkedAt {
		return ""
	}
	for _, t := range tiers {
		if !available[t.Backend] || used[t.Backend] >= t.Budget {
			continue
		}
		if t.Paid && paidToday >= paidDailyCap {
			return ""
		}
		return t.Backend
	}
	return ""
}

// BlockedKeys lists ledger keys NextBackend refuses, so the picker moves on
// to other failures instead of re-picking a spent key every tick. counts maps
// key -> current occurrence count; a missing key keeps a park in force.
func BlockedKeys(rows []LedgerRow, tiers []LadderTier, counts map[string]int, available map[string]bool, paidDailyCap int, now time.Time) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rows {
		if seen[r.Key] {
			continue
		}
		seen[r.Key] = true
		if NextBackend(rows, tiers, r.Key, counts[r.Key], available, paidDailyCap, now) == "" {
			out = append(out, r.Key)
		}
	}
	return out
}
