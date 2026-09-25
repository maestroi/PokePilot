package game

// BattleKind classifies the current encounter without exposing a cartridge's
// native battle-mode byte.
type BattleKind uint8

const (
	BattleNone    BattleKind = 0
	BattleWild    BattleKind = 1
	BattleTrainer BattleKind = 2
)

// BattleMove is one live move slot. Native move ids remain byte-sized for the
// currently targeted generations; adapters own their semantic naming and ROM
// metadata.
type BattleMove struct {
	ID       uint8
	PP       uint8
	Disabled bool
}

// BattleState is the portable live combat state consumed by reusable battle
// execution and move policies. It intentionally contains only facts the game
// has already resolved in RAM; damage/mechanics interpretation remains with a
// generation strategy layer.
//
// ActiveSpecial/EnemySpecial retain the Gen-I combined special stat for
// compatibility. Gen-II profiles may also fill the split fields when strategy
// support reaches that generation.
type BattleState struct {
	Kind          BattleKind
	EnemySpecies  uint8
	EnemyHP       uint16
	EnemyMaxHP    uint16
	EnemyLevel    uint8
	ActiveSpecies uint8
	ActiveHP      uint16
	ActiveLevel   uint8
	ActiveMaxHP   uint16
	Moves         [4]BattleMove

	ActiveAttack  uint16
	ActiveDefense uint16
	ActiveSpecial uint16
	EnemyAttack   uint16
	EnemyDefense  uint16
	EnemySpecial  uint16

	ActiveSpecialAttack  uint16
	ActiveSpecialDefense uint16
	EnemySpecialAttack   uint16
	EnemySpecialDefense  uint16

	ActiveSpeed uint16
	EnemySpeed  uint16

	ActiveAttackMod  uint8
	ActiveDefenseMod uint8
	EnemyAttackMod   uint8
	EnemyDefenseMod  uint8

	EnemyType1  uint8
	EnemyType2  uint8
	ActiveType1 uint8
	ActiveType2 uint8
}

// StatStageNeutral is the Gen-I neutral stage representation. It remains here
// because existing deterministic Gen-I policies consume it through the
// portable state; other generations need not use these fields the same way.
const StatStageNeutral uint8 = 7

// OffenceStage reports the stored attack-vs-defense stage delta.
func (b BattleState) OffenceStage() int {
	return int(b.ActiveAttackMod) - int(b.EnemyDefenseMod)
}

// DefenceStage reports the stored incoming attack-vs-defense stage delta.
func (b BattleState) DefenceStage() int {
	return int(b.EnemyAttackMod) - int(b.ActiveDefenseMod)
}

// Usable returns selectable move slots in stable slot order.
func (b BattleState) Usable() []int {
	var out []int
	for i, mv := range b.Moves {
		if mv.ID != 0 && mv.PP > 0 && !mv.Disabled {
			out = append(out, i)
		}
	}
	return out
}

// BattleResult reports how a finished battle ended.
type BattleResult uint8

const (
	BattleWon BattleResult = iota
	BattleLost
	BattleDraw
)

// BattleStateDecoder hides game-specific battle RAM and result encodings.
type BattleStateDecoder interface {
	DecodeBattleState(MemoryReader) (BattleState, bool)
	DecodeBattleResult(MemoryReader) BattleResult
}

// BattleStateProfile is a game profile exposing portable live battle state.
type BattleStateProfile interface {
	GameProfile
	BattleStateDecoder
}
