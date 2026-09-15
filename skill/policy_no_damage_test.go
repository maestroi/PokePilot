package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// When no usable move can reduce HP directly or through residual damage,
// choosing the first slot can burn dozens of no-op turns before Red finally
// gives the player STRUGGLE. Prefer the smallest remaining PP pool instead so
// an otherwise unwinnable status-only set converges on that legal fallback.
func TestStatAwareMoveStatusOnlyExhaustsShortestMoveFirst(t *testing.T) {
	growl := rom.Move{ID: 45, Effect: rom.AttackDown1Effect}
	tailWhip := rom.Move{ID: 39, Effect: rom.DefenseDown1Effect}
	p := StatAwareMove(fakeROM(t, growl, tailWhip))
	b := battleWith(state.StatStageNeutral, state.StatStageNeutral, 20, 20, growl.ID, tailWhip.ID)
	b.Moves[0].PP = 40
	b.Moves[1].PP = 3

	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d, want 1: status-only fallback should exhaust 3 PP before 40 PP", got)
	}
}
