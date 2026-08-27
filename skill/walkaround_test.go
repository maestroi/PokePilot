package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/world"
)

// blockedAt builds the *ErrBlocked WalkPath returns when the player,
// standing at (x,y), cannot take step s.
func blockedAt(x, y uint8, s world.Step) *ErrBlocked {
	e := &ErrBlocked{Step: s}
	e.At.X, e.At.Y = x, y
	return e
}

// walkAroundProbe records what each plan was told was blocked.
type walkAroundProbe struct {
	plans  []map[[2]int]bool
	walks  int
	waits  int
	blocks []error // returned by walk, in order; a short list ends in success
}

func (p *walkAroundProbe) run() error {
	return walkAround(
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			snap := map[[2]int]bool{}
			for k, v := range blocked {
				snap[k] = v
			}
			p.plans = append(p.plans, snap)
			return []world.Step{world.StepUp}, nil
		},
		func([]world.Step) error {
			p.walks++
			if p.walks <= len(p.blocks) {
				return p.blocks[p.walks-1]
			}
			return nil
		},
		func() { p.waits++ },
	)
}

// TestWalkAroundWaitsOutAWanderingSprite is the Route 1 case, measured
// 2026-08-27. An NPC walked beside the player; banning its tile on the
// first collision poisoned a corridor that the static grid says is open,
// and Traverse then reported "no reachable walkable tile on the north edge
// from (15,13)" — from a tile that reaches the north edge fine.
//
// One collision means a sprite was passing through. Wait, re-plan, ban
// NOTHING: a ban must describe something that is still there.
func TestWalkAroundWaitsOutAWanderingSprite(t *testing.T) {
	p := &walkAroundProbe{blocks: []error{blockedAt(14, 14, world.StepUp)}}
	if err := p.run(); err != nil {
		t.Fatalf("walkAround: %v", err)
	}
	if len(p.plans) != 2 {
		t.Fatalf("planned %d times, want 2 (the walk, then the re-plan)", len(p.plans))
	}
	if len(p.plans[1]) != 0 {
		t.Errorf("re-plan banned %v after ONE collision; a passing sprite must only cost a wait", p.plans[1])
	}
	if p.waits != 1 {
		t.Errorf("waited %d times, want 1: the sprite needs game time to move on", p.waits)
	}
}

// The same tile twice is something standing there, and that does get banned.
func TestWalkAroundBansATileBlockedTwice(t *testing.T) {
	stuck := blockedAt(14, 14, world.StepUp)
	p := &walkAroundProbe{blocks: []error{stuck, stuck}}
	if err := p.run(); err != nil {
		t.Fatalf("walkAround: %v", err)
	}
	if len(p.plans) != 3 {
		t.Fatalf("planned %d times, want 3", len(p.plans))
	}
	if !p.plans[2][[2]int{14, 13}] {
		t.Errorf("third plan blocked = %v, want the tile north of (14,14) banned", p.plans[2])
	}
}

// A tile that stays blocked gives up rather than looping, and the caller
// still sees the ErrBlocked underneath.
func TestWalkAroundGivesUpAfterMaxRetries(t *testing.T) {
	stuck := blockedAt(14, 14, world.StepUp)
	blocks := make([]error, maxWalkRetries+1)
	for i := range blocks {
		blocks[i] = stuck
	}
	p := &walkAroundProbe{blocks: blocks}
	var eb *ErrBlocked
	if err := p.run(); !errors.As(err, &eb) {
		t.Fatalf("err = %v, want an *ErrBlocked", err)
	}
	if p.walks != maxWalkRetries+1 {
		t.Errorf("walked %d times, want %d", p.walks, maxWalkRetries+1)
	}
}

// Anything that is not an ErrBlocked — a battle, a text box — is returned
// at once. Retrying a walk a wild encounter interrupted would walk the
// player around inside a battle.
func TestWalkAroundDoesNotRetryOtherFailures(t *testing.T) {
	p := &walkAroundProbe{blocks: []error{ErrBattleInterrupted}}
	if err := p.run(); !errors.Is(err, ErrBattleInterrupted) {
		t.Fatalf("err = %v, want ErrBattleInterrupted", err)
	}
	if p.walks != 1 {
		t.Errorf("walked %d times, want 1: a battle is not re-planned around", p.walks)
	}
}

// When planning fails while bans are held, the bans are the first thing to
// doubt: they came from sprites, which move, and the static grid does not
// lie. Dropping them and re-planning is what turns the measured Route 1
// failure into a recovery instead of a dead run.
func TestWalkAroundDropsItsOwnBansWhenPlanningFails(t *testing.T) {
	stuck := blockedAt(14, 14, world.StepUp)
	walks, plans := 0, 0
	var lastPlan map[[2]int]bool
	err := walkAround(
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			plans++
			lastPlan = blocked
			// Once anything is banned, the destination looks unreachable —
			// exactly what edgeTarget reported on Route 1.
			if len(blocked) > 0 {
				return nil, errors.New("no reachable walkable tile on the north edge")
			}
			return []world.Step{world.StepUp}, nil
		},
		func([]world.Step) error {
			walks++
			if walks <= 2 {
				return stuck // twice, so the tile gets banned
			}
			return nil
		},
		func() {},
	)
	if err != nil {
		t.Fatalf("walkAround: %v, want the bans dropped and the walk retried", err)
	}
	if len(lastPlan) != 0 {
		t.Errorf("final plan still held bans %v", lastPlan)
	}
	if plans < 4 {
		t.Errorf("planned %d times, want at least 4 (walk, re-plan, banned plan fails, cleared plan)", plans)
	}
}
