package main

import (
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/maestroi/pokepilot/deploy"
	"github.com/maestroi/pokepilot/operatorapi"
)

// nodeReport is one node's local facts: free space on each mount (read
// through the read-only host root) and, on the fixer node, the ledger summary.
func nodeReport(host, root string, mounts []string, ledgerPath, ladder string, paidCap int, now time.Time) operatorapi.NodeReport {
	r := operatorapi.NodeReport{Node: host, At: now.Unix()}
	for _, m := range mounts {
		var st syscall.Statfs_t
		if err := syscall.Statfs(filepath.Join(root, m), &st); err != nil {
			continue
		}
		r.Disks = append(r.Disks, operatorapi.DiskFree{Mount: m, FreeGB: int(st.Bavail * uint64(st.Bsize) >> 30)})
	}
	f, err := os.Open(filepath.Join(root, ledgerPath))
	if err != nil {
		return r
	}
	defer f.Close()
	tiers, err := deploy.ParseLadder(ladder)
	if err != nil {
		return r
	}
	s := fixerSummary(deploy.ReadLedger(f), tiers, paidCap, now)
	r.Fixer = &s
	return r
}

// fixerSummary mirrors the retired farm-watch.sh: paid/free starts in the
// rolling 24h and the keys the ladder refuses. The paid cap is passed as
// unlimited to BlockedKeys so a capped day does not mark every key blocked.
func fixerSummary(rows []deploy.LedgerRow, tiers []deploy.LadderTier, paidCap int, now time.Time) operatorapi.FixerSummary {
	s := operatorapi.FixerSummary{PaidCap: paidCap}
	paid := map[string]bool{}
	available := map[string]bool{}
	for _, t := range tiers {
		paid[t.Backend] = t.Paid
		available[deploy.BackendBase(t.Backend)] = true
	}
	for _, r := range rows {
		if now.Sub(r.At) >= 24*time.Hour {
			continue
		}
		switch r.Event {
		case "started":
			s.Attempts24h++
			if paid[r.Backend] {
				s.PaidStarts24h++
			} else {
				s.FreeStarts24h++
			}
		case "pr":
			s.PRs24h++
		}
	}
	s.BlockedKeys = deploy.BlockedKeys(rows, tiers, map[string]int{}, available, 1_000_000, now)
	return s
}
