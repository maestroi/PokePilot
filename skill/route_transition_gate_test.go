package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestEvaluateRedRouteGateUsesDeclaredRequirements(t *testing.T) {
	transition := gameruntime.Transition{
		ID:       "red:future_passive_gate",
		Gate:     true,
		Requires: []gameruntime.CapabilityID{capCanEnterSaffron},
	}
	mem := new(state.Mem)

	blockage, handled := evaluateRedRouteGate(nil, mem, transition)
	if !handled {
		t.Fatal("Gate=true transition was not handled as a passive gate")
	}
	if blockage == nil || len(blockage.Missing) != 1 || blockage.Missing[0] != capCanEnterSaffron {
		t.Fatalf("closed gate blockage = %+v, want missing %q", blockage, capCanEnterSaffron)
	}

	// BIT_GAVE_SAFFRON_GUARDS_DRINK is the durable fact behind
	// capCanEnterSaffron. The transition ID is deliberately unknown: Gate and
	// Requires, not an executor switch case, must be enough to execute it.
	mem[sym.StatusFlags1] |= 1 << 6
	blockage, handled = evaluateRedRouteGate(nil, mem, transition)
	if !handled || blockage != nil {
		t.Fatalf("open passive gate = handled %v blockage %+v, want handled with no blockage", handled, blockage)
	}

	transition.Gate = false
	if blockage, handled := evaluateRedRouteGate(nil, mem, transition); handled || blockage != nil {
		t.Fatalf("action transition was consumed by passive gate evaluator: handled %v blockage %+v", handled, blockage)
	}
}

func TestSaffronGuardTransitionIsGenericPassiveGate(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeWarp, From: route7GateMap, To: semanticRoute7Map}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok {
		t.Fatalf("Saffron edge %+v has no semantic transition", edge)
	}
	if transition.ID != "red:saffron_guard_drink" || !transition.Gate {
		t.Fatalf("Saffron transition = %+v, want passive red:saffron_guard_drink", transition)
	}
	if len(transition.Requires) != 1 || transition.Requires[0] != capCanEnterSaffron {
		t.Fatalf("Saffron requirements = %v, want %q", transition.Requires, capCanEnterSaffron)
	}
}
