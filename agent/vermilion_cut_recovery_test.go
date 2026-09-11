package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func hasVermilionGymRecoveryObjective(objs []Objective) bool {
	for _, o := range objs {
		if o.Kind == KindGym && o.Place == "vermilion gym" {
			return true
		}
	}
	return false
}

func recoverableVermilionCutObservation() Observation {
	return Observation{
		Map:        0x05,
		MapName:    "VERMILION_CITY",
		PartyCount: 1,
		RouteBlockages: []RouteBlockage{{
			Destination: "vermilion gym",
			Missing:     []CapabilityID{"can_cut"},
		}},
		FieldCapabilities: []FieldCapability{{
			Name:       "cut",
			Badge:      state.BadgeCascade.String(),
			BadgeOwned: true,
			HMOwned:    true,
			Learned:    false,
			PartySlot:  -1,
			Usable:     false,
			Preparable: false,
		}},
	}
}

func TestRedProgressionSurfacesRecoverableSurgeCutGate(t *testing.T) {
	obs := recoverableVermilionCutObservation()
	if !hasVermilionGymRecoveryObjective(redProgressionObjectives(obs)) {
		t.Fatal("recoverable Vermilion can_cut blockage did not surface the Surge gym objective")
	}
}

func TestRedProgressionDoesNotPromiseSurgeRecoveryWithoutHM01(t *testing.T) {
	obs := recoverableVermilionCutObservation()
	obs.FieldCapabilities[0].HMOwned = false
	if hasVermilionGymRecoveryObjective(redProgressionObjectives(obs)) {
		t.Fatal("Surge recovery was offered without HM01")
	}
}

func TestRedProgressionDoesNotReofferSurgeAfterThunderBadge(t *testing.T) {
	obs := recoverableVermilionCutObservation()
	obs.Badges = []string{state.BadgeThunder.String()}
	if hasVermilionGymRecoveryObjective(redProgressionObjectives(obs)) {
		t.Fatal("Surge recovery was offered after the Thunder Badge")
	}
}
