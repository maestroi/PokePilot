package deploy

import (
	"strings"
	"testing"
	"time"
)

func TestNextBackendEscalatesFreeThenPaidThenStops(t *testing.T) {
	tiers, err := ParseLadder("opencode:2,cursor:1,claude:1")
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]bool{"opencode": true, "cursor": true, "claude": true}
	now := time.Unix(1_000_000, 0)
	var rows []LedgerRow
	start := func(backend string) {
		rows = append(rows, LedgerRow{At: now, Key: "k", Backend: backend, Event: "started"})
	}
	for _, want := range []string{"opencode", "opencode", "cursor", "claude", ""} {
		got := NextBackend(rows, tiers, "k", 0, all, 20, now)
		if got != want {
			t.Fatalf("after %d attempts got %q, want %q", len(rows), got, want)
		}
		if got != "" {
			start(got)
		}
	}
	if b := BlockedKeys(rows, tiers, nil, all, 20, now); len(b) != 1 || b[0] != "k" {
		t.Fatalf("spent key not blocked: %v", b)
	}
	if got := NextBackend(rows, tiers, "other", 0, all, 20, now); got != "opencode" {
		t.Fatalf("fresh key got %q, want free tier", got)
	}

	// An opened PR resets the key: a regression after a merge starts free again.
	rows = append(rows, LedgerRow{At: now, Key: "k", Event: "pr"})
	if got := NextBackend(rows, tiers, "k", 0, all, 20, now); got != "opencode" {
		t.Fatalf("after PR got %q, want opencode", got)
	}
}

func TestNextBackendSkipsUnavailableAndHonorsPaidCap(t *testing.T) {
	tiers, _ := ParseLadder("opencode:1,cursor:1,claude:1")
	now := time.Unix(1_000_000, 0)
	rows := ReadLedger(strings.NewReader(
		"999000\tk\topencode\tstarted\n" +
			"torn line\n" +
			"999100\tx\tcursor\tstarted\n"))
	noCursor := map[string]bool{"opencode": true, "claude": true}
	if got := NextBackend(rows, tiers, "k", 0, noCursor, 20, now); got != "claude" {
		t.Fatalf("got %q, want claude when cursor is unavailable", got)
	}
	all := map[string]bool{"opencode": true, "cursor": true, "claude": true}
	if got := NextBackend(rows, tiers, "k", 0, all, 1, now); got != "" {
		t.Fatalf("got %q, want paid cap to hold the key", got)
	}
	// The cap is a rolling 24h window.
	if got := NextBackend(rows, tiers, "k", 0, all, 1, now.Add(25*time.Hour)); got != "cursor" {
		t.Fatalf("got %q, want cursor once the paid start ages out", got)
	}
}

// A paid tier's "not actionable as seen" verdict parks the key: no tier
// retries the same evidence, and a new occurrence restarts at the free tier.
func TestParkedKeyWaitsForNewEvidence(t *testing.T) {
	tiers, _ := ParseLadder("opencode:2,cursor:2,claude:2")
	all := map[string]bool{"opencode": true, "cursor": true, "claude": true}
	now := time.Unix(1_000_000, 0)
	rows := ReadLedger(strings.NewReader(
		"999000\tk\topencode\tstarted\n" +
			"999100\tk\tcursor\tstarted\n" +
			"999200\tk\tcursor\tparked\t8\n"))
	if got := NextBackend(rows, tiers, "k", 8, all, 20, now); got != "" {
		t.Fatalf("parked key got %q on the same evidence", got)
	}
	if b := BlockedKeys(rows, tiers, map[string]int{"k": 8}, all, 20, now); len(b) != 1 {
		t.Fatalf("parked key not blocked: %v", b)
	}
	if got := NextBackend(rows, tiers, "k", 9, all, 20, now); got != "opencode" {
		t.Fatalf("new occurrence got %q, want a fresh free-tier attempt", got)
	}
}
