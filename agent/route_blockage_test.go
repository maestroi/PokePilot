package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

type fakeRouteReachability map[skill.Destination]error

func (f fakeRouteReachability) Reachability(d skill.Destination) error { return f[d] }

func blockedRoute(transition string, missing ...gameruntime.CapabilityID) error {
	return &world.RouteBlockedError{Blockages: []gameruntime.TransitionBlockage{{
		Transition: gameruntime.Transition{ID: transition}, Missing: missing,
	}}}
}

func TestCollectRouteAvailabilityPreservesSemanticBlockages(t *testing.T) {
	cinnabar, ok := skill.Place("vermilion city")
	if !ok {
		t.Fatal("missing vermilion city fixture")
	}
	vermilion, ok := skill.Place("vermilion gym")
	if !ok {
		t.Fatal("missing vermilion gym fixture")
	}
	route13, ok := skill.Place("route 13")
	if !ok {
		t.Fatal("missing route 13 fixture")
	}
	mtMoon, ok := skill.Place("mt moon 1f")
	if !ok {
		t.Fatal("missing mt moon 1f fixture")
	}

	planner := fakeRouteReachability{
		cinnabar:  blockedRoute("red:route21_surf", "can_surf"),
		vermilion: blockedRoute("red:vermilion_gym_cut", "can_cut"),
		route13:   blockedRoute("red:route12_snorlax", "can_clear_snorlax"),
		mtMoon:    world.ErrNoRoute,
	}
	got := collectRouteAvailability(planner, []string{"route 13", "mt moon 1f", "vermilion city", "vermilion gym"})
	if len(got.Unroutable) != 4 {
		t.Fatalf("unroutable = %v, want four unavailable destinations", got.Unroutable)
	}
	if len(got.Blockages) != 3 {
		t.Fatalf("blockages = %+v, want three semantic blockages and geometry kept separate", got.Blockages)
	}

	byPlace := map[PlaceID]RouteBlockage{}
	for _, blockage := range got.Blockages {
		byPlace[blockage.Destination] = blockage
	}
	surf := byPlace["vermilion city"]
	if len(surf.Missing) != 1 || surf.Missing[0] != "can_surf" || len(surf.Prerequisites) != 1 || surf.Prerequisites[0].FieldCapability != "surf" {
		t.Fatalf("surf blockage = %+v", surf)
	}
	cut := byPlace["vermilion gym"]
	if len(cut.Missing) != 1 || cut.Missing[0] != "can_cut" || cut.Prerequisites[0].FieldCapability != "cut" {
		t.Fatalf("cut blockage = %+v", cut)
	}
	snorlax := byPlace["route 13"]
	if len(snorlax.Missing) != 1 || snorlax.Missing[0] != "can_clear_snorlax" || snorlax.Prerequisites[0].Progress != redProgressPokeFluteAcquired {
		t.Fatalf("snorlax blockage = %+v", snorlax)
	}
	if _, geometryWasMisclassified := byPlace["mt moon 1f"]; geometryWasMisclassified {
		t.Fatal("plain ErrNoRoute was exposed as a missing-capability blockage")
	}
}

func TestRouteBlockageDisappearsWhenCapabilityBecomesReachable(t *testing.T) {
	cinnabar, _ := skill.Place("vermilion city")
	blocked := collectRouteAvailability(fakeRouteReachability{cinnabar: blockedRoute("red:route21_surf", "can_surf")}, []string{"vermilion city"})
	if len(blocked.Blockages) != 1 {
		t.Fatalf("before capability: %+v", blocked)
	}
	reachable := collectRouteAvailability(fakeRouteReachability{cinnabar: nil}, []string{"vermilion city"})
	if len(reachable.Blockages) != 0 || len(reachable.Unroutable) != 0 {
		t.Fatalf("fresh reachable snapshot retained stale blockage: %+v", reachable)
	}
}

func TestPlannerRouteBlockageKeepsUnknownPrerequisiteHonest(t *testing.T) {
	err := blockedRoute("portable:unknown_gate", "can_teleport")
	var blocked *world.RouteBlockedError
	if !errors.As(err, &blocked) {
		t.Fatal("test setup did not produce RouteBlockedError")
	}
	got := plannerRouteBlockage("somewhere", blocked)
	if len(got.Missing) != 1 || got.Missing[0] != "can_teleport" {
		t.Fatalf("missing = %v", got.Missing)
	}
	if len(got.Prerequisites) != 0 {
		t.Fatalf("unknown prerequisite fabricated preparation: %+v", got.Prerequisites)
	}
}

