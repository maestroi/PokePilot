package combat

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

const (
	testNormal  uint8 = 0x00
	testFlying  uint8 = 0x02
	testGround  uint8 = 0x04
	testRock    uint8 = 0x05
	testFire    uint8 = 0x14
	testWater   uint8 = 0x15
	testPsychic uint8 = 0x18
)

type testTypePair struct{ atk, def, mult uint8 }

func combatROM(t *testing.T, chart []testTypePair, moves ...rom.Move) []byte {
	t.Helper()
	const (
		moveOffset  = 0x0E * 0x4000
		chartOffset = 0x0F*0x4000 + (0x6474 - 0x4000)
	)
	data := make([]byte, chartOffset+3*(len(chart)+1))
	for _, mv := range moves {
		off := moveOffset + (int(mv.ID)-1)*6
		data[off] = mv.ID
		data[off+1] = mv.Effect
		data[off+2] = mv.Power
		data[off+3] = mv.Type
		data[off+4] = mv.Accuracy
		data[off+5] = mv.PP
	}
	for i, p := range chart {
		off := chartOffset + i*3
		data[off], data[off+1], data[off+2] = p.atk, p.def, p.mult
	}
	data[chartOffset+len(chart)*3] = 0xff
	return data
}

func TestGen1PhysicalSpecialBoundary(t *testing.T) {
	for _, tc := range []struct {
		typeID   uint8
		physical bool
	}{
		{testNormal, true},
		{testFlying, true},
		{testRock, true},
		{0x08, true}, // GHOST is physical in Gen 1
		{testFire, false},
		{testWater, false},
		{testPsychic, false},
	} {
		if got := IsPhysicalType(tc.typeID); got != tc.physical {
			t.Errorf("IsPhysicalType(%#02x) = %v, want %v", tc.typeID, got, tc.physical)
		}
	}
}

func TestEvaluateMoveUsesSelectedStatPair(t *testing.T) {
	attacker := Combatant{Level: 30, Attack: 40, Special: 120, Type1: testFire, Type2: testFire}
	defender := Combatant{Defense: 120, Special: 50, Type1: testNormal, Type2: testNormal}
	physical := rom.Move{ID: 1, Power: 90, Type: testNormal, Accuracy: 255, PP: 20}
	special := rom.Move{ID: 55, Power: 70, Type: testWater, Accuracy: 255, PP: 20}
	data := combatROM(t, nil, physical, special)

	p, err := EvaluateMove(data, attacker, defender, physical, 10)
	if err != nil {
		t.Fatal(err)
	}
	s, err := EvaluateMove(data, attacker, defender, special, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Physical || p.AttackStat != 40 || p.DefenseStat != 120 {
		t.Fatalf("physical evaluation = %+v, want Attack 40 vs Defense 120", p)
	}
	if s.Physical || s.AttackStat != 120 || s.DefenseStat != 50 {
		t.Fatalf("special evaluation = %+v, want Special 120 vs Special 50", s)
	}
	if !BetterMove(s, p) {
		t.Fatalf("special evaluation should outrank physical: special=%+v physical=%+v", s, p)
	}
}

func TestEvaluateMoveCombinesSTABDualTypeAndImmunity(t *testing.T) {
	water := rom.Move{ID: 55, Power: 40, Type: testWater, Accuracy: 255, PP: 25}
	ground := rom.Move{ID: 91, Power: 100, Type: testGround, Accuracy: 255, PP: 10}
	data := combatROM(t, []testTypePair{
		{testWater, testRock, 20},
		{testWater, testGround, 20},
		{testGround, testFlying, 0},
	}, water, ground)
	attacker := Combatant{Level: 20, Attack: 70, Special: 70, Type1: testWater, Type2: testWater}

	quad, err := EvaluateMove(data, attacker, Combatant{Defense: 70, Special: 70, Type1: testRock, Type2: testGround}, water, 12)
	if err != nil {
		t.Fatal(err)
	}
	if quad.Effectiveness != 40 || !quad.STAB {
		t.Fatalf("water vs rock/ground = %+v, want 4x + STAB", quad)
	}

	immune, err := EvaluateMove(data, attacker, Combatant{Defense: 70, Special: 70, Type1: testFlying, Type2: testFlying}, ground, 5)
	if err != nil {
		t.Fatal(err)
	}
	if immune.Effectiveness != 0 || immune.ExpectedScore != 0 {
		t.Fatalf("ground vs flying = %+v, want immunity score 0", immune)
	}
}

func TestEvaluateMoveIncludesAccuracy(t *testing.T) {
	attacker := Combatant{Level: 30, Attack: 80, Type1: testNormal, Type2: testNormal}
	defender := Combatant{Defense: 80, Type1: testNormal, Type2: testNormal}
	wild := rom.Move{ID: 25, Power: 120, Type: testNormal, Accuracy: 127, PP: 5}
	reliable := rom.Move{ID: 70, Power: 80, Type: testNormal, Accuracy: 255, PP: 15}
	data := combatROM(t, nil, wild, reliable)

	a, err := EvaluateMove(data, attacker, defender, wild, 5)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EvaluateMove(data, attacker, defender, reliable, 15)
	if err != nil {
		t.Fatal(err)
	}
	if !BetterMove(b, a) {
		t.Fatalf("reliable move should win expected score: reliable=%+v wild=%+v", b, a)
	}
}

func TestBetterMoveUsesPPOnlyAfterDamageTie(t *testing.T) {
	a := MoveEvaluation{ExpectedScore: 1000, CurrentPP: 2, Accuracy: 255}
	b := MoveEvaluation{ExpectedScore: 1000, CurrentPP: 9, Accuracy: 255}
	if !BetterMove(b, a) {
		t.Fatal("equal damage should prefer more current PP")
	}
	// PP never overrides actual expected damage.
	b.ExpectedScore = 999
	b.CurrentPP = 63
	if BetterMove(b, a) {
		t.Fatal("larger PP pool must not beat a higher expected-damage move")
	}
}

func TestEvaluationDiagnosticExplainsFactors(t *testing.T) {
	e := MoveEvaluation{
		MoveID: 55, Physical: false, AttackStat: 120, DefenseStat: 50,
		NeutralDamage: 20, Effectiveness: 20, STAB: true, Accuracy: 242,
		CurrentPP: 8, MaxPP: 25, ExpectedScore: 12345,
	}
	s := e.String()
	for _, want := range []string{"class=special", "stats=120/50", "eff=2.0x", "stab=1.5x", "acc=242/255", "pp=8/25"} {
		if !strings.Contains(s, want) {
			t.Fatalf("diagnostic %q does not contain %q", s, want)
		}
	}
}
