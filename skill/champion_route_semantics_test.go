package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestLanceToChampionRequiresDefeatCapability(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  lanceRoomMap,
		To:    championsRoomMap,
		WarpX: lanceExitStand.X,
		WarpY: 0,
	}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok {
		t.Fatal("Lance -> Champion warp is missing semantic transition")
	}
	if transition.ID != "red:lance_to_champion" {
		t.Fatalf("transition id=%q, want red:lance_to_champion", transition.ID)
	}
	if transition.Gate || transition.PivotOnly || !transition.PortBypass {
		t.Fatalf("Lance -> Champion transition mode=%+v, want scripted port-owning action", transition)
	}
	if len(transition.Requires) != 1 || transition.Requires[0] != capCanPassLanceExit {
		t.Fatalf("requirements=%v, want [%s]", transition.Requires, capCanPassLanceExit)
	}

	mem := new(state.Mem)
	if caps := redRouteCapabilities(nil, mem); caps.Has(capCanPassLanceExit) {
		t.Fatalf("fresh state unexpectedly projects %q: %v", capCanPassLanceExit, caps)
	}
	bit := uint16(eventBeatLance)
	mem[sym.EventFlags+bit/8] |= byte(1 << (bit % 8))
	if caps := redRouteCapabilities(nil, mem); !caps.Has(capCanPassLanceExit) {
		t.Fatalf("EVENT_BEAT_LANCE did not project %q: %v", capCanPassLanceExit, caps)
	}
}