func TestRedRoutePrerequisiteLinksCoverStoryGates(t *testing.T) {
	for _, tc := range []struct {
		cap      CapabilityID
		progress ProgressID
		badge    string
	}{
		{cap: "can_leave_viridian_north", progress: redProgressPokedexAcquired},
		{cap: "can_leave_pewter_east", badge: state.BadgeBoulder.String()},
		{cap: "can_enter_saffron", progress: ProgressSaffronGateOpen},
	} {
		link, ok := redRoutePrerequisiteLink(tc.cap)
		if !ok {
			t.Fatalf("capability %q has no planner prerequisite link", tc.cap)
		}
		if link.Progress != tc.progress || link.Badge != tc.badge {
			t.Fatalf("capability %q link = %+v, want progress=%q badge=%q", tc.cap, link, tc.progress, tc.badge)
		}
	}
}

func TestRouteBlockagesArePlannerVisibleAndBounded(t *testing.T) {
	obs := Observation{RouteBlockages: []RouteBlockage{{
		Destination:   "cinnabar island",
		Transitions:   []string{"red:route21_surf"},
		Missing:       []CapabilityID{"can_surf"},
		Prerequisites: []RoutePrerequisiteLink{{Capability: "can_surf", FieldCapability: "surf"}},
	}}}
	encoded, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"RouteBlockages":[{"destination":"cinnabar island"`) || !strings.Contains(text, `"missing":["can_surf"]`) {
		t.Fatalf("planner JSON omitted structured route blockage: %s", text)
	}

	names := make([]string, 0, routeBlockageCap+5)
	planner := fakeRouteReachability{}
	for _, name := range skill.PlaceNames() {
		d, ok := skill.Place(name)
		if !ok {
			continue
		}
		names = append(names, name)
		planner[d] = blockedRoute("portable:gate", "can_surf")
		if len(names) == routeBlockageCap+5 {
			break
		}
	}
	got := collectRouteAvailability(planner, names)
	if len(got.Blockages) != len(names) {
		t.Fatalf("runtime blockages = %d, want all %d semantic blockers retained", len(got.Blockages), len(names))
	}

	encoded, err = json.Marshal(Observation{RouteBlockages: got.Blockages})
	if err != nil {
		t.Fatal(err)
	}
	var projected struct {
		RouteBlockages []RouteBlockage
	}
	if err := json.Unmarshal(encoded, &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected.RouteBlockages) != routeBlockageCap {
		t.Fatalf("planner JSON contains %d blockages, want cap %d", len(projected.RouteBlockages), routeBlockageCap)
	}
}

func TestOfferAlwaysWithholdsSemanticBlockedDestination(t *testing.T) {
	known := NewKnowledge(map[uint8][]uint8{0x05: {0x5c}, 0x5c: {0x05}})
	known.SawMap(0x05)
	known.SawMap(0x5c)
	obs := Observation{
		Map: 0x05, MapName: "VERMILION_CITY", PartyCount: 1,
		Unroutable:     []string{"vermilion gym"},
		RouteBlockages: []RouteBlockage{{Destination: "vermilion gym", Missing: []CapabilityID{"can_cut"}}},
	}
	if offersPlace(Offer(obs, known), "vermilion gym") {
		t.Fatal("semantic-blocked destination was offered as an immediate GoTo")
	}
}

func TestOfferWithholdsSemanticBlockedDestinationBeyondPlannerCap(t *testing.T) {
	names := make([]string, 0, routeBlockageCap+1)
	planner := fakeRouteReachability{}
	for _, name := range skill.PlaceNames() {
		d, ok := skill.Place(name)
		if !ok {
			continue
		}
		names = append(names, name)
		planner[d] = blockedRoute("portable:gate", "can_surf")
		if len(names) == routeBlockageCap+1 {
			break
		}
	}
	if len(names) <= routeBlockageCap {
		t.Fatalf("not enough place fixtures to exercise cap: %d", len(names))
	}

	got := collectRouteAvailability(planner, names)
	last := names[len(names)-1]
	destination, _ := skill.Place(last)
	known := NewKnowledge(map[uint8][]uint8{destination.Map: {destination.Map}})
	known.SawMap(destination.Map)
	obs := Observation{Map: destination.Map, MapName: "TEST", PartyCount: 1, Unroutable: got.Unroutable, RouteBlockages: got.Blockages}

	if offersPlace(Offer(obs, known), last) {
		t.Fatalf("semantic blocker beyond planner JSON cap leaked back into Offer: %q", last)
	}
}
