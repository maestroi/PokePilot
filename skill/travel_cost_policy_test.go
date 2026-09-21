package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
)

func TestWithTravelCostPolicyIsScopedPerEmulator(t *testing.T) {
	a := &emu.Emu{}
	b := &emu.Emu{}

	restore := WithTravelCostPolicy(a, TravelCostFastest)
	if got := travelCostPolicyFor(a); got != TravelCostFastest {
		t.Fatalf("emulator A policy = %v, want fastest", got)
	}
	if got := travelCostPolicyFor(b); got != TravelCostConservative {
		t.Fatalf("emulator B policy = %v, want conservative", got)
	}
	restore()
	if got := travelCostPolicyFor(a); got != TravelCostConservative {
		t.Fatalf("restored emulator A policy = %v, want conservative", got)
	}
}
