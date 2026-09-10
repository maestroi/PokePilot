package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

func TestTrainerStatusRoute3AndStoryLeader(t *testing.T) {
	e := fixture.Load(t, "post_starter")
	var mem state.Mem
	state.Snapshot(e, &mem)

	ordinary, err := skill.TrainerStatusAt(e.ROM(), &mem, 0x0e, 10, 6)
	if err != nil {
		t.Fatalf("Route 3 trainer status: %v", err)
	}
	if ordinary.Defeated || !ordinary.Challengeable {
		t.Fatalf("fresh Route 3 trainer = %+v, want undefeated generic challenge", ordinary)
	}

	// Brock's object carries the trainer bit, but his text script owns badge,
	// TM and gym-state postconditions rather than the standard TalkToTrainer
	// stub. The generic observer must fail closed on it.
	if _, err := skill.TrainerStatusAt(e.ROM(), &mem, 0x36, 4, 1); err == nil {
		t.Fatal("Brock decoded as a generic standard trainer")
	}
}

func TestChallengeTrainerExplicitInteractionAndIdempotence(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed trainer journey")
	}
	e := fixture.Load(t, "post_boulder")
	policy := skill.StatAwareMove(e.ROM())

	// Enter Route 3 on the west side, before the first trainer. Youngster 1
	// faces RIGHT while we approach from the west, so this reaches the
	// adjacent tile without his line-of-sight firing and exercises the explicit
	// A -> trainer dialogue -> Battle path.
	entry := skill.Destination{Map: 0x0e, X: 3, Y: 7}
	if _, err := skill.TravelFlee(e, e.ROM(), entry, policy, 20); err != nil {
		t.Fatalf("reach Route 3 west side: %v", err)
	}

	var before state.Mem
	state.Snapshot(e, &before)
	status, err := skill.TrainerStatusAt(e.ROM(), &before, 0x0e, 10, 6)
	if err != nil {
		t.Fatalf("status before challenge: %v", err)
	}
	if status.Defeated {
		t.Fatal("first Route 3 trainer already defeated before challenge")
	}

	if err := skill.ChallengeTrainer(e, e.ROM(), 10, 6, policy); err != nil {
		t.Fatalf("ChallengeTrainer: %v", err)
	}
	var after state.Mem
	state.Snapshot(e, &after)
	status, err = skill.TrainerStatusAt(e.ROM(), &after, 0x0e, 10, 6)
	if err != nil {
		t.Fatalf("status after challenge: %v", err)
	}
	if !status.Defeated {
		t.Fatal("ChallengeTrainer returned success without the trainer fought flag")
	}

	x, y := after.U8(0xD362), after.U8(0xD361) // wXCoord / wYCoord
	if err := skill.ChallengeTrainer(e, e.ROM(), 10, 6, policy); err != nil {
		t.Fatalf("idempotent ChallengeTrainer: %v", err)
	}
	var repeat state.Mem
	state.Snapshot(e, &repeat)
	if repeat.U8(0xD362) != x || repeat.U8(0xD361) != y {
		t.Fatalf("already-defeated challenge moved player: (%d,%d) -> (%d,%d)", x, y, repeat.U8(0xD362), repeat.U8(0xD361))
	}
}
