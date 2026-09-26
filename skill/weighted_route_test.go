package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

func TestFirstStepNeedsSurfPortValidation(t *testing.T) {
	surf := gameruntime.Transition{
		ID:         "test:surf",
		Requires:   []gameruntime.CapabilityID{capCanSurf},
		PortBypass: true,
	}
	cut := gameruntime.Transition{
		ID:         "test:cut",
		Requires:   []gameruntime.CapabilityID{capCanCut},
		PortBypass: true,
	}

	if !firstStepNeedsSurfPortValidation([]world.RouteStep{{Transition: &surf}}) {
		t.Fatal("Surf PortBypass first hop must require concrete local-port validation")
	}
	if firstStepNeedsSurfPortValidation([]world.RouteStep{{Transition: &cut}}) {
		t.Fatal("Cut PortBypass must keep its live-topology bypass semantics")
	}
	surf.PortBypass = false
	if firstStepNeedsSurfPortValidation([]world.RouteStep{{Transition: &surf}}) {
		t.Fatal("ordinary Surf-tagged edge without PortBypass needs no extra validation")
	}
	if firstStepNeedsSurfPortValidation(nil) {
		t.Fatal("empty route needs no validation")
	}
}
