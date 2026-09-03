package combat

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// SpecialType is the Gen 1 physical/special split used by the ROM. Types
// below 0x14 use Attack/Defense; FIRE and every type after it use Special.
// This is the exact `cp SPECIAL` boundary in GetDamageVarsForPlayerAttack.
const SpecialType uint8 = 0x14

const stabTenths = 15

// Combatant is the part of one Pokémon needed to compare damage matchups.
// It is intentionally independent of party position or battle menus so the
// same evaluator can be reused by the in-battle move policy and #47's future
// switch/team policy.
type Combatant struct {
	Level   uint8
	HP      uint16
	MaxHP   uint16
	Attack  uint16
	Defense uint16
	Special uint16
	Type1   uint8
	Type2   uint8
}

// MoveEvaluation is a deterministic, comparable description of one damaging
// move. ExpectedScore is not presented as literal HP damage: it keeps the
// ROM's accuracy threshold as an integer multiplier so rankings do not lose
// precision to float rounding. A larger score is a better expected-damage
// choice for the same turn.
type MoveEvaluation struct {
	MoveID            uint8
	Physical          bool
	AttackStat        uint16
	DefenseStat       uint16
	NeutralDamage     uint16
	Effectiveness     int // tenths: 5=0.5x, 10=1x, 20=2x, 40=4x
	STAB              bool
	Accuracy          uint8
	CurrentPP         uint8
	MaxPP             uint8
	ExpectedScore     int64
}

// IsPhysicalType applies Red's type-based damage-class split. There are no
// per-move physical/special categories in Gen 1.
func IsPhysicalType(moveType uint8) bool { return moveType < SpecialType }

// PlayerMatchup converts the live battle state into the generic combatants
// used by EvaluateMove. The stats are already the current values Red itself
// consumes after stat changes.
func PlayerMatchup(b state.BattleState) (Combatant, Combatant) {
	return Combatant{
		Level: b.ActiveLevel, HP: b.ActiveHP, MaxHP: b.ActiveMaxHP,
		Attack: b.ActiveAttack, Defense: b.ActiveDefense, Special: b.ActiveSpecial,
		Type1: b.ActiveType1, Type2: b.ActiveType2,
	}, Combatant{
		Level: b.EnemyLevel, HP: b.EnemyHP, MaxHP: b.EnemyMaxHP,
		Attack: b.EnemyAttack, Defense: b.EnemyDefense, Special: b.EnemySpecial,
		Type1: b.EnemyType1, Type2: b.EnemyType2,
	}
}

// EvaluateBattleMove evaluates one live move slot from BattleState.
func EvaluateBattleMove(romData []byte, b state.BattleState, slot int) (MoveEvaluation, error) {
	if slot < 0 || slot >= len(b.Moves) {
		return MoveEvaluation{}, fmt.Errorf("combat: move slot %d out of range", slot)
	}
	bm := b.Moves[slot]
	if bm.ID == 0 {
		return MoveEvaluation{}, fmt.Errorf("combat: move slot %d is empty", slot)
	}
	mv, err := rom.LookupMove(romData, bm.ID)
	if err != nil {
		return MoveEvaluation{}, err
	}
	attacker, defender := PlayerMatchup(b)
	return EvaluateMove(romData, attacker, defender, mv, bm.PP)
}

