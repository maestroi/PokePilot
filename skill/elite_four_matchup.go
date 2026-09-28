package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// Generation-I type ids are stable across Red/Blue/Yellow. These are matchup
// archetypes for the League sequence, not species special cases: the battle
// strategy still reads the loaded ROM's move/type chart and scores the actual
// player's moves.
const (
	leagueTypeNormal   uint16 = 0x00
	leagueTypeFighting uint16 = 0x01
	leagueTypeFlying   uint16 = 0x02
	leagueTypePoison   uint16 = 0x03
	leagueTypeGround   uint16 = 0x04
	leagueTypeRock     uint16 = 0x05
	leagueTypeGhost    uint16 = 0x08
	leagueTypeFire     uint16 = 0x14
	leagueTypeWater    uint16 = 0x15
	leagueTypeGrass    uint16 = 0x16
	leagueTypePsychic  uint16 = 0x18
	leagueTypeIce      uint16 = 0x19
	leagueTypeDragon   uint16 = 0x1a
)

func leagueMatchup(type1, type2 uint16) game.BattleCombatant {
	// Neutral representative stats keep the portable move scorer meaningful
	// without pretending to know exact DVs/stat-exp for an unreached opponent.
	// Type effectiveness, STAB, the player's real stats/moves/PP and the
	// generation's own mechanics remain the differentiating signals.
	return game.BattleCombatant{
		Level: 55,
		HP:    100, MaxHP: 100,
		Attack: 100, Defense: 100, Speed: 100,
		Special: 100, SpecialAttack: 100, SpecialDefense: 100,
		Type1: type1, Type2: type2,
	}
}

func leagueSequenceContext(stage leagueStageDescriptor) BattleSequenceContext {
	context := BattleSequenceContext{MinimumViableParty: 2}
	stageIndex := -1
	for i := range leagueBattleStages {
		if leagueBattleStages[i].ID == stage.ID {
			stageIndex = i
			break
		}
	}
	if stageIndex < 0 {
		return context
	}
	for i := stageIndex + 1; i < len(leagueBattleStages); i++ {
		context.FutureOpponents = append(context.FutureOpponents, leagueBattleStages[i].Matchups...)
	}
	return context
}

// prepareLeagueStageLead chooses a healthy lead before committing to a League
// member. This avoids paying a switch turn just to discover that the historical
// carry is a poor matchup. Auto-started battles (notably the Champion room)
// are already past the safe party-reorder boundary and are handled by the
// in-battle sequence-aware switch policy instead.
func prepareLeagueStageLead(
	m *emu.Emu,
	romData []byte,
	stage leagueStageDescriptor,
) error {
	if len(stage.Matchups) == 0 {
		return nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) != nil || !state.Controllable(&mem) {
		return nil
	}
	resources := gen1BattleResourcesFromMem(&mem)
	if len(resources.Party) < 2 {
		return nil
	}
	strategy, err := combatStrategyForROM(romData)
	if err != nil {
		return fmt.Errorf("League lead strategy: %w", err)
	}
	slot, eval := bestSequenceLeadSlotWithStrategy(
		strategy,
		romData,
		resources,
		stage.Matchups,
		leagueSequenceContext(stage),
	)
	if slot <= 0 {
		return nil
	}
	if zbatDebug {
		fmt.Printf("zbat league=LEAD stage=%s slot=%d eval={%s}\n", stage.Name, slot, eval.String())
	}
	if err := SetLead(m, slot); err != nil {
		return fmt.Errorf("prepare %s lead slot %d: %w", stage.Name, slot, err)
	}
	return nil
}
