package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/deploy"
)

func TestFixerSummaryCountsPaidFreeAndBlocked(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ts := func(ago time.Duration) string { return strconvI(now.Add(-ago).Unix()) }
	ledger := strings.Join([]string{
		ts(time.Hour) + "\tk1\topencode\tstarted",
		ts(time.Hour) + "\tk1\topencode\tstarted",
		ts(50*time.Minute) + "\tk1\tcursor\tstarted",
		ts(40*time.Minute) + "\tk1\tcursor\tstarted",
		ts(30*time.Minute) + "\tk2\tcursor\tstarted",
		ts(20*time.Minute) + "\tk2\tcursor\tpr",
		ts(30*time.Hour) + "\tk3\tcursor\tstarted", // outside 24h
	}, "\n")
	rows := deploy.ReadLedger(strings.NewReader(ledger))
	tiers, _ := deploy.ParseLadder("opencode:2,cursor:2")
	got := fixerSummary(rows, tiers, 20, now)
	if got.FreeStarts24h != 2 || got.PaidStarts24h != 3 || got.Attempts24h != 5 || got.PRs24h != 1 || got.PaidCap != 20 {
		t.Fatalf("counts: %+v", got)
	}
	if len(got.BlockedKeys) != 1 || got.BlockedKeys[0] != "k1" {
		t.Fatalf("k1 spent every tier; blocked = %v", got.BlockedKeys)
	}
}

func TestNodeReportReadsDiskAndOptionalLedger(t *testing.T) {
	root := t.TempDir()
	r := nodeReport("n1", root, []string{"/"}, "/opt/pokefixer/state/ledger.tsv", "opencode:2", 20, time.Now())
	if r.Node != "n1" || len(r.Disks) != 1 || r.Disks[0].Mount != "/" || r.Fixer != nil {
		t.Fatalf("no ledger on this node: %+v", r)
	}
	p := filepath.Join(root, "opt/pokefixer/state/ledger.tsv")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(""), 0o644)
	r = nodeReport("n1", root, []string{"/"}, "/opt/pokefixer/state/ledger.tsv", "opencode:2", 20, time.Now())
	if r.Fixer == nil {
		t.Fatal("ledger present: fixer summary expected")
	}
}

func strconvI(v int64) string { return strconv.FormatInt(v, 10) }
