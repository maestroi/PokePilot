package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
)

func TestDirectedHopInput(t *testing.T) {
	for _, tc := range []struct {
		step   world.Step
		button emu.Button
	}{{world.Step{DX: 2}, emu.Right}, {world.Step{DX: -2}, emu.Left}, {world.Step{DY: 2}, emu.Down}} {
		got, ok := buttonFor(tc.step)
		if !ok || got != tc.button {
			t.Errorf("hop %+v: button=%v ok=%v", tc.step, got, ok)
		}
	}
	if _, ok := buttonFor(world.Step{DX: 2, DY: 2}); ok {
		t.Fatal("diagonal hop accepted")
	}
}
