package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

// fakeCaptureMechanics bounds each move id by a fixed worst case; ids in
// unbounded have no safe bound, ids in status inflict a capture status.
type fakeCaptureMechanics struct {
	damage    map[uint16]uint16
	unbounded map[uint16]bool
	status    map[uint16]bool
}

func (f fakeCaptureMechanics) MaxCombatDamage(_ []byte, _, _ game.BattleCombatant, id uint16) (uint16, bool) {
	return f.damage[id], !f.unbounded[id]
}

func (f fakeCaptureMechanics) CaptureStatusMove(_ []byte, _ game.BattleCombatant, id uint16) bool {
	return f.status[id]
}

func TestStaticWeakeningNeverRisksAKnockOut(t *testing.T) {
	const strong, weak, explode, sleep = 1, 2, 3, 4
	mech := fakeCaptureMechanics{
		damage:    map[uint16]uint16{strong: 200, weak: 40},
		unbounded: map[uint16]bool{explode: true},
		status:    map[uint16]bool{sleep: true},
	}
	w := staticWeakening{bound: mech, status: mech}
	b := game.BattleState{EnemyHP: 150, EnemyMaxHP: 150}
	b.Moves[0] = game.BattleMove{ID: strong, PP: 5}
	b.Moves[1] = game.BattleMove{ID: explode, PP: 5}
	b.Moves[2] = game.BattleMove{ID: weak, PP: 5}
	b.Moves[3] = game.BattleMove{ID: sleep, PP: 5}

	// Weaken first with the strongest move that cannot knock it out.
	if got := w.move(b); got != 2 {
		t.Fatalf("full HP: move = %d, want the safe slot 2", got)
	}
	// At the goal (a third of max HP), put it to sleep.
	b.EnemyHP = 50
	if got := w.move(b); got != 3 {
		t.Fatalf("weakened: move = %d, want the sleep slot 3", got)
	}
	// Asleep and weakened: stop and throw balls.
	b.EnemyStatus = "asleep"
	if w.wanted(b) {
		t.Fatal("a weakened, sleeping target still wanted preparation")
	}
	// Even above the goal, a target the safe move could knock out is left alone.
	b.EnemyHP, b.EnemyStatus = 40, ""
	b.EnemyMaxHP = 200
	if got := w.safeMove(b); got != -1 {
		t.Fatalf("40 HP left: safe move = %d, want none (40 damage would faint it)", got)
	}
}

func TestStaticWeakenerPicksASafeBenchMember(t *testing.T) {
	const crush, peck = 1, 2
	mech := fakeCaptureMechanics{damage: map[uint16]uint16{crush: 300, peck: 30}}
	w := staticWeakening{bound: mech}
	b := game.BattleState{EnemyHP: 150, EnemyMaxHP: 150}
	party := game.BattleResourcesState{
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			{HP: 300, Moves: [4]game.BattlePartyMove{{NativeMoveID: crush, PP: 5}}},
			{HP: 0, Moves: [4]game.BattlePartyMove{{NativeMoveID: peck, PP: 5}}},
			{HP: 90, Moves: [4]game.BattlePartyMove{{NativeMoveID: crush, PP: 5}, {NativeMoveID: peck, PP: 5}}},
		},
	}
	if got := w.weakener(b, party); got != 2 {
		t.Fatalf("weakener = %d, want conscious slot 2 with the safe PECK", got)
	}
}
