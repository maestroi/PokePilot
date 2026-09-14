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
		MapName: state.MapName(mapID),
	}
}

func TestRedProgressionOffersLavenderStageFirstAfterSurge(t *testing.T) {
	obs := postSurgeObservation(0x05)
	got := redProgressionObjectives(obs)
	if !hasProgressObjective(got, redProgressPostSurgeLavenderReached) {
		t.Fatal("post-Surge observation did not offer the Lavender checkpoint stage")
	}
	if hasProgressObjective(got, redProgressPostSurgeCeladonReady) || hasProgressObjective(got, redProgressRainbowBadge) {
		t.Fatalf("later post-Surge stages leaked before Lavender completion: %v", got)
	}
}

func TestRedProgressionAdvancesFromLavenderToCeladonRecovery(t *testing.T) {
	obs := postSurgeObservation(0x04)
	obs.Story = append(obs.Story, ProgressFact{ID: redProgressPostSurgeLavenderReached, Complete: true})
	got := redProgressionObjectives(obs)
	if !hasProgressObjective(got, redProgressPostSurgeCeladonReady) {
		t.Fatal("Lavender-complete observation did not offer the Celadon recovery stage")
	}
	if hasProgressObjective(got, redProgressPostSurgeLavenderReached) || hasProgressObjective(got, redProgressRainbowBadge) {
		t.Fatalf("wrong post-Surge stage set after Lavender: %v", got)
	}
}

func TestRedProgressionOffersErikaOnlyAfterCeladonReady(t *testing.T) {
	obs := postSurgeObservation(0x85)
	obs.Story = append(obs.Story,
		ProgressFact{ID: redProgressPostSurgeLavenderReached, Complete: true},
		ProgressFact{ID: redProgressPostSurgeCeladonReady, Complete: true},
	)
	got := redProgressionObjectives(obs)
	if !hasProgressObjective(got, redProgressRainbowBadge) {
		t.Fatal("Celadon-ready observation did not offer the Erika/Rainbow stage")
	}
	if hasProgressObjective(got, redProgressPostSurgeLavenderReached) || hasProgressObjective(got, redProgressPostSurgeCeladonReady) {
		t.Fatalf("completed post-Surge travel stages were re-offered: %v", got)
	}
}

func TestRedProgressionStopsPostSurgeStagesAfterErika(t *testing.T) {
	obs := postSurgeObservation(0x06)
	obs.Story = append(obs.Story, ProgressFact{ID: redProgressRainbowBadge, Complete: true})
	got := redProgressionObjectives(obs)
	for _, id := range []ProgressID{redProgressPostSurgeLavenderReached, redProgressPostSurgeCeladonReady, redProgressRainbowBadge} {
		if hasProgressObjective(got, id) {
			t.Fatalf("post-Surge progression stage %q was re-offered after Erika", id)
		}
	}
}

func TestRainbowStageDoesNotSuppressRocketHideout(t *testing.T) {
	obs := postSurgeObservation(0x06)
	obs.Story = append(obs.Story,
		ProgressFact{ID: redProgressPostSurgeLavenderReached, Complete: true},
		ProgressFact{ID: redProgressPostSurgeCeladonReady, Complete: true},
	)
	if !hasProgressObjective(redProgressionObjectives(obs), redProgressSilphScopeAcquired) {
		t.Fatal("adding the bounded Rainbow stage suppressed the independently available Rocket Hideout objective")
	}
}

func TestPostSurgeStageIDsAreAcceptedByRedAdapter(t *testing.T) {
	for _, id := range []ProgressID{
		redProgressPostSurgeLavenderReached,
		redProgressPostSurgeCeladonReady,
		redProgressRainbowBadge,
	} {
		if !redProgressionKnown(id) {
			t.Fatalf("post-Surge semantic progression ID %q is not accepted by the Red adapter", id)
		}
	}
}

func TestRainbowBadgeProgressIsProjectedFromRAM(t *testing.T) {
	var mem state.Mem
	mem[sym.ObtainedBadges] = 1 << uint8(state.BadgeRainbow)
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressRainbowBadge) {
		t.Fatal("Rainbow Badge bit was not projected into semantic progression state")
	}
}
