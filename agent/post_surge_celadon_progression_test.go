package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func hasProgressObjective(objs []Objective, id ProgressID) bool {
	for _, o := range objs {
		if o.Kind == KindProgress && o.Progress == id {
			return true
		}
	}
	return false
}

func postSurgeObservation(mapID uint8) Observation {
	return Observation{
		Map:     mapID,
		Badges:  []string{state.BadgeBoulder.String(), state.BadgeCascade.String(), state.BadgeThunder.String()},
		Story:   ProgressState{{ID: redProgressHM01Acquired, Complete: true}},
		MapName: "VERMILION_CITY",
	}
}

func TestRedProgressionOffersRainbowBadgeStageAfterSurge(t *testing.T) {
	obs := postSurgeObservation(0x05)
	if !hasProgressObjective(redProgressionObjectives(obs), redProgressRainbowBadge) {
		t.Fatal("post-Surge observation did not offer the Rainbow Badge progression stage")
	}
}

func TestRedProgressionKeepsRainbowBadgeStageAvailableAwayFromVermilion(t *testing.T) {
	for _, mapID := range []uint8{0x03, 0x14, 0x15, 0x52, 0x04, 0x13, 0x12, 0x06, 0x85} {
		obs := postSurgeObservation(mapID)
		if !hasProgressObjective(redProgressionObjectives(obs), redProgressRainbowBadge) {
			t.Errorf("map %#04x lost the resumable Rainbow Badge progression stage", mapID)
		}
	}
}

func TestRedProgressionStopsRainbowBadgeStageAfterErika(t *testing.T) {
	obs := postSurgeObservation(0x06)
	obs.Story = append(obs.Story, ProgressFact{ID: redProgressRainbowBadge, Complete: true})
	if hasProgressObjective(redProgressionObjectives(obs), redProgressRainbowBadge) {
		t.Fatal("Rainbow Badge progression was re-offered after completion")
	}
}

func TestRocketHideoutWaitsForRainbowBadge(t *testing.T) {
	obs := postSurgeObservation(0x06)
	if hasProgressObjective(redProgressionObjectives(obs), redProgressSilphScopeAcquired) {
		t.Fatal("Rocket Hideout was offered before the post-Surge Rainbow Badge stage completed")
	}
	obs.Story = append(obs.Story, ProgressFact{ID: redProgressRainbowBadge, Complete: true})
	if !hasProgressObjective(redProgressionObjectives(obs), redProgressSilphScopeAcquired) {
		t.Fatal("Rocket Hideout was not offered from Celadon after the Rainbow Badge")
	}
}

func TestRainbowBadgeProgressIsProjectedFromRAM(t *testing.T) {
	var mem state.Mem
	mem[sym.ObtainedBadges] = 1 << uint8(state.BadgeRainbow)
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressRainbowBadge) {
		t.Fatal("Rainbow Badge bit was not projected into semantic progression state")
	}
	if !redProgressionKnown(redProgressRainbowBadge) {
		t.Fatal("Rainbow Badge semantic progression ID is not accepted by the Red adapter")
	}
}
