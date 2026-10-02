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

func TestGoldGrowlAndReflectAreSetupMoves(t *testing.T) {
	p := NewGold()
	_, growlRole, err := p.EvaluateCombatMove(nil, game.BattleCombatant{}, game.BattleCombatant{}, gsMoveGrowl, 40)
	if err != nil {
		t.Fatal(err)
	}
	if growlRole != game.BattleMoveRoleSetup {
		t.Fatalf("Growl role = %v, want setup", growlRole)
	}
	_, reflectRole, err := p.EvaluateCombatMove(nil, game.BattleCombatant{}, game.BattleCombatant{}, gsMoveReflect, 20)
	if err != nil {
		t.Fatal(err)
	}
	if reflectRole != game.BattleMoveRoleSetup {
		t.Fatalf("Reflect role = %v, want setup", reflectRole)
	}
}

func TestGoldPreferSetupMoveUsesReflectThenGrowl(t *testing.T) {
	p := NewGold()
	b := game.BattleState{
		ActiveHP:       50,
		ActiveMaxHP:    62,
		EnemyLevel:     16,
		EnemyAttackMod: game.StatStageNeutral,
		ActiveReflect:  false,
	}
	reflectEval := game.BattleMoveEvaluation{NativeMoveID: gsMoveReflect, PolicyPriority: 40}
	growlEval := game.BattleMoveEvaluation{NativeMoveID: gsMoveGrowl, PolicyPriority: 30}
	attack := game.BattleMoveEvaluation{NativeMoveID: 33, ExpectedScore: 350}

	if !p.PreferSetupMove(b, reflectEval, attack) {
		t.Fatal("want Reflect before the screen is up")
	}
	b.ActiveReflect = true
	if p.PreferSetupMove(b, reflectEval, attack) {
		t.Fatal("Reflect must stop once the screen is up")
	}
	if !p.PreferSetupMove(b, growlEval, attack) {
		t.Fatal("want Growl while enemy attack stage is neutral")
	}
	b.EnemyAttackMod = game.StatStageNeutral - 1
	if p.PreferSetupMove(b, growlEval, attack) {
		t.Fatal("Growl must stop after the enemy attack stage drops")
	}
	b.EnemyLevel = 14
	b.EnemyAttackMod = game.StatStageNeutral
	b.ActiveReflect = false
	if p.PreferSetupMove(b, reflectEval, attack) {
		t.Fatal("setup must not stall against sub-Scyther bugs")
	}
}