// EvaluateMove applies the Gen 1 ordinary damage ordering inputs that matter
// before the shared random damage roll: level, power, the type-selected stat
// pair, STAB, type effectiveness, accuracy, and current PP.
//
// The base-damage arithmetic mirrors CalculateDamage closely enough to retain
// its integer ordering. If either selected stat exceeds one byte, Red divides
// both by four before the 8-bit calculation; doing the same matters for high
// level matchups. Reflect/Light Screen, critical hits, multi-hit count, fixed
// damage and other move-specific effects remain separate mechanics and are not
// invented here. A zero-power move therefore gets score zero and can be given
// bounded utility by the caller instead of being mislabelled as damage.
func EvaluateMove(romData []byte, attacker, defender Combatant, mv rom.Move, currentPP uint8) (MoveEvaluation, error) {
	e := MoveEvaluation{
		MoveID:    mv.ID,
		Physical:  IsPhysicalType(mv.Type),
		Accuracy:  mv.Accuracy,
		CurrentPP: currentPP,
		MaxPP:     mv.PP,
	}
	if mv.Power == 0 || currentPP == 0 {
		return e, nil
	}

	if e.Physical {
		e.AttackStat, e.DefenseStat = attacker.Attack, defender.Defense
	} else {
		e.AttackStat, e.DefenseStat = attacker.Special, defender.Special
	}
	atk, def := damageStats(e.AttackStat, e.DefenseStat)
	e.NeutralDamage = neutralDamage(attacker.Level, mv.Power, atk, def)

	eff, err := rom.TypeEffectiveness(romData, mv.Type, defender.Type1, defender.Type2)
	if err != nil {
		return MoveEvaluation{}, err
	}
	e.Effectiveness = eff
	e.STAB = mv.Type == attacker.Type1 || mv.Type == attacker.Type2
	stab := rom.NeutralEffect
	if e.STAB {
		stab = stabTenths
	}

	// Accuracy is the byte the ROM compares against its random roll. Keeping
	// it unnormalised preserves the real ordering including the 255 threshold
	// used by table entries written as 100 percent.
	e.ExpectedScore = int64(e.NeutralDamage) * int64(eff) * int64(stab) * int64(e.Accuracy)
	return e, nil
}

// BetterMove orders two damaging choices. Expected damage dominates. Exact
// ties prefer the move with more current PP, making PP a finite resource
// without allowing a weak move to beat a materially safer KO merely because
// it has a larger PP pool. Accuracy then breaks the remaining tie.
func BetterMove(candidate, incumbent MoveEvaluation) bool {
	if candidate.ExpectedScore != incumbent.ExpectedScore {
		return candidate.ExpectedScore > incumbent.ExpectedScore
	}
	if candidate.CurrentPP != incumbent.CurrentPP {
		return candidate.CurrentPP > incumbent.CurrentPP
	}
	return candidate.Accuracy > incumbent.Accuracy
}

// String is compact enough for ZBAT diagnostics and deliberately explains the
// inputs instead of only printing an opaque scalar.
func (e MoveEvaluation) String() string {
	class := "special"
	if e.Physical {
		class = "physical"
	}
	stab := "1.0x"
	if e.STAB {
		stab = "1.5x"
	}
	return fmt.Sprintf("move=%d class=%s stats=%d/%d neutral=%d eff=%0.1fx stab=%s acc=%d/255 pp=%d/%d score=%d",
		e.MoveID, class, e.AttackStat, e.DefenseStat, e.NeutralDamage,
		float64(e.Effectiveness)/10, stab, e.Accuracy, e.CurrentPP, e.MaxPP, e.ExpectedScore)
}

func damageStats(attack, defense uint16) (uint16, uint16) {
	// Hand-built ROM-free battle states from older tests do not carry the new
	// stat fields. Treat an absent pair as a neutral 1:1 ratio so those states
	// retain their old power/type ordering while live states use real stats.
	if attack == 0 && defense == 0 {
		return 1, 1
	}
	if attack == 0 {
		attack = 1
	}
	if defense == 0 {
		defense = 1
	}
	if attack > 0xff || defense > 0xff {
		attack /= 4
		defense /= 4
		if attack == 0 {
			attack = 1
		}
		if defense == 0 {
			defense = 1
		}
	}
	return attack, defense
}

func neutralDamage(level, power uint8, attack, defense uint16) uint16 {
	if level == 0 {
		level = 1
	}
	if defense == 0 {
		defense = 1
	}
	dmg := (int64(2*uint16(level))/5 + 2) * int64(power) * int64(attack)
	dmg /= int64(defense)
	dmg /= 50
	dmg += 2
	if dmg > 999 {
		dmg = 999
	}
	if dmg < 1 {
		dmg = 1
	}
	return uint16(dmg)
}
