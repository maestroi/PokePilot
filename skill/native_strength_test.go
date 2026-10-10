package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

func TestNativeStrengthBridgeSelectsOnlyVerifiedMovableBoulders(t *testing.T) {
	grid := nativeTestGrid(t, 5, 3, []bool{
		true, true, true, false, false,
		true, true, true, true, true,
		false, false, true, false, false,
	})
	blocked := map[[2]int]bool{{2, 1}: true}
	live := game.LiveTopologyState{LiveObjects: []game.LiveMapObject{
		{Slot: 4, X: 2, Y: 1, Pushable: true},
	}}
	push, err := planNativeStrengthBridge(grid, live, world.Point{X: 0, Y: 1}, blocked, [][2]int{{4, 1}})
	if err != nil {
		t.Fatalf("plan native Strength: %v", err)
	}
	if push.MovableID != 4 || push.Stand != (world.Point{X: 2, Y: 0}) ||
		push.To != (world.Point{X: 2, Y: 2}) {
		t.Fatalf("bridge push=%+v", push)
	}

	// A static sprite using the same art must remain immutable.
	live.LiveObjects[0].Pushable = false
	if _, err := planNativeStrengthBridge(grid, live, world.Point{X: 0, Y: 1}, blocked, [][2]int{{4, 1}}); !errors.Is(err, world.ErrNoPath) {
		t.Fatalf("static boulder was considered movable: %v", err)
	}
}
