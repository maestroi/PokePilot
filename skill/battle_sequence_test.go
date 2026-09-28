package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

type sequenceTestStrategy struct {
	// defender type -> move id -> score
	scores map[uint16]map[uint16]int64
}

func (s *sequenceTestStrategy) EvaluateCombatMove(
	_ []byte,
	_, defender game.BattleCombatant,
	nativeMoveID uint16,
	currentPP uint8,
) (game.BattleMoveEvaluation, game.BattleMoveRole, error) {
	score := int64(0)
	if byMove := s.scores[defender.Type1]; byMove != nil {
		score = byMove[nativeMoveID]
	}
	return game.BattleMoveEvaluation{
		NativeMoveID:  nativeMoveID,
		CurrentPP:     currentPP,
		Accuracy:      255,
		ExpectedScore: score,
	}, game.BattleMoveRoleDirectDamage, nil
}

func (*sequenceTestStrategy) IncomingTypeRisk([]byte, game.BattleCombatant, game.BattleCombatant) int {
	return 10
}

func (*sequenceTestStrategy) PreferSetupMove(
	game.BattleState,
	game.BattleMoveEvaluation,
	game.BattleMoveEvaluation,
) bool {
	return false
}

func (*sequenceTestStrategy) IsFieldMove(uint16) bool { return false }

func sequenceTestMon(species, move uint16, hp uint16) game.BattlePartyMon {
	return game.BattlePartyMon{
		NativeSpeciesID: species,
		Level:           50,
		HP:              hp,
		MaxHP:           100,
		Attack:          100,
		Defense:         100,
		Speed:           100,
		Special:         100,
		SpecialAttack:   100,
		SpecialDefense:  100,
		Moves: [4]game.BattlePartyMove{
			{NativeMoveID: move, PP: 10},
		},
	}
}

func sequenceBattle(activeMove uint16, defenderType uint16) game.BattleState {
	return game.BattleState{
		Kind:          game.BattleTrainer,
		ActiveLevel:   50,
		ActiveHP:      100,
		ActiveMaxHP:   100,
		ActiveAttack:  100,
		ActiveDefense: 100,
		ActiveSpecial: 100,
		EnemyLevel:    50,
		EnemyHP:       100,
		EnemyMaxHP:    100,
		EnemyAttack:   100,
		EnemyDefense:  100,
		EnemySpecial:  100,
		EnemyType1:    uint8(defenderType),
		EnemyType2:    uint8(defenderType),
		Moves: [4]game.BattleMove{
			{ID: uint8(activeMove), PP: 10},
		},
	}
}

func TestSequenceSwitchPreservesUniqueFutureCounter(t *testing.T) {
	const (
		currentType uint16 = 10
		futureType  uint16 = 20
	)
	strategy := &sequenceTestStrategy{scores: map[uint16]map[uint16]int64{
		currentType: {1: 100, 2: 300, 3: 450},
		futureType:  {1: 100, 2: 100, 3: 1000},
	}}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			sequenceTestMon(1, 1, 100),
			sequenceTestMon(2, 2, 100),
			sequenceTestMon(3, 3, 100),
		},
	}
	context := BattleSequenceContext{
		FutureOpponents: []game.BattleCombatant{{Type1: futureType, Type2: futureType}},
	}

	decision := chooseTacticalSwitchWithStrategyContext(
		strategy,
		nil,
		resources,
		sequenceBattle(1, currentType),
		context,
	)
	if !decision.Switch || decision.Slot != 1 {
		t.Fatalf("decision = %+v, want slot 1 so stronger-current slot 2 is preserved for future coverage", decision)
	}
	if decision.Candidate.FutureValue != 0 {
		t.Fatalf("chosen expendable counter future value = %d, want 0", decision.Candidate.FutureValue)
	}

	values := futurePreservationValues(strategy, nil, resources.Party, context)
	if values[2] <= values[1] || values[2] < futureSacrificeProtectionGap {
		t.Fatalf("future values = %v, want slot 2 recognized as unique future counter", values)
	}
}

