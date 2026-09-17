package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestAuditedRouteCapabilitiesRecoverSnorlaxCapabilityFromCompletedEncounter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event state.Event
	}{
		{name: "route 12", event: eventBeatRoute12Snorlax},
		{name: "route 16", event: eventBeatRoute16Snorlax},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mem state.Mem
			caps := gameruntime.NewCapabilitySet()
			addAuditedRedRouteCapabilities(&mem, caps, redWram())
			if caps.Has(capCanClearSnorlax) {
				t.Fatal("fresh state unexpectedly has Snorlax-clear capability")
			}

			addr := sym.EventFlags + uint16(tc.event)/8
			mem[addr] |= byte(1 << (uint16(tc.event) % 8))

			caps = gameruntime.NewCapabilitySet()
			addAuditedRedRouteCapabilities(&mem, caps, redWram())
			if !caps.Has(capCanClearSnorlax) {
				t.Fatalf("completed %s Snorlax event did not restore can_clear_snorlax", tc.name)
			}
			if caps.Has(capCanRideCyclingRoad) {
				t.Fatal("Snorlax completion must not grant Cycling Road/Bicycle capability")
			}
		})
	}
}
