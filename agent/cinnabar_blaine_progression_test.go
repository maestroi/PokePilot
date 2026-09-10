package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestOfferBlaineProgressionAfterSecretKey(t *testing.T) {
	obs := Observation{
		Map:        0xd8,
		PartyCount: 1,
		Story: ProgressState{
			{ID: ProgressSecretKeyOwned, Complete: true},
			{ID: redProgressVolcanoBadge, Complete: false},
		},
	}
	if got := redProgressionObjectives(obs); !offeredProgressID(got, redProgressVolcanoBadge) {
		t.Fatalf("Blaine progression missing after Secret Key: %v", got)
	}
}

func TestBlaineProgressionWaitsForSecretKeyAndStopsAtBadge(t *testing.T) {
	before := Observation{Map: 0x08, PartyCount: 1}
	if got := redProgressionObjectives(before); offeredProgressID(got, redProgressVolcanoBadge) {
		t.Fatalf("Blaine progression offered without Secret Key: %v", got)
	}

	after := before
	after.Story = ProgressState{
		{ID: ProgressSecretKeyOwned, Complete: true},
		{ID: redProgressVolcanoBadge, Complete: true},
	}
	if got := redProgressionObjectives(after); offeredProgressID(got, redProgressVolcanoBadge) {
		t.Fatalf("Blaine progression re-offered after Volcano Badge: %v", got)
	}
}

func TestRedProgressionAcceptsVolcanoBadgeFact(t *testing.T) {
	if !redProgressionKnown(redProgressVolcanoBadge) {
		t.Fatal("volcano_badge is not registered as executable Red progression")
	}
}

func TestRedProgressStateDerivesVolcanoBadgeFromRAM(t *testing.T) {
	var mem state.Mem
	mem[sym.ObtainedBadges] = 1 << uint8(state.BadgeVolcano)
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressVolcanoBadge) {
		t.Fatal("Volcano Badge bit did not project into volcano_badge semantic progress")
	}
}
