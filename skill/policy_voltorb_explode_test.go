package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	moveScreech      uint8 = 103
	moveSelfdestruct uint8 = 120
	moveBide         uint8 = 117
)

// TestStatAwareMoveVoltorbPrefersTackleOverSelfdestruct pins the farm
// stagnation on run-1db3o6gfh1oep20c9ucky4khvw: a solo L22 Voltorb with
// TACKLE/SCREECH/SELFDESTRUCT/BIDE was choosing SELFDESTRUCT against Route 3
// wilds, blacking out after one battle, then combat-prep re-issued train
// forever. Explode moves must not outrank a survivable damaging attack.
func TestStatAwareMoveVoltorbPrefersTackleOverSelfdestruct(t *testing.T) {
	selfdestruct := rom.Move{
		ID: moveSelfdestruct, Effect: rom.ExplodeEffect,
		Power: 130, Type: typeNormal, Accuracy: 255, PP: 5,
	}
	screech := rom.Move{ID: moveScreech, Power: 0, Type: typeNormal, Accuracy: 216, PP: 40}
	bide := rom.Move{ID: moveBide, Power: 0, Type: typeNormal, Accuracy: 255, PP: 10}
	p := StatAwareMove(fakeROM(t, tackle, screech, selfdestruct, bide))

	b := battleWith(state.StatStageNeutral, state.StatStageNeutral, 58, 58,
		tackle.ID, moveScreech, moveSelfdestruct, moveBide)
	b.ActiveLevel = 22
	b.ActiveAttack, b.ActiveDefense, b.ActiveSpecial = 30, 38, 34
	b.EnemyLevel = 3
	b.EnemyHP, b.EnemyMaxHP = 15, 15
	b.EnemyDefense, b.EnemySpecial = 20, 20
	b.Moves[0].PP = 35
	b.Moves[2].PP = 5

	if got := p(b); got != 0 {
		t.Fatalf("policy chose slot %d, want 0 (TACKLE): SELFDESTRUCT must not suicide a solo grind lead", got)
	}
}
