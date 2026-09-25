package game

import "fmt"

// BattleMoveRole is the semantic role a generation strategy assigns to one
// move for reusable turn policy. Concrete profiles own native effect ids and
// mechanics; generic battle code only needs to know whether a move can make
// direct progress, residual progress, or is a bounded setup option.
type BattleMoveRole uint8

const (
	BattleMoveRoleOther BattleMoveRole = iota
	BattleMoveRoleDirectDamage
	BattleMoveRoleResidualDamage
	BattleMoveRoleSetup
)

// BattleCombatant is the portable stat/type view consumed by generation-owned
// combat scoring. Type ids remain native and intentionally wider than today's
// BattleState bytes so party strategy does not truncate a future profile.
//
// Special retains the Gen-I combined stat. SpecialAttack/SpecialDefense let a
// Gen-II strategy consume the split stats without changing reusable policy.
type BattleCombatant struct {
	Level   uint8
	HP      uint16
	MaxHP   uint16
	Attack  uint16
	Defense uint16
	Speed   uint16

	Special        uint16
	SpecialAttack  uint16
	SpecialDefense uint16

	Type1 uint16
	Type2 uint16
}

// BattleMoveEvaluation is a comparable, generation-neutral projection of one
// move score. ExpectedScore is deliberately opaque: only a strategy from the
// same profile may define its scale. CurrentPP and Accuracy are stable tie
// breakers used after exact score ties.
//
// MoveID is the retained byte-sized compatibility view used by existing Gen-I
// diagnostics/tests. NativeMoveID is authoritative for generic strategy and
// can represent wider ids without truncation.
type BattleMoveEvaluation struct {
	MoveID       uint8
	NativeMoveID uint16

	Physical      bool
	AttackStat    uint16
	DefenseStat   uint16
	NeutralDamage uint16
	Effectiveness int
	STAB          bool
	Accuracy      uint8
	CurrentPP     uint8
	MaxPP         uint8
	DamageRule    string

	ExpectedScore  int64
	PolicyPriority int
	DebugText      string
}

func (e BattleMoveEvaluation) String() string {
	if e.DebugText != "" {
		return e.DebugText
	}
	id := e.NativeMoveID
	if id == 0 {
		id = uint16(e.MoveID)
	}
	return fmt.Sprintf("move=%d score=%d pp=%d acc=%d", id, e.ExpectedScore, e.CurrentPP, e.Accuracy)
}

// BetterBattleMove provides the stable cross-policy ordering after a concrete
// generation has projected its mechanics into ExpectedScore.
func BetterBattleMove(candidate, incumbent BattleMoveEvaluation) bool {
	if candidate.ExpectedScore != incumbent.ExpectedScore {
		return candidate.ExpectedScore > incumbent.ExpectedScore
	}
	if candidate.CurrentPP != incumbent.CurrentPP {
		return candidate.CurrentPP > incumbent.CurrentPP
	}
	return candidate.Accuracy > incumbent.Accuracy
}

// BattleCombatStrategy is the optional generation-mechanics capability used by
// reusable move and switch policy. It deliberately owns ROM lookup, damage
// scoring, type-risk interpretation, setup semantics, and field-move identity.
// A Gen-II profile can therefore implement split-special/type mechanics without
// teaching skill.Battle or switch policy about a concrete generation.
type BattleCombatStrategy interface {
	EvaluateCombatMove(
		romData []byte,
		attacker, defender BattleCombatant,
		nativeMoveID uint16,
		currentPP uint8,
	) (BattleMoveEvaluation, BattleMoveRole, error)
	IncomingTypeRisk(romData []byte, enemy, candidate BattleCombatant) int
	PreferSetupMove(BattleState, BattleMoveEvaluation, BattleMoveEvaluation) bool
	IsFieldMove(nativeMoveID uint16) bool
}

// BattleCombatStrategyProfile is a game profile exposing generation mechanics
// to reusable combat policy.
type BattleCombatStrategyProfile interface {
	GameProfile
	BattleCombatStrategy
}
