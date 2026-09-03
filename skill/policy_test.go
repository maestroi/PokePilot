package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// Type ids, constants/type_constants.asm.
const (
	typeNormal uint8 = 0x00
	typeGround uint8 = 0x04
	typeRock   uint8 = 0x05
	typeFire   uint8 = 0x14
	typeWater  uint8 = 0x15
)

// typePair is one row of the ROM's type chart: attacker, defender, and the
// multiplier in tenths.
type typePair struct{ atk, def, mult uint8 }

// fakeROM builds a ROM image holding just enough of the move table and the
// type chart for the policy to read. The layout mirrors red/rom: the move
// table starts at bank 0x0E:0x4000 with six-byte entries carrying the move's
// own id in the animation byte LookupMove sanity-checks, and TypeEffects
// sits at 0x0F:0x6474 as three-byte rows ended by 0xFF.
//
// Tests written before accuracy/PP were part of the scorer leave those Move
// fields zero; the helper fills them with neutral legal defaults. Tests that
// care about either field set it explicitly.
func fakeROM(t *testing.T, moves ...rom.Move) []byte {
	return fakeROMChart(t, nil, moves...)
}

func fakeROMChart(t *testing.T, chart []typePair, moves ...rom.Move) []byte {
	t.Helper()
	const (
		tableOffset = 0x0E * 0x4000
		chartOffset = 0x0F*0x4000 + (0x6474 - 0x4000)
	)
	romData := make([]byte, chartOffset+3*(len(chart)+1))
	for _, mv := range moves {
		off := tableOffset + (int(mv.ID)-1)*6
		accuracy := mv.Accuracy
		if accuracy == 0 {
			accuracy = 255
		}
		pp := mv.PP
		if pp == 0 {
			pp = 20
		}
		romData[off] = mv.ID
		romData[off+1] = mv.Effect
		romData[off+2] = mv.Power
		romData[off+3] = mv.Type
		romData[off+4] = accuracy
		romData[off+5] = pp
	}
	for i, e := range chart {
		off := chartOffset + i*3
		romData[off], romData[off+1], romData[off+2] = e.atk, e.def, e.mult
	}
	romData[chartOffset+len(chart)*3] = 0xFF
	return romData
}

// The three moves the opening of the game actually turns on. The Ember test
// fixture keeps its type neutral here because the legacy strongest-attack
// test isolates power only; dedicated tests below cover real special typing.
var (
	tackle   = rom.Move{ID: 33, Power: 35}
	tailWhip = rom.Move{ID: 39, Power: 0, Effect: rom.DefenseDown1Effect}
	ember    = rom.Move{ID: 52, Power: 40}
)

// battleWith builds a neutral but possible battle rather than leaving the
// newly scored level/stats at their zero values. Level 20 and equal 70 stats
// make power differences visible through the same integer damage formula the
// production scorer uses; individual tests override the matchup when needed.
func battleWith(atkMod, defMod uint8, hp, maxHP uint16, ids ...uint8) state.BattleState {
	b := state.BattleState{
		ActiveHP: hp, ActiveMaxHP: maxHP, ActiveLevel: 20,
		ActiveAttack: 70, ActiveSpecial: 70,
		EnemyDefense: 70, EnemySpecial: 70,
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
	// Slot 0 is Tackle (35), slot 1 Ember (40): power wins when the rest of
	// the matchup is neutral.
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

// TestStatAwareMovePicksByEffectivenessNotPower is the Brock fight: BUBBLE
// is weaker than TACKLE on paper and lands for eight times as much against a
// ROCK/GROUND ONIX once both weaknesses and WATER STAB are included.
func TestStatAwareMovePicksByEffectivenessNotPower(t *testing.T) {
	var (
		tackleN = rom.Move{ID: 33, Power: 35, Type: typeNormal}
		bubble  = rom.Move{ID: 145, Power: 20, Type: typeWater}
	)
	chart := []typePair{
		{typeNormal, typeRock, 5},
		{typeWater, typeRock, 20},
		{typeWater, typeGround, 20},
	}
	p := StatAwareMove(fakeROMChart(t, chart, tackleN, bubble))

	b := battleWith(7, 7, 20, 20, tackleN.ID, bubble.ID)
	b.ActiveType1, b.ActiveType2 = typeWater, typeWater
	b.EnemyType1, b.EnemyType2 = typeRock, typeGround
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1 (BUBBLE): raw power says TACKLE, matchup says BUBBLE", got)
	}
}

func TestStatAwareMoveWillNotPickAnImmuneMove(t *testing.T) {
	const typeFlying uint8 = 0x02
	var (
		dig     = rom.Move{ID: 91, Power: 100, Type: typeGround}
		tackleN = rom.Move{ID: 33, Power: 35, Type: typeNormal}
	)
	chart := []typePair{{typeGround, typeFlying, 0}}
	p := StatAwareMove(fakeROMChart(t, chart, dig, tackleN))

	b := battleWith(7, 7, 20, 20, dig.ID, tackleN.ID)
	b.ActiveType1, b.ActiveType2 = typeNormal, typeNormal
	b.EnemyType1, b.EnemyType2 = typeFlying, typeFlying
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1 (TACKLE): DIG does no damage to FLYING", got)
	}
}

