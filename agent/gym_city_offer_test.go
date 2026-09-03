package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func hasOfferedKind(objs []Objective, kind Kind) bool {
	for _, o := range objs {
		if o.Kind == kind {
			return true
		}
	}
	return false
}

func TestOfferPewterCitySurfacesBrockUntilBoulderBadge(t *testing.T) {
	obs := Observation{Map: 0x02, MapName: "PEWTER_CITY", PartyCount: 1}
	known := NewKnowledge(nil)
	if !hasOfferedKind(Offer(obs, known), KindGym) {
		t.Fatal("Pewter City did not offer the Brock gym challenge before the Boulder Badge")
	}

	obs.Badges = []string{state.BadgeBoulder.String()}
	if hasOfferedKind(Offer(obs, known), KindGym) {
		t.Fatal("Pewter City still offered Brock after the Boulder Badge was observed")
	}
}
