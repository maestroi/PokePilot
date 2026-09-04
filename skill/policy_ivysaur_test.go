package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	moveVineWhip      uint8 = 22
	moveGrowl         uint8 = 45
	moveLeechSeed     uint8 = 73
	movePoisonPowder  uint8 = 77
	typeGrassIvysaur  uint8 = 0x16
	typePoisonIvysaur uint8 = 0x03
)

func ivysaurPolicy(t *testing.T) MovePolicy {
	t.Helper()
	return StatAwareMove(fakeROM(t,
		rom.Move{ID: moveVineWhip, Power: 35, Type: typeGrassIvysaur},
		rom.Move{ID: moveGrowl, Power: 0},
		rom.Move{ID: moveLeechSeed, Power: 0},
		rom.Move{ID: movePoisonPowder, Power: 0},
		tackle,
	))
}

// At level 20 the normal four-move learn prompt can replace Tackle while
// leaving Ivysaur with three status moves and Vine Whip. This is the shape
// observed in the post-Brock run: a damaging move still exists, so the
// battle policy must never fall through to Growl merely because Tackle is
// gone.
func TestStatAwareMoveIvysaurUsesVineWhipAfterTackleIsForgotten(t *testing.T) {
	p := ivysaurPolicy(t)
	b := battleWith(state.StatStageNeutral, state.StatStageNeutral, 55, 55,
		movePoisonPowder, moveGrowl, moveLeechSeed, moveVineWhip)
	b.ActiveType1, b.ActiveType2 = typeGrassIvysaur, typePoisonIvysaur

	if got := p(b); got != 3 {
		t.Fatalf("policy chose slot %d, want 3 (VINE WHIP): it is the only damaging move", got)
	}
}

// The live spectator report that prompted #50 also looked like TACKLE had
// reached 0 PP and GROWL (slot 2 in the menu) was being repeated. Exhausted
// TACKLE must disappear from Usable, but that does not make a status move a
// better attack while VINE WHIP still has PP.
func TestStatAwareMoveIvysaurUsesVineWhipWhenTackleHasNoPP(t *testing.T) {
	p := ivysaurPolicy(t)
	b := battleWith(state.StatStageNeutral, state.StatStageNeutral, 55, 55,
		tackle.ID, moveGrowl, moveLeechSeed, moveVineWhip)
	b.ActiveType1, b.ActiveType2 = typeGrassIvysaur, typePoisonIvysaur
	b.Moves[0].PP = 0

	if got := p(b); got != 3 {
		t.Fatalf("policy chose slot %d, want 3 (VINE WHIP): TACKLE has no PP and status moves must not replace live damage", got)
	}
}

// DISABLE must have the same fallback property. PR #23 made the disabled
// slot disappear from BattleState.Usable(); this pins the later-Ivysaur case
// where Tackle is temporarily unavailable while Vine Whip remains legal.
func TestStatAwareMoveIvysaurUsesVineWhipWhenTackleIsDisabled(t *testing.T) {
	p := ivysaurPolicy(t)
	b := battleWith(state.StatStageNeutral, state.StatStageNeutral, 55, 55,
		tackle.ID, moveGrowl, moveLeechSeed, moveVineWhip)
	b.ActiveType1, b.ActiveType2 = typeGrassIvysaur, typePoisonIvysaur
	b.DisabledMove = 1 // game slot 1 = Tackle

	if got := p(b); got != 3 {
		t.Fatalf("policy chose slot %d, want 3 (VINE WHIP): TACKLE is disabled and status moves must not replace damage", got)
	}
}
