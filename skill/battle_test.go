package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// TestFirstUsableMove is pure unit coverage: no emulator, no ROM.
func TestFirstUsableMove(t *testing.T) {
	t.Run("all four have PP picks slot 0", func(t *testing.T) {
		b := state.BattleState{
			Moves: [4]state.Move{
				{ID: 1, PP: 10},
				{ID: 2, PP: 10},
				{ID: 3, PP: 10},
				{ID: 4, PP: 10},
			},
		}
		if got := skill.FirstUsableMove(b); got != 0 {
			t.Errorf("FirstUsableMove = %d, want 0", got)
		}
	})

	t.Run("slots 0 and 1 out of PP picks slot 2", func(t *testing.T) {
		b := state.BattleState{
			Moves: [4]state.Move{
				{ID: 1, PP: 0},
				{ID: 2, PP: 0},
				{ID: 3, PP: 10},
				{ID: 4, PP: 10},
			},
		}
		if got := skill.FirstUsableMove(b); got != 2 {
			t.Errorf("FirstUsableMove = %d, want 2", got)
		}
	})

	t.Run("empty slots are skipped", func(t *testing.T) {
		b := state.BattleState{
			Moves: [4]state.Move{
				{ID: 0, PP: 10},
				{ID: 0, PP: 10},
				{ID: 3, PP: 5},
				{ID: 0, PP: 0},
			},
		}
		if got := skill.FirstUsableMove(b); got != 2 {
			t.Errorf("FirstUsableMove = %d, want 2", got)
		}
	})

	t.Run("nothing usable returns -1", func(t *testing.T) {
		b := state.BattleState{}
		if got := skill.FirstUsableMove(b); got != -1 {
			t.Errorf("FirstUsableMove = %d, want -1", got)
		}
	})
}

// TestBattleNoBattleInProgress is ROM-gated: without POKEMON_RED_ROM the
// fixture loader skips. A failure assertion must also assert nothing
// changed (docs/DESIGN.md 3.2b).
func TestBattleNoBattleInProgress(t *testing.T) {
	e := loadFixture(t)

	before := playerAt(t, e)
	if before.X != 3 || before.Y != 6 {
		t.Fatalf("fixture start = (%d,%d), want (3,6)", before.X, before.Y)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	if !state.Controllable(&mem) {
		t.Fatal("fixture not controllable at start")
	}

	result, err := skill.Battle(e, skill.FirstUsableMove)
	if err == nil {
		t.Fatalf("Battle = %v, nil error; want error (no battle in progress)", result)
	}

	after := playerAt(t, e)
	if before.MapID != after.MapID || before.X != after.X || before.Y != after.Y {
		t.Errorf("player changed: before %+v, after %+v", before, after)
	}
	state.Snapshot(e, &mem)
	if !state.Controllable(&mem) {
		t.Error("player not controllable after failed Battle")
	}
}