func TestSequenceSwitchStillUsesFutureSpecialistWhenCurrentNeedIsLarge(t *testing.T) {
	const (
		currentType uint16 = 10
		futureType  uint16 = 20
	)
	strategy := &sequenceTestStrategy{scores: map[uint16]map[uint16]int64{
		currentType: {1: 100, 2: 300, 3: 700},
		futureType:  {1: 100, 2: 100, 3: 1000},
	}}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			sequenceTestMon(1, 1, 100),
			sequenceTestMon(2, 2, 100),
			sequenceTestMon(3, 3, 100),
		},
	}
	context := BattleSequenceContext{
		FutureOpponents: []game.BattleCombatant{{Type1: futureType, Type2: futureType}},
	}

	decision := chooseTacticalSwitchWithStrategyContext(
		strategy,
		nil,
		resources,
		sequenceBattle(1, currentType),
		context,
	)
	if !decision.Switch || decision.Slot != 2 {
		t.Fatalf("decision = %+v, want future specialist slot 2 when its current advantage is decisive", decision)
	}
}

func TestSequenceLeadUsesSaferCurrentCounterAndPreservesLaterSpecialist(t *testing.T) {
	const (
		currentType uint16 = 10
		futureType  uint16 = 20
	)
	strategy := &sequenceTestStrategy{scores: map[uint16]map[uint16]int64{
		currentType: {1: 100, 2: 300, 3: 450},
		futureType:  {1: 100, 2: 100, 3: 1000},
	}}
	resources := game.BattleResourcesState{Party: []game.BattlePartyMon{
		sequenceTestMon(1, 1, 100),
		sequenceTestMon(2, 2, 100),
		sequenceTestMon(3, 3, 100),
	}}
	context := BattleSequenceContext{
		FutureOpponents: []game.BattleCombatant{{Type1: futureType, Type2: futureType}},
	}

	slot, eval := bestSequenceLeadSlotWithStrategy(
		strategy,
		nil,
		resources,
		[]game.BattleCombatant{{Type1: currentType, Type2: currentType}},
		context,
	)
	if slot != 1 {
		t.Fatalf("lead slot = %d (%+v), want safer current counter slot 1 while preserving slot 2", slot, eval)
	}
}

func TestSequenceSacrificeHealProtectsUniqueFutureCounter(t *testing.T) {
	const (
		currentType uint16 = 10
		futureType  uint16 = 20
	)
	strategy := &sequenceTestStrategy{scores: map[uint16]map[uint16]int64{
		currentType: {2: 500, 3: 100},
		futureType:  {2: 100, 3: 1000},
	}}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			sequenceTestMon(3, 3, 100),
			sequenceTestMon(2, 2, 10),
		},
		Bag: []game.InventoryItem{{NativeItemID: uint16(itemSuperPotion), Quantity: 1}},
	}
	context := BattleSequenceContext{
		FutureOpponents: []game.BattleCombatant{{Type1: futureType, Type2: futureType}},
	}

	if choice, ok := chooseSacrificialBenchHealWithStrategyContext(
		strategy,
		nil,
		resources,
		sequenceBattle(3, currentType),
		context,
	); ok {
		t.Fatalf("sacrifice heal = %+v, true; want future specialist active protected", choice)
	}
}

func TestSequenceSacrificeHealRespectsMinimumViableParty(t *testing.T) {
	const currentType uint16 = 10
	strategy := &sequenceTestStrategy{scores: map[uint16]map[uint16]int64{
		currentType: {1: 100, 2: 1000},
	}}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			sequenceTestMon(1, 1, 100),
			sequenceTestMon(2, 2, 10),
		},
		Bag: []game.InventoryItem{{NativeItemID: uint16(itemSuperPotion), Quantity: 1}},
	}
	context := BattleSequenceContext{MinimumViableParty: 2}

	if choice, ok := chooseSacrificialBenchHealWithStrategyContext(
		strategy,
		nil,
		resources,
		sequenceBattle(1, currentType),
		context,
	); ok {
		t.Fatalf("sacrifice heal = %+v, true; want no sacrifice at two-member viable-party floor", choice)
	}
}
