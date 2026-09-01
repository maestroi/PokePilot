package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestGymOutcomeErr pins the KindGym branch's classification — the whole of
// S10-2d's gym change: a win (badge in RAM) is success, a loss is a failure
// the run can see. It lives in the internal package because the decision is
// the unexported helper Execute calls, and because the emulator side of the
// branch cannot be exercised cheaply: the win side of the journey is pinned
// by skill.TestGymBoulderBadge (full journey, -short guard), and a loss
// side measured on 2026-08-31 (post_errand, trained to level 11, Route 2
// to Pewter) panics inside the vendored emulator's APU before the fight
// ends — apu.sample index out of range, a defect of that emulator, not of
// this branch.
func TestGymOutcomeErr(t *testing.T) {
	o := Objective{Kind: KindGym}

	if err := gymOutcomeErr(o, state.ResultWon); err != nil {
		t.Fatalf("gymOutcomeErr(ResultWon) = %v, want nil (the badge is in RAM: the objective did what it said)", err)
	}

	err := gymOutcomeErr(o, state.ResultLost)
	if err == nil {
		t.Fatalf("gymOutcomeErr(ResultLost) = nil, want an error (a loss is the objective NOT having done what it said)")
	}
	if !strings.Contains(err.Error(), "lost to the gym leader") {
		t.Fatalf("error = %v, want it to name the loss", err)
	}
}
