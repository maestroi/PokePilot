package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestGoldTypeChartGhostImmunityAndGrassPoison(t *testing.T) {
	if got := gsTypeEffectiveness(gsTypeNormal, gsTypeGhost, gsTypePoison); got != 0 {
		t.Fatalf("Tackle vs Gastly effectiveness = %d, want 0", got)
	}
	if got := gsTypeEffectiveness(gsTypeGrass, gsTypeGhost, gsTypePoison); got != gsTypeNotVeryEffective {
		t.Fatalf("Razor Leaf vs Gastly effectiveness = %d, want %d", got, gsTypeNotVeryEffective)
	}
}

func TestGoldCombatPrefersRazorLeafOverTackleVsGastly(t *testing.T) {
	p := NewGold()
	attacker := game.BattleCombatant{Level: 26, Type1: uint16(gsTypeGrass), Type2: uint16(gsTypeGrass), Attack: 46, SpecialAttack: 48}
	defender := game.BattleCombatant{Level: 12, Type1: uint16(gsTypeGhost), Type2: uint16(gsTypePoison), Defense: 30, SpecialDefense: 30}

	tackle, tackleRole, err := p.EvaluateCombatMove(nil, attacker, defender, 33, 35)
	if err != nil {
		t.Fatal(err)
	}
	leaf, leafRole, err := p.EvaluateCombatMove(nil, attacker, defender, 75, 25)
	if err != nil {
		t.Fatal(err)
	}
	if tackleRole != game.BattleMoveRoleDirectDamage || leafRole != game.BattleMoveRoleDirectDamage {
		t.Fatalf("roles tackle=%v leaf=%v, want direct damage", tackleRole, leafRole)
	}
	if tackle.ExpectedScore != 0 {
		t.Fatalf("Tackle vs Gastly score = %d, want 0", tackle.ExpectedScore)
	}
	if !game.BetterBattleMove(leaf, tackle) {
		t.Fatalf("Razor Leaf %+v should outrank Tackle %+v vs Gastly", leaf, tackle)
	}
}

func TestGoldGrowlIsNotDirectDamage(t *testing.T) {
	_, role, err := NewGold().EvaluateCombatMove(nil, game.BattleCombatant{}, game.BattleCombatant{}, 45, 40)
	if err != nil {
		t.Fatal(err)
	}
	if role != game.BattleMoveRoleOther {
		t.Fatalf("Growl role = %v, want other", role)
	}
}
