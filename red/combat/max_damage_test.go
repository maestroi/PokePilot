package combat

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

const testElectric uint8 = 0x17

func TestMaxDamageBoundsCriticalTopRollAndHits(t *testing.T) {
	tackle := rom.Move{ID: 0x21, Power: 40, Type: testNormal, Accuracy: 255, PP: 35}
	surf := rom.Move{ID: 0x39, Power: 40, Type: testWater, Accuracy: 255, PP: 15}
	furySwipes := rom.Move{ID: 0x9A, Effect: twoToFiveEffect, Power: 40, Type: testNormal, Accuracy: 255, PP: 15}
	data := combatROM(t, []testTypePair{{testWater, testFire, 20}}, tackle, surf, furySwipes)
	attacker := Combatant{Level: 50, Attack: 100, Special: 100, Type1: testWater, Type2: testWater}
	defender := Combatant{Level: 50, HP: 150, MaxHP: 150, Defense: 100, Special: 100, Type1: testFire, Type2: testFire}

	// Critical hit doubles the level: ((2*100/5+2)*40*100/100)/50 + 2 = 35.
	for _, tc := range []struct {
		mv   rom.Move
		want uint16
	}{
		{tackle, 35},
		{surf, 35 * 15 / 10 * 2}, // STAB, then super effective
		{furySwipes, 35 * 5},     // every one of up to five hits
	} {
		got, ok := MaxDamage(data, attacker, defender, tc.mv)
		if !ok || got != tc.want {
			t.Errorf("MaxDamage(move %#02x) = %d, %v; want %d", tc.mv.ID, got, ok, tc.want)
		}
		eval, err := EvaluateMove(data, attacker, defender, tc.mv, tc.mv.PP)
		if err != nil || got < eval.NeutralDamage {
			t.Errorf("move %#02x: bound %d below the ordinary roll %d (err %v)", tc.mv.ID, got, eval.NeutralDamage, err)
		}
	}
}

func TestMaxDamageFixedAndUnboundedMoves(t *testing.T) {
	attacker := Combatant{Level: 60, Attack: 100, Defense: 100}
	defender := Combatant{Level: 50, HP: 151, MaxHP: 160, Defense: 100}
	for _, tc := range []struct {
		name string
		mv   rom.Move
		want uint16
		ok   bool
	}{
		{"seismic toss", rom.Move{ID: moveSeismicToss, Effect: rom.SpecialDamageEffect, Power: 1}, 60, true},
		{"psywave", rom.Move{ID: movePsywave, Effect: rom.SpecialDamageEffect, Power: 1}, 90, true},
		{"super fang", rom.Move{ID: 0xA2, Effect: rom.SuperFangEffect, Power: 1}, 75, true},
		{"status move", rom.Move{ID: 0x56, Effect: paralyzeEffect}, 0, true},
		{"explosion", rom.Move{ID: 0x99, Effect: rom.ExplodeEffect, Power: 170}, 0, false},
		{"horn drill", rom.Move{ID: 0x20, Effect: rom.OHKOEffect, Power: 1}, 0, false},
		{"thrash", rom.Move{ID: 0x25, Effect: thrashEffect, Power: 90}, 0, false},
		{"wrap", rom.Move{ID: 0x23, Effect: trappingEffect, Power: 15}, 0, false},
		{"counter", rom.Move{ID: moveCounter, Power: 1}, 0, false},
	} {
		got, ok := MaxDamage(nil, attacker, defender, tc.mv)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: MaxDamage = %d, %v; want %d, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCaptureStatusRespectsTypeImmunity(t *testing.T) {
	hypnosis := rom.Move{ID: 0x5F, Effect: sleepEffect, Type: testPsychic, Accuracy: 153}
	thunderWave := rom.Move{ID: 0x56, Effect: paralyzeEffect, Type: testElectric, Accuracy: 255}
	growl := rom.Move{ID: 0x2D, Effect: 0x12, Type: testNormal, Accuracy: 255}
	data := combatROM(t, []testTypePair{{testElectric, testGround, 0}}, hypnosis, thunderWave, growl)
	flying := Combatant{Type1: testFire, Type2: testFlying}
	ground := Combatant{Type1: testGround, Type2: testGround}

	if !CaptureStatus(data, flying, hypnosis) || !CaptureStatus(data, ground, hypnosis) {
		t.Error("a sleep move must count against any type")
	}
	if !CaptureStatus(data, flying, thunderWave) {
		t.Error("Thunder Wave must count against a Fire/Flying target")
	}
	if CaptureStatus(data, ground, thunderWave) {
		t.Error("Thunder Wave never lands on a Ground type")
	}
	if CaptureStatus(data, flying, growl) {
		t.Error("a stat-lowering move is not a capture status")
	}
}
