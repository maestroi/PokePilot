package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestViridianGymUsesSharedForcedMovementAdapter(t *testing.T) {
	landing, ok := forcedLandingForMap(viridianGymMap, 19, 11)
	if !ok || landing.X != 19 || landing.Y != 2 {
		t.Fatalf("Viridian arrow (19,11) = %+v,%v; want (19,2)", landing, ok)
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
