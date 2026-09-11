package agent

import (
	"fmt"
	"reflect"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

func TestFailureCauseUsesTypedSentinelNotProse(t *testing.T) {
	cause, ctx := failureCauseFor(fmt.Errorf("arbitrary wrapper wording: %w", skill.ErrMenuStuck))
	if cause != "menu_stuck" || len(ctx) != 0 {
		t.Fatalf("cause=%q context=%v, want menu_stuck", cause, ctx)
	}
}

func TestFailureCauseGatedPathUsesTypedIdentity(t *testing.T) {
	cause, ctx := failureCauseFor(fmt.Errorf("agent: go to route 9: %w", &skill.ErrRouteGateClosed{Text: "Oh wait there, the road's closed."}))
	if cause != "route_gate_closed" || len(ctx) != 0 {
		t.Fatalf("cause=%q context=%v, want route_gate_closed", cause, ctx)
	}
	cause, ctx = failureCauseFor(fmt.Errorf("%w: CUT requires the Cascade Badge", skill.ErrFieldMovePrerequisite))
	if cause != "field_move_prerequisite_missing" || len(ctx) != 0 {
		t.Fatalf("cause=%q context=%v, want field_move_prerequisite_missing", cause, ctx)
	}
}

func TestFailureCauseCatchBlockedOnMissingBall(t *testing.T) {
	err := fmt.Errorf("%w: compatible wild species %#02x exists on map %#04x but no POKE BALL is available",
		skill.ErrFieldRosterNoBalls, 0xb9, 0x17)
	cause, ctx := failureCauseFor(err)
	if cause != "no_pokeball" || len(ctx) != 0 {
		t.Fatalf("cause=%q context=%v, want no_pokeball", cause, ctx)
	}
}

func TestFailureCauseCarriesSemanticRoutePrerequisites(t *testing.T) {
	err := &world.RouteBlockedError{Blockages: []gameruntime.TransitionBlockage{
		{Missing: []gameruntime.CapabilityID{"can_surf", "can_clear_snorlax"}},
		{Missing: []gameruntime.CapabilityID{"can_surf"}},
	}}
	cause, ctx := failureCauseFor(err)
	if cause != "route_prerequisite_missing" {
		t.Fatalf("cause=%q, want route_prerequisite_missing", cause)
	}
	want := []string{"can_clear_snorlax", "can_surf"}
	if !reflect.DeepEqual(ctx, want) {
		t.Fatalf("context=%v, want %v", ctx, want)
	}
}

func TestFailureStateDoesNotCarryRawRedMapIdentity(t *testing.T) {
	base := Observation{
		Map:          0x17,
		Location:     "route_12",
		X:            10,
		Y:            61,
		Controllable: true,
		Party:        []PartyMon{{Species: SpeciesID("pikachu"), Level: 24, HP: 40, MaxHP: 60}},
		Bag:          []Item{{Name: "poke flute", Quantity: 1}},
	}
	other := base
	other.Map = 0xfe
	if got, want := FailureStateFor(base), FailureStateFor(other); !reflect.DeepEqual(got, want) {
		t.Fatalf("raw map byte leaked into semantic failure state:\n%+v\n%+v", got, want)
	}
}
