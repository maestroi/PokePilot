package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
	"github.com/maestroi/pokepilot/world"
)

// TestHopAfterAWalkClearsTheLedge is the regression for the input that leaked
// from one step into the next.
//
// A step used to end the frame its coordinate landed, which is one or two
// frames BEFORE the overworld polls the joypad again. The direction was still
// held at that poll, so the game started a second step nobody asked for. On
// an ordinary tile the overrun is invisible. On a ledge it armed a scripted
// two-tile jump (pokered/engine/overworld/ledges.asm) whose two simulated
// inputs were then spent while the next StepOnce believed it was pressing the
// hop itself: the player advanced one tile and stopped ON the ledge tile,
// which is a tile no walk may stand on, and the walk failed with the hop half
// done. MEASURED on the Pewter City descent at (22,28) — wJoyIgnore was
// already 0xff and the simulated index already exhausted on the step's first
// frame.
//
// The shape that matters is a walk immediately followed by a hop, which is
// what a planned path over any ledge looks like.
func TestHopAfterAWalkClearsTheLedge(t *testing.T) {
	if testing.Short() {
		t.Skip("emulator journey; not part of the -short gate")
	}
	e := fixture.Load(t, "post_boulder")
	if err := skill.GoTo(e, e.ROM(), skill.Destination{Map: 0x02, X: 22, Y: 27}); err != nil {
		t.Fatalf("GoTo the tile above the Pewter ledge: %v", err)
	}
	if err := skill.WalkPath(e, []world.Step{world.StepDown, {DY: 2}}); err != nil {
		t.Fatalf("walk then hop: %v", err)
	}
	if x, y := e.Peek8(sym.XCoord), e.Peek8(sym.YCoord); x != 22 || y != 30 {
		t.Fatalf("landed at (%d,%d), want (22,30): the hop did not clear the ledge", x, y)
	}
}