func TestStatAwareMoveAppliesSTAB(t *testing.T) {
	var (
		tackleN = rom.Move{ID: 33, Power: 40, Type: typeNormal}
		bubble  = rom.Move{ID: 145, Power: 40, Type: typeWater}
	)
	p := StatAwareMove(fakeROMChart(t, nil, tackleN, bubble))

	b := battleWith(7, 7, 20, 20, tackleN.ID, bubble.ID)
	b.ActiveType1, b.ActiveType2 = typeWater, typeWater
	b.EnemyType1, b.EnemyType2 = typeNormal, typeNormal
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1 (BUBBLE): equal power, only BUBBLE gets STAB", got)
	}
}

func TestStatAwareMoveAccountsForAccuracy(t *testing.T) {
	// A 120-power 50%-accurate physical attack has lower expected turn value
	// than an 80-power fully accurate one in an otherwise identical matchup.
	wild := rom.Move{ID: 25, Power: 120, Type: typeNormal, Accuracy: 127}
	reliable := rom.Move{ID: 70, Power: 80, Type: typeNormal, Accuracy: 255}
	p := StatAwareMove(fakeROM(t, wild, reliable))
	b := battleWith(7, 7, 30, 30, wild.ID, reliable.ID)
	b.ActiveLevel = 30
	b.ActiveAttack, b.EnemyDefense = 80, 80
	b.ActiveType1, b.ActiveType2 = typeNormal, typeNormal
	b.EnemyType1, b.EnemyType2 = typeNormal, typeNormal
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1: reliable expected damage beats the inaccurate high-power move", got)
	}
}

func TestStatAwareMoveUsesGen1PhysicalSpecialStats(t *testing.T) {
	physical := rom.Move{ID: 1, Power: 90, Type: typeNormal, Accuracy: 255}
	special := rom.Move{ID: 55, Power: 70, Type: typeWater, Accuracy: 255}
	p := StatAwareMove(fakeROM(t, physical, special))
	b := battleWith(7, 7, 30, 30, physical.ID, special.ID)
	b.ActiveLevel = 30
	b.ActiveAttack, b.EnemyDefense = 40, 120
	b.ActiveSpecial, b.EnemySpecial = 120, 50
	b.ActiveType1, b.ActiveType2 = typeFire, typeFire
	b.EnemyType1, b.EnemyType2 = typeNormal, typeNormal
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1: lower-power WATER uses the much stronger Special matchup", got)
	}
}

func TestStatAwareMoveUsesPPAsTieBreak(t *testing.T) {
	a := rom.Move{ID: 1, Power: 50, Type: typeNormal, Accuracy: 255, PP: 20}
	bmove := rom.Move{ID: 10, Power: 50, Type: typeNormal, Accuracy: 255, PP: 20}
	p := StatAwareMove(fakeROM(t, a, bmove))
	b := battleWith(7, 7, 30, 30, a.ID, bmove.ID)
	b.ActiveLevel = 20
	b.ActiveAttack, b.EnemyDefense = 70, 70
	b.Moves[0].PP = 2
	b.Moves[1].PP = 9
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1: equal expected damage should spend the move with more PP remaining", got)
	}
}
