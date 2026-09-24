package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

type recordingBattleTurns struct {
	states   []game.BattleDecisionState
	executed []game.BattleAction
}

func (r *recordingBattleTurns) ObserveBattleTurn(s game.BattleDecisionState, executed game.BattleAction) {
	r.states = append(r.states, s)
	r.executed = append(r.executed, executed)
}

type battleTurnPlanner struct {
	blockingPlannerForBattleTurns
	recordingBattleTurns
}

type blockingPlannerForBattleTurns struct{}

func (blockingPlannerForBattleTurns) Next(Observation, []Objective) (Objective, error) {
	return Objective{}, nil
}

func TestGen1MoveObserverReportsPortableMoveTurn(t *testing.T) {
	if gen1MoveObserver(nil, nil) != nil {
		t.Fatal("no observer must install no skill observer")
	}
	rec := &recordingBattleTurns{}
	observe := gen1MoveObserver(nil, rec)
	b := state.BattleState{
		Kind:          state.BattleWild,
		ActiveSpecies: 0xB1, ActiveLevel: 14, ActiveHP: 38, ActiveMaxHP: 40,
		EnemySpecies: 0xA9, EnemyLevel: 12, EnemyHP: 33, EnemyMaxHP: 33,
	}
	b.Moves[0] = state.Move{ID: 0x21, PP: 30}
	b.Moves[1] = state.Move{ID: 0x37, PP: 25}
	observe(b, 1)

	if len(rec.states) != 1 {
		t.Fatalf("observed %d turns, want 1", len(rec.states))
	}
	if got := rec.executed[0]; got != (game.BattleAction{Kind: game.BattleActionMove, Slot: 1}) {
		t.Fatalf("executed = %+v, want move:1", got)
	}
	if _, err := rec.states[0].Legal(rec.executed[0].ID()); err != nil {
		t.Fatalf("executed action is not legal in the reported state: %v", err)
	}
}

func TestBindBattleTurnObserverAttachesPlannerToAdapter(t *testing.T) {
	adapter := newRedObjectiveAdapter(nil, nil)
	bindBattleTurnObserver(adapter, blockingPlannerForBattleTurns{})
	if adapter.battleTurns != nil {
		t.Fatal("planner without the seam must not attach an observer")
	}
	p := &battleTurnPlanner{}
	bindBattleTurnObserver(adapter, p)
	if adapter.battleTurns != BattleTurnObserver(p) {
		t.Fatal("observer planner was not attached to the adapter")
	}
}
