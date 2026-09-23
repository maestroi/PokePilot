package main

// inheritRunPolicyLocked fills a resumed child's missing gameplay policy from
// the settled parent it continues. Policy normally travels with the run (the
// wall copies it when it queues an endless successor or a manual clone), so
// this only matters for a child restored from an older state file that
// predates the policy fields on Tile. Fields the child set itself always win.
//
// w.mu must be held.
func (w *Wall) inheritRunPolicyLocked(t *Tile) {
	if t == nil || t.ResumeFromRunID == "" {
		return
	}
	parent := w.tiles[t.ResumeFromRunID]
	if parent == nil {
		return
	}
	if t.PlayStyle == "" {
		t.PlayStyle = parent.PlayStyle
	}
	if t.RiskTolerance == "" {
		t.RiskTolerance = parent.RiskTolerance
	}
	if t.WildEncounters == "" {
		t.WildEncounters = parent.WildEncounters
	}
}
