package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

type fakeGen2CombatStrategy struct {
	scores       map[uint16]int64
	roles        map[uint16]game.BattleMoveRole
	priorities   map[uint16]int
	preferSetup  bool
	seenMoves    []uint16
	lastAttacker game.BattleCombatant
	lastDefender game.BattleCombatant
	wideTypeSeen bool
}

func (s *fakeGen2CombatStrategy) EvaluateCombatMove(
	_ []byte,
	attacker, defender game.BattleCombatant,
	nativeMoveID uint16,
	currentPP uint8,
) (game.BattleMoveEvaluation, game.BattleMoveRole, error) {
	s.seenMoves = append(s.seenMoves, nativeMoveID)
	s.lastAttacker, s.lastDefender = attacker, defender
	legacy := uint8(0)
	if nativeMoveID <= 0xff {
		legacy = uint8(nativeMoveID)
	}
	return game.BattleMoveEvaluation{
		MoveID:          legacy,
		NativeMoveID:    nativeMoveID,
		Accuracy:        255,
		CurrentPP:       currentPP,
		ExpectedScore:   s.scores[nativeMoveID],
		PolicyPriority:  s.priorities[nativeMoveID],
	}, s.roles[nativeMoveID], nil
}

func (s *fakeGen2CombatStrategy) IncomingTypeRisk(
	_ []byte,
	_, candidate game.BattleCombatant,
) int {
	if candidate.Type1 > 0xff || candidate.Type2 > 0xff {
		s.wideTypeSeen = true
	}
	return 10
}

func (s *fakeGen2CombatStrategy) PreferSetupMove(
	_ game.BattleState,
	_, _ game.BattleMoveEvaluation,
) bool {
	return s.preferSetup
}

func (*fakeGen2CombatStrategy) IsFieldMove(uint16) bool { return false }

func TestStatAwareMoveUsesInjectedGenerationStrategy(t *testing.T) {
	strategy := &fakeGen2CombatStrategy{
		scores: map[uint16]int64{1: 100, 2: 250},
		roles: map[uint16]game.BattleMoveRole{
			1: game.BattleMoveRoleDirectDamage,
			2: game.BattleMoveRoleDirectDamage,
		},
	}
	b := game.BattleState{
		ActiveLevel:          30,
		ActiveHP:             70,
		ActiveMaxHP:          80,
		ActiveAttack:         55,
		ActiveDefense:        60,
		ActiveSpecialAttack:  120,
		ActiveSpecialDefense: 90,
		ActiveSpeed:          100,
		EnemyLevel:           30,
		EnemyHP:              75,
		EnemyMaxHP:           75,
		EnemyAttack:          70,
		EnemyDefense:         65,
		EnemySpecialAttack:   95,
		EnemySpecialDefense:  50,
		EnemySpeed:           80,
		Moves: [4]game.BattleMove{
			{ID: 1, PP: 10},
			{ID: 2, PP: 10},
		},
	}

	if got := statAwareMoveWithStrategy(nil, strategy)(b); got != 1 {
		t.Fatalf("move slot = %d, want 1 from injected generation scorer", got)
	}
	if strategy.lastAttacker.SpecialAttack != 120 || strategy.lastDefender.SpecialDefense != 50 {
		t.Fatalf("split special stats not projected: attacker=%+v defender=%+v",
			strategy.lastAttacker, strategy.lastDefender)
	}
}

func TestStatAwareMoveDelegatesSetupChoiceToGenerationStrategy(t *testing.T) {
	strategy := &fakeGen2CombatStrategy{
		scores: map[uint16]int64{1: 100},
		roles: map[uint16]game.BattleMoveRole{
			1: game.BattleMoveRoleDirectDamage,
			2: game.BattleMoveRoleSetup,
		},
		preferSetup: true,
	}
	b := game.BattleState{
		ActiveHP: 20, ActiveMaxHP: 20,
		Moves: [4]game.BattleMove{{ID: 1, PP: 10}, {ID: 2, PP: 10}},
	}
	if got := statAwareMoveWithStrategy(nil, strategy)(b); got != 1 {
		t.Fatalf("move slot = %d, want setup slot 1 selected by generation strategy", got)
	}

	strategy.preferSetup = false
	if got := statAwareMoveWithStrategy(nil, strategy)(b); got != 0 {
		t.Fatalf("move slot = %d, want direct attack slot 0 when setup is declined", got)
	}
}

func TestSwitchPolicyPreservesWiderNativePartyMechanics(t *testing.T) {
	const wideMove uint16 = 0x123
	strategy := &fakeGen2CombatStrategy{
		scores: map[uint16]int64{1: 100, wideMove: 1000},
		roles: map[uint16]game.BattleMoveRole{
			1:        game.BattleMoveRoleDirectDamage,
			wideMove: game.BattleMoveRoleDirectDamage,
		},
	}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			{
				NativeSpeciesID: 152,
				Level: 20, HP: 60, MaxHP: 60,
				Type1: 1, Type2: 1,
				Attack: 60, Defense: 60, SpecialAttack: 60, SpecialDefense: 60,
				Moves: [4]game.BattlePartyMove{{NativeMoveID: 1, PP: 20}},
			},
			{
				NativeSpeciesID: 251,
				Level: 20, HP: 60, MaxHP: 60,
				Type1: 0x101, Type2: 0x102,
				Attack: 60, Defense: 60, SpecialAttack: 130, SpecialDefense: 110,
				Moves: [4]game.BattlePartyMove{{NativeMoveID: wideMove, PP: 20}},
			},
		},
	}
	b := game.BattleState{
		ActiveLevel: 20, ActiveHP: 60, ActiveMaxHP: 60,
		ActiveAttack: 60, ActiveDefense: 60,
		EnemyLevel: 20, EnemyHP: 60, EnemyMaxHP: 60,
		EnemyAttack: 60, EnemyDefense: 60,
		Moves: [4]game.BattleMove{{ID: 1, PP: 20}},
	}

	decision := chooseTacticalSwitchWithStrategy(strategy, nil, resources, b)
	if !decision.Switch || decision.Slot != 1 {
		t.Fatalf("decision = %+v, want switch to wider-native-id bench member", decision)
	}
	if decision.Candidate.BestMove.NativeMoveID != wideMove {
		t.Fatalf("candidate move = %#x, want %#x", decision.Candidate.BestMove.NativeMoveID, wideMove)
	}
	if !strategy.wideTypeSeen {
		t.Fatal("wider native party type ids were not passed to generation strategy")
	}
	seenWide := false
	for _, id := range strategy.seenMoves {
		if id == wideMove {
			seenWide = true
			break
		}
	}
	if !seenWide {
		t.Fatalf("generation strategy never received wider native move %#x; saw %#v", wideMove, strategy.seenMoves)
	}
}
