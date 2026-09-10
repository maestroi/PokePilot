package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestViridianGymSpinnerTransitionsMatchDecomp(t *testing.T) {
	want := map[rocketPoint]rocketPoint{
		{19, 11}: {19, 2},
		{19, 1}:  {11, 1},
		{18, 2}:  {18, 11},
		{11, 2}:  {17, 2},
		{16, 10}: {16, 12},
		{4, 6}:   {4, 13},
		{5, 13}:  {13, 13},
		{4, 14}:  {13, 14},
		{0, 15}:  {0, 7},
		{1, 15}:  {1, 9},
		{13, 16}: {7, 16},
		{13, 17}: {1, 17},
	}
	if len(viridianGymSpins) != len(want) {
		t.Fatalf("spinner count = %d, want %d", len(viridianGymSpins), len(want))
	}
	for enter, landing := range want {
		if got, ok := viridianGymSpins[enter]; !ok || got != landing {
			t.Fatalf("spinner %v = %v,%v, want %v", enter, got, ok, landing)
		}
	}
}

func TestViridianSpinnerPlannerTreatsArrowAsForcedEdge(t *testing.T) {
	walkable := func(x, y int) bool { return x >= 0 && x < 20 && y >= 0 && y < 18 }
	actions, err := planRocketSpinner(20, 18, walkable, 19, 12, 19, 1, viridianGymSpins, nil)
	if err != nil {
		t.Fatalf("plan spinner: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("spinner planner returned no actions")
	}
	first := actions[0]
	if !first.Forced || first.Enter != (rocketPoint{19, 11}) || first.Landing != (rocketPoint{19, 2}) {
		t.Fatalf("first action = %+v, want forced (19,11)->(19,2)", first)
	}
}

func TestViridianGymReadyUsesStoryOpenEvent(t *testing.T) {
	var mem state.Mem
	if ViridianGymReady(&mem) {
		t.Fatal("Viridian Gym ready without story-open event")
	}
	setSkillTestEvent(&mem, state.Event(0x028))
	if !ViridianGymReady(&mem) {
		t.Fatal("Viridian Gym not ready after EVENT_VIRIDIAN_GYM_OPEN")
	}
}

func TestGiovanniGymRegistration(t *testing.T) {
	for _, mapID := range []uint8{viridianCityMap, viridianGymMap} {
		g, ok := GymAt(mapID)
		if !ok {
			t.Fatalf("GymAt(%#02x) missing Viridian challenge", mapID)
		}
		if g.Leader != "GIOVANNI" || g.Badge != state.BadgeEarth || g.LeaderX != 2 || g.LeaderY != 1 {
			t.Fatalf("Viridian Gym registration from %#02x = %+v", mapID, g)
		}
	}
	dest, ok := Place("viridian gym")
	if !ok {
		t.Fatal("viridian gym place missing")
	}
	if dest.Map != viridianGymMap || dest.X != 2 || dest.Y != 2 {
		t.Fatalf("viridian gym destination = %+v, want 2d(2,2)", dest)
	}
}
