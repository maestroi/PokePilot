package skill_test

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// loadFixture restores the reds_bedroom fixture (player at map 0x26, X 3,
// Y 6). It never calls BootToOverworld directly; that is the point of PP-9.
func loadFixture(t *testing.T) *emu.Emu {
	t.Helper()
	return fixture.Load(t, "reds_bedroom")
}

func playerAt(t *testing.T, m *emu.Emu) state.PlayerState {
	t.Helper()
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodePlayer(&mem)
}

// Every expectation in this file is ground truth measured by driving the
// real ROM from a correctly booted overworld state (map 0x26, Red's
// bedroom, player at X=3 Y=6; y=7 is the bottom walkable row). Do not
// "correct" these values from a guess about the collision grid.

func TestStepOnceMoves(t *testing.T) {
	e := loadFixture(t)
	before := playerAt(t, e)
	if before.X != 3 || before.Y != 6 {
		t.Fatalf("fixture start = (%d,%d), want (3,6)", before.X, before.Y)
	}

	if err := skill.StepOnce(e, world.StepLeft); err != nil {
		t.Fatalf("StepOnce(StepLeft): %v", err)
	}
	after := playerAt(t, e)
	if after.X != 2 || after.Y != 6 {
		t.Errorf("after StepLeft = (%d,%d), want (2,6)", after.X, after.Y)
	}
}

func TestStepOnceIntoWallIsBlocked(t *testing.T) {
	e := loadFixture(t)
	before := playerAt(t, e)
	if before.X != 3 || before.Y != 6 {
		t.Fatalf("fixture start = (%d,%d), want (3,6)", before.X, before.Y)
	}

	err := skill.StepOnce(e, world.StepUp)
	if err == nil {
		t.Fatal("StepOnce(StepUp) = nil, want *ErrBlocked")
	}
	var blocked *skill.ErrBlocked
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v (%T), want *skill.ErrBlocked", err, err)
	}
	// The coordinate assertion is what keeps this test honest: without it
	// a StepOnce that reports everything as blocked would also pass.
	after := playerAt(t, e)
	if after.X != 3 || after.Y != 6 {
		t.Errorf("coords = (%d,%d), want unchanged (3,6)", after.X, after.Y)
	}
}

func TestWalkPathTwoSteps(t *testing.T) {
	e := loadFixture(t)
	path := []world.Step{world.StepLeft, world.StepLeft}
	if err := skill.WalkPath(e, path); err != nil {
		t.Fatalf("WalkPath: %v", err)
	}
	p := playerAt(t, e)
	if p.X != 1 || p.Y != 6 {
		t.Errorf("final = (%d,%d), want (1,6)", p.X, p.Y)
	}
}

func TestWalkPathIsDeterministic(t *testing.T) {
	path := []world.Step{world.StepLeft, world.StepLeft}
	var finals [2]state.PlayerState
	for i := range finals {
		e := loadFixture(t)
		if err := skill.WalkPath(e, path); err != nil {
			t.Fatalf("run %d: WalkPath: %v", i, err)
		}
		finals[i] = playerAt(t, e)
	}
	if finals[0].X != 1 || finals[0].Y != 6 {
		t.Errorf("run 0 = (%d,%d), want (1,6)", finals[0].X, finals[0].Y)
	}
	if finals[1].X != 1 || finals[1].Y != 6 {
		t.Errorf("run 1 = (%d,%d), want (1,6)", finals[1].X, finals[1].Y)
	}
}

func TestStepOnceEveryOpenDirection(t *testing.T) {
	directions := []struct {
		step world.Step
		want [2]int
	}{
		{world.StepDown, [2]int{3, 7}},
		{world.StepLeft, [2]int{2, 6}},
		{world.StepRight, [2]int{4, 6}},
	}
	for _, d := range directions {
		// Fresh fixture for every direction so the steps cannot interfere.
		e := loadFixture(t)
		if err := skill.StepOnce(e, d.step); err != nil {
			t.Fatalf("StepOnce(%s): %v", d.step, err)
		}
		p := playerAt(t, e)
		if int(p.X) != d.want[0] || int(p.Y) != d.want[1] {
			t.Errorf("after %s = (%d,%d), want (%d,%d)",
				d.step, p.X, p.Y, d.want[0], d.want[1])
		}
	}
}
