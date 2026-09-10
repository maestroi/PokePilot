package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestOfferViridianGymProgressionAfterVolcanoBadge(t *testing.T) {
	obs := Observation{
		Map:        0x08,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressVolcanoBadge, Complete: true},
			{ID: redProgressEarthBadge, Complete: false},
		},
	}
	if got := redProgressionObjectives(obs); !offeredProgressID(got, redProgressEarthBadge) {
		t.Fatalf("Viridian Gym progression missing after Volcano Badge: %v", got)
	}
}

func TestViridianGymProgressionWaitsForVolcanoAndStopsAtEarth(t *testing.T) {
	before := Observation{Map: 0x08, PartyCount: 1}
	if got := redProgressionObjectives(before); offeredProgressID(got, redProgressEarthBadge) {
		t.Fatalf("Viridian Gym progression offered before Volcano Badge: %v", got)
	}

	after := before
	after.Story = ProgressState{
		{ID: redProgressVolcanoBadge, Complete: true},
		{ID: redProgressEarthBadge, Complete: true},
	}
	if got := redProgressionObjectives(after); offeredProgressID(got, redProgressEarthBadge) {
		t.Fatalf("Viridian Gym progression re-offered after Earth Badge: %v", got)
	}
}

func TestRedProgressionAcceptsEarthBadgeFact(t *testing.T) {
	if !redProgressionKnown(redProgressEarthBadge) {
		t.Fatal("earth_badge is not registered as executable Red progression")
	}
}

func TestRedProgressStateDerivesEarthBadgeFromRAM(t *testing.T) {
	var mem state.Mem
	mem[sym.ObtainedBadges] = 1 << uint8(state.BadgeEarth)
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressEarthBadge) {
		t.Fatal("Earth Badge bit did not project into earth_badge semantic progress")
	}
}
