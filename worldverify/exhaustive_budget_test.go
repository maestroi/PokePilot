package worldverify

import "testing"

func TestCapabilityStatesStayExhaustiveThroughSixteenCapabilities(t *testing.T) {
	caps := make([]CapabilityID, 13)
	for i := range caps {
		caps[i] = CapabilityID(string(rune('a' + i)))
	}

	states, exhaustive := capabilityStates(caps, 16)
	if !exhaustive {
		t.Fatal("13 capabilities should remain exhaustive under the 16-capability budget")
	}
	if got, want := len(states), 1<<13; got != want {
		t.Fatalf("states=%d, want %d", got, want)
	}
}

func TestCapabilityStatesFallBackAboveSixteenCapabilities(t *testing.T) {
	caps := make([]CapabilityID, 17)
	for i := range caps {
		caps[i] = CapabilityID(string(rune('a' + i)))
	}

	states, exhaustive := capabilityStates(caps, 16)
	if exhaustive {
		t.Fatal("17 capabilities should use bounded exploration under the 16-capability budget")
	}
	if got, want := len(states), 36; got != want { // empty + full + 17 singles + 17 full-minus-one
		t.Fatalf("states=%d, want %d", got, want)
	}
}
