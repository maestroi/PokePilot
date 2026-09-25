package skill

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// delayedHealBoundaryClock reproduces the boundary race from #1756: the
// overworld looks controllable briefly, then the nurse script publishes one
// last ordinary text page. A single-snapshot success check returns too early.
type delayedHealBoundaryClock struct {
	base   *fakeClock
	openAt int
	opened bool
}

func (f *delayedHealBoundaryClock) PeekInto(addr uint16, dst []byte) {
	f.base.PeekInto(addr, dst)
}

func (f *delayedHealBoundaryClock) Tap(b emu.Button, holdFrames, gapFrames int) {
	f.base.Tap(b, holdFrames, gapFrames)
}

func (f *delayedHealBoundaryClock) StepFrame() {
	f.base.StepFrame()
	if !f.opened && f.base.steps >= f.openAt {
		f.opened = true
		openTextBox(f.base.mem, "WE HOPE TO SEE YOU AGAIN")
	}
}

func (f *delayedHealBoundaryClock) StepFrames(n int) {
	for i := 0; i < n; i++ {
		f.StepFrame()
	}
}

func TestSettleHealBoundarySurvivesTransientControllableGap(t *testing.T) {
	mem := newFakeRAM()
	base := &fakeClock{mem: mem, closesOnTap: true}
	clock := &delayedHealBoundaryClock{base: base, openAt: 5}

	if err := settleHealBoundary(clock, 500); err != nil {
		t.Fatalf("settleHealBoundary: %v", err)
	}
	if !clock.opened {
		t.Fatal("test did not publish the delayed nurse page")
	}
	if base.taps != 1 {
		t.Fatalf("taps = %d, want exactly one A to close delayed ordinary text", base.taps)
	}
	var final state.Mem
	state.Snapshot(clock, &final)
	if !state.Controllable(&final) {
		t.Fatal("settle returned before the overworld was controllable")
	}
	minSteps := clock.openAt + talkSettle + healBoundaryStableFrames - 1
	if base.steps < minSteps {
		t.Fatalf("steps = %d, want at least %d so controllability was re-established for the full stability window", base.steps, minSteps)
	}
}

func TestSettleHealBoundaryNeverAnswersLateChoice(t *testing.T) {
	mem := newFakeRAM()
	openChoice(mem, 8, 12, "HEAL AGAIN")
	base := &fakeClock{mem: mem, closesOnTap: true}

	err := settleHealBoundary(base, 100)
	if err == nil || !strings.Contains(err.Error(), "unexpected choice") {
		t.Fatalf("settleHealBoundary error = %v, want unexpected choice", err)
	}
	if base.taps != 0 {
		t.Fatalf("taps = %d, want zero input on a late choice", base.taps)
	}
}
