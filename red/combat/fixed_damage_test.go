package combat

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

func TestSpecialDamageMovesBypassOrdinaryFormulaAndType(t *testing.T) {
	seismic := rom.Move{
		ID: moveSeismicToss, Effect: rom.SpecialDamageEffect,
		Power: 1, Type: testNormal, Accuracy: 255, PP: 20,
	}
	// Deliberately give NORMAL an immunity in this synthetic chart. Red's
	// SPECIAL_DAMAGE_EFFECT path skips AdjustDamageForMoveType, so Seismic
	// Toss still deals the user's level rather than becoming zero damage.
	data := combatROM(t, []testTypePair{{testNormal, testFlying, 0}}, seismic)
	attacker := Combatant{Level: 37, Attack: 1, Type1: testNormal, Type2: testNormal}
	defender := Combatant{HP: 100, Defense: 999, Type1: testFlying, Type2: testFlying}

	e, err := EvaluateMove(data, attacker, defender, seismic, 12)
	if err != nil {
		t.Fatal(err)
	}
	if e.NeutralDamage != 37 || e.DamageRule != "level-damage" {
		t.Fatalf("Seismic Toss evaluation = %+v, want fixed level damage 37", e)
	}
	if e.Effectiveness != rom.NeutralEffect || e.STAB {
		t.Fatalf("fixed damage must bypass type/STAB, got eff=%d STAB=%v", e.Effectiveness, e.STAB)
	}
}

func TestDragonRageCanBeatOrdinaryMove(t *testing.T) {
	dragonRage := rom.Move{
		ID: moveDragonRage, Effect: rom.SpecialDamageEffect,
		Power: 1, Type: testFire, Accuracy: 255, PP: 10,
	}
	tackle := rom.Move{ID: 33, Power: 35, Type: testNormal, Accuracy: 255, PP: 35}
	data := combatROM(t, nil, dragonRage, tackle)
	attacker := Combatant{Level: 15, Attack: 40, Special: 40, Type1: testNormal, Type2: testNormal}
	defender := Combatant{HP: 100, Defense: 80, Special: 80, Type1: testNormal, Type2: testNormal}

	fixed, err := EvaluateMove(data, attacker, defender, dragonRage, 5)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := EvaluateMove(data, attacker, defender, tackle, 20)
	if err != nil {
		t.Fatal(err)
	}
	if fixed.NeutralDamage != 40 || !BetterMove(fixed, ordinary) {
		t.Fatalf("Dragon Rage should outrank weak ordinary attack: fixed=%+v ordinary=%+v", fixed, ordinary)
	}
}

func TestSuperFangScoresHalfCurrentHP(t *testing.T) {
	fang := rom.Move{ID: 162, Effect: rom.SuperFangEffect, Power: 1, Type: testNormal, Accuracy: 229, PP: 10}
	data := combatROM(t, nil, fang)
	e, err := EvaluateMove(data,
		Combatant{Level: 30, Attack: 100},
		Combatant{HP: 81, Defense: 200, Type1: testRock, Type2: testRock},
		fang, 6)
	if err != nil {
		t.Fatal(err)
	}
	if e.NeutralDamage != 40 || e.DamageRule != "super-fang" {
		t.Fatalf("Super Fang evaluation = %+v, want floor(81/2)=40", e)
	}
}

func TestOHKOMoveIsNotFakedAsOnePowerDamage(t *testing.T) {
	ohko := rom.Move{ID: 32, Effect: rom.OHKOEffect, Power: 1, Type: testNormal, Accuracy: 76, PP: 5}
	data := combatROM(t, nil, ohko)
	e, err := EvaluateMove(data,
		Combatant{Level: 50, Attack: 200},
		Combatant{HP: 150, Defense: 20},
		ohko, 5)
	if err != nil {
		t.Fatal(err)
	}
	if e.DamageRule != "ohko" || e.ExpectedScore != 0 || e.NeutralDamage != 0 {
		t.Fatalf("OHKO evaluation = %+v, want conservative special-rule score rather than fake formula damage", e)
	}
}
