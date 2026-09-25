package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func combatStrategyForROM(romData []byte) (game.BattleCombatStrategy, error) {
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return nil, fmt.Errorf("skill: combat strategy: detect profile: %w", err)
	}
	strategy, ok := profile.(game.BattleCombatStrategy)
	if !ok {
		return nil, fmt.Errorf(
			"skill: combat strategy: profile %s@%s does not expose generation combat mechanics",
			profile.ID(), profile.Revision(),
		)
	}
	return strategy, nil
}

func battleCombatants(b game.BattleState) (game.BattleCombatant, game.BattleCombatant) {
	attacker := game.BattleCombatant{
		Level: b.ActiveLevel, HP: b.ActiveHP, MaxHP: b.ActiveMaxHP,
		Attack: b.ActiveAttack, Defense: b.ActiveDefense, Speed: b.ActiveSpeed,
		Special: b.ActiveSpecial,
		SpecialAttack: b.ActiveSpecialAttack, SpecialDefense: b.ActiveSpecialDefense,
		Type1: uint16(b.ActiveType1), Type2: uint16(b.ActiveType2),
	}
	defender := game.BattleCombatant{
		Level: b.EnemyLevel, HP: b.EnemyHP, MaxHP: b.EnemyMaxHP,
		Attack: b.EnemyAttack, Defense: b.EnemyDefense, Speed: b.EnemySpeed,
		Special: b.EnemySpecial,
		SpecialAttack: b.EnemySpecialAttack, SpecialDefense: b.EnemySpecialDefense,
		Type1: uint16(b.EnemyType1), Type2: uint16(b.EnemyType2),
	}
	return attacker, defender
}

func battlePartyCombatant(mon game.BattlePartyMon) game.BattleCombatant {
	return game.BattleCombatant{
		Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP,
		Attack: mon.Attack, Defense: mon.Defense, Speed: mon.Speed,
		Special: mon.Special,
		SpecialAttack: mon.SpecialAttack, SpecialDefense: mon.SpecialDefense,
		Type1: mon.Type1, Type2: mon.Type2,
	}
}
