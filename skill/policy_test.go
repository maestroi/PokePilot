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
// The chart is written even when it is empty — a bare terminator, meaning
// every pairing is ordinary damage. Leaving it off the end of the image
// instead would make every lookup an out-of-ROM error that moveScore
// quietly reads as neutral, so the tests would pass with a broken offset.
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
		romData[off] = mv.ID
		romData[off+1] = mv.Effect
		romData[off+2] = mv.Power
		romData[off+3] = mv.Type
	}
	for i, e := range chart {
		off := chartOffset + i*3
		romData[off], romData[off+1], romData[off+2] = e.atk, e.def, e.mult
	}
	romData[chartOffset+len(chart)*3] = 0xFF
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

// TestStatAwareMovePicksByEffectivenessNotPower is the Brock fight, and it
// FAILS on the policy as it stood before types were read: BUBBLE is weaker
// than TACKLE on paper (20 against 35) and lands for eight times as much
// against a ROCK/GROUND ONIX — quadruple for hitting both of its types, and
// half again for coming off a WATER attacker. Ranking by raw power picked
// TACKLE every time.
func TestStatAwareMovePicksByEffectivenessNotPower(t *testing.T) {
	var (
		tackleN = rom.Move{ID: 33, Power: 35, Type: typeNormal}
		bubble  = rom.Move{ID: 145, Power: 20, Type: typeWater}
	)
	chart := []typePair{
		{typeNormal, typeRock, 5},   // normal into rock: half
		{typeWater, typeRock, 20},   // water into rock: double
		{typeWater, typeGround, 20}, // water into ground: double again
	}
	p := StatAwareMove(fakeROMChart(t, chart, tackleN, bubble))

	b := battleWith(7, 7, 20, 20, tackleN.ID, bubble.ID)
	b.ActiveType1, b.ActiveType2 = typeWater, typeWater // SQUIRTLE
	b.EnemyType1, b.EnemyType2 = typeRock, typeGround   // ONIX
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1 (BUBBLE): raw power says TACKLE, the type chart says BUBBLE does eight times as much", got)
	}
}

// TestStatAwareMoveWillNotPickAnImmuneMove: an immunity is zero damage
// however strong the move is, so the weaker move that can actually connect
// has to win. A GROUND move into a FLYING opponent is the case the chart
// states as NO_EFFECT.
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
		t.Fatalf("policy chose slot %d, want 1 (TACKLE): DIG is the stronger move and does nothing at all to a FLYING opponent", got)
	}
}

// TestStatAwareMoveAppliesSTAB: with the type chart silent about both moves,
// the tie is broken by the attacker's own type. Equal power, and only one of
// them comes off a matching type.
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
		t.Fatalf("policy chose slot %d, want 1 (BUBBLE): equal power, but only BUBBLE gets the same-type bonus", got)
	}
}
