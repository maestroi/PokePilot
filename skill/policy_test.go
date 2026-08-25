package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// fakeROM builds a ROM image holding just enough of the move table for the
// policy to read. The layout mirrors red/rom/move.go: the table starts at
// bank 0x0E:0x4000 and each entry is six bytes, with the move's own id in
// the animation byte that LookupMove sanity-checks.
func fakeROM(t *testing.T, moves ...rom.Move) []byte {
	t.Helper()
	const tableOffset = 0x0E * 0x4000
	romData := make([]byte, tableOffset+6*256)
	for _, mv := range moves {
		off := tableOffset + (int(mv.ID)-1)*6
		romData[off] = mv.ID
		romData[off+1] = mv.Effect
		romData[off+2] = mv.Power
	}
	return romData
}

// The three moves the opening of the game actually turns on.
var (
	tackle   = rom.Move{ID: 33, Power: 35}
	tailWhip = rom.Move{ID: 39, Power: 0, Effect: rom.DefenseDown1Effect}
	ember    = rom.Move{ID: 52, Power: 40}
)

func battleWith(atkMod, defMod uint8, hp, maxHP uint16, ids ...uint8) state.BattleState {
	b := state.BattleState{
		ActiveHP: hp, ActiveMaxHP: maxHP,
		ActiveAttackMod: atkMod, EnemyDefenseMod: defMod,
		ActiveDefenseMod: state.StatStageNeutral, EnemyAttackMod: state.StatStageNeutral,
	}
	for i, id := range ids {
		b.Moves[i] = state.Move{ID: id, PP: 10}
	}
	return b
}

func TestStatAwareMovePicksStrongestAttack(t *testing.T) {
	p := StatAwareMove(fakeROM(t, tackle, tailWhip, ember))
	// Slot 0 is Tackle (35), slot 1 Ember (40): power wins, not slot order.
	b := battleWith(7, 7, 20, 20, tackle.ID, ember.ID)
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1 (Ember, the stronger move)", got)
	}
}

func TestStatAwareMoveAnswersADebuffWithTailWhip(t *testing.T) {
	p := StatAwareMove(fakeROM(t, tackle, tailWhip))
	// One GROWL landed: our Attack stage is 6 against a neutral 7 Defense.
	b := battleWith(6, 7, 20, 20, tackle.ID, tailWhip.ID)
	if b.OffenceStage() != -1 {
		t.Fatalf("OffenceStage = %d, want -1", b.OffenceStage())
	}
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1 (TAIL WHIP) while behind", got)
	}
}

func TestStatAwareMoveAttacksWhenNotBehind(t *testing.T) {
	p := StatAwareMove(fakeROM(t, tackle, tailWhip))
	b := battleWith(7, 7, 20, 20, tackle.ID, tailWhip.ID)
	if got := p(b); got != 0 {
		t.Fatalf("policy chose slot %d, want 0 (TACKLE) at even stages", got)
	}
}

// Below half HP the damage race is on: another non-damaging turn loses it.
func TestStatAwareMoveAttacksWhenLowOnHP(t *testing.T) {
	p := StatAwareMove(fakeROM(t, tackle, tailWhip))
	b := battleWith(5, 7, 8, 20, tackle.ID, tailWhip.ID)
	if got := p(b); got != 0 {
		t.Fatalf("policy chose slot %d, want 0 (TACKLE) at 8/20 HP even though behind", got)
	}
}

func TestStatAwareMoveSkipsSlotsWithNoPP(t *testing.T) {
	p := StatAwareMove(fakeROM(t, tackle, ember))
	b := battleWith(7, 7, 20, 20, tackle.ID, ember.ID)
	b.Moves[1].PP = 0 // the stronger move is spent
	if got := p(b); got != 0 {
		t.Fatalf("policy chose slot %d, want 0: slot 1 has no PP left", got)
	}
}

func TestStatAwareMoveReportsNoUsableMove(t *testing.T) {
	p := StatAwareMove(fakeROM(t, tackle))
	if got := p(state.BattleState{}); got != -1 {
		t.Fatalf("policy chose slot %d on an empty move set, want -1", got)
	}
}
