package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestRedFieldMoveForCapability(t *testing.T) {
	tests := []struct {
		capability CapabilityID
		want       skill.FieldMove
	}{
		{"cut", skill.FieldCut},
		{"fly", skill.FieldFly},
		{"surf", skill.FieldSurf},
		{"strength", skill.FieldStrength},
		{"flash", skill.FieldFlash},
	}
	for _, tc := range tests {
		got, ok := redFieldMoveForCapability(tc.capability)
		if !ok || got != tc.want {
			t.Errorf("redFieldMoveForCapability(%q) = %v, %v; want %v, true", tc.capability, got, ok, tc.want)
		}
	}
	if _, ok := redFieldMoveForCapability("teleport"); ok {
		t.Fatal("unsupported field capability teleport resolved")
	}
}

func TestRedAdapterValidatesFieldCapabilityRepair(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	if err := adapter.Validate(Objective{
		Kind:            KindRepairFieldCapability,
		FieldCapability: "surf",
	}, Observation{}); err != nil {
		t.Fatalf("Surf field repair validation failed: %v", err)
	}
	if err := adapter.Validate(Objective{
		Kind:            KindRepairFieldCapability,
		FieldCapability: "teleport",
	}, Observation{}); err == nil {
		t.Fatal("unsupported Red field capability passed validation")
	}
}

func TestFieldCapabilityRepairSurvivesObjectiveKeyRoundTrip(t *testing.T) {
	want := Objective{Kind: KindRepairFieldCapability, FieldCapability: "surf"}
	if got := want.Key().Objective(); got != want {
		t.Fatalf("ObjectiveKey round trip = %+v, want %+v", got, want)
	}
}
