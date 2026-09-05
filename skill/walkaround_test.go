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

// walkAroundProbe records what each readBlocked returned and what each plan
// was told was blocked.
type walkAroundProbe struct {
	reads     []map[[2]int]bool // returned by readBlocked, in order; past the end it returns an empty map
	readCalls int
	plans     []map[[2]int]bool // each plan's blocked argument, snapshotted, in order
	walks     int
	waits     int
	blocks    []error // returned by walk, in order; a short list ends in success
}

func (p *walkAroundProbe) run() error {
	return walkAround(
		func() map[[2]int]bool {
			i := p.readCalls
			p.readCalls++
			if i < len(p.reads) {
				return p.reads[i]
			}
			return map[[2]int]bool{}
		},
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

// TestWalkAroundRereadsBlockersAfterCollision is the S5c-5a contract: a
// collision is not learned, it is re-measured. Read 1 sees the sprite at
// (14,13) and the first walk runs into it; read 2 sees the sprite moved to
// (15,13) and the second walk gets through. Both snapshots must reach plan,
// in order.
func TestWalkAroundRereadsBlockersAfterCollision(t *testing.T) {
	p := &walkAroundProbe{
		reads:  []map[[2]int]bool{{[2]int{14, 13}: true}, {[2]int{15, 13}: true}},
		blocks: []error{blockedAt(14, 14, world.StepUp)},
	}
	if err := p.run(); err != nil {
		t.Fatalf("walkAround: %v", err)
	}
	if len(p.plans) != 2 {
		t.Fatalf("planned %d times, want 2 (the walk, then the re-plan)", len(p.plans))
	}
	if !p.plans[0][[2]int{14, 13}] || len(p.plans[0]) != 1 {
		t.Errorf("first plan blocked = %v, want exactly {(14,13)} from read 1", p.plans[0])
	}
	if !p.plans[1][[2]int{15, 13}] || len(p.plans[1]) != 1 {
		t.Errorf("second plan blocked = %v, want exactly {(15,13)} from read 2", p.plans[1])
	}
	if p.waits != 1 {
		t.Errorf("waited %d times, want 1: the sprite needs game time to move on", p.waits)
	}
}

// TestWalkAroundLearnsRepeatedUnexplainedBlock is the Mt. Moon regression:
// the static grid and sprite snapshot both say (9,22) is available, but real
// movement from (10,22) repeatedly refuses StepLeft. One miss is treated as
// a possible sprite race; the second makes (9,22) a call-local blocker so the
// third plan can take an alternate route instead of repeating the same step
// until maxWalkRetries.
func TestWalkAroundLearnsRepeatedUnexplainedBlock(t *testing.T) {
	target := [2]int{9, 22}
	var plans []map[[2]int]bool
	walks := 0
	waits := 0

	err := walkAround(
		func() map[[2]int]bool { return map[[2]int]bool{} },
		func(blocked map[[2]int]bool) ([]world.Step, error) {
			snap := map[[2]int]bool{}
			for k, v := range blocked {
				snap[k] = v
			}
			plans = append(plans, snap)
			if blocked[target] {
				return []world.Step{world.StepRight}, nil
			}
			return []world.Step{world.StepLeft}, nil
		},
		func(steps []world.Step) error {
			walks++
			if len(steps) != 1 {
				t.Fatalf("walk %d got %d steps, want exactly 1", walks, len(steps))
			}
			if steps[0] == world.StepLeft {
				return blockedAt(10, 22, world.StepLeft)
			}
			if steps[0] != world.StepRight {
				t.Fatalf("walk %d step = %v, want left or right", walks, steps[0])
			}
			return nil
		},
		func() { waits++ },
	)
	if err != nil {
		t.Fatalf("walkAround: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("planned %d times, want 3: two confirmations then one reroute", len(plans))
	}
	if plans[0][target] || plans[1][target] {
		t.Errorf("target %v learned too early: plans = %v", target, plans)
	}
	if !plans[2][target] {
		t.Errorf("third plan blockers = %v, want learned target %v", plans[2], target)
	}
	if walks != 3 {
		t.Errorf("walked %d times, want 3", walks)
	}
	if waits != 2 {
		t.Errorf("waited %d times, want 2", waits)
	}
}

// TestWalkAroundForgetsVacatedSpriteTile proves the absence of a live-sprite
// cache without a second collision: read 1 sees the sprite at (14,13), the
// walk collides, read 2 sees it gone. The second plan must not receive a
// remembered copy of (14,13).
func TestWalkAroundForgetsVacatedSpriteTile(t *testing.T) {
	p := &walkAroundProbe{
		reads:  []map[[2]int]bool{{[2]int{14, 13}: true}, {}},
		blocks: []error{blockedAt(14, 14, world.StepUp)},
	}
	if err := p.run(); err != nil {
		t.Fatalf("walkAround: %v", err)
	}
	if len(p.plans) != 2 {
		t.Fatalf("planned %d times, want 2", len(p.plans))
	}
	if p.plans[1][[2]int{14, 13}] {
		t.Errorf("second plan still carries %v from read 1; a vacated sprite tile must not survive into a new snapshot", p.plans[1])
	}
	if len(p.plans[1]) != 0 {
		t.Errorf("second plan blocked = %v, want empty: read 2 saw no sprites", p.plans[1])
	}
}

// A tile that stays blocked gives up rather than looping, and the caller
// still sees the ErrBlocked underneath. Learning that tile is one piece of
// new runtime evidence, so the stagnant-retry budget restarts once; after
// that, the unchanged bad plan must still terminate.
func TestWalkAroundGivesUpAfterMaxRetries(t *testing.T) {
	stuck := blockedAt(14, 14, world.StepUp)
	wantWalks := maxWalkRetries + unexplainedBlockLearnThreshold + 1
	blocks := make([]error, wantWalks)
	for i := range blocks {
		blocks[i] = stuck
	}
	p := &walkAroundProbe{blocks: blocks}
	var eb *ErrBlocked
	if err := p.run(); !errors.As(err, &eb) {
		t.Fatalf("err = %v, want an *ErrBlocked", err)
	}
	if p.walks != wantWalks {
		t.Errorf("walked %d times, want %d", p.walks, wantWalks)
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
