package agent

import "testing"

func offeredProgressID(out []Objective, want ProgressID) bool {
	for _, o := range out {
		if o.Kind == KindProgress && o.Progress == want {
			return true
		}
	}
	return false
}

// twoBadgeEasternObservation is the deadlocked shape itself: HM01 in hand (so
// the run has been roaming the east), Cut unlocked, and Celadon already
// crossed — but the Thunder Badge still missing and Saffron's guardhouses shut.
func twoBadgeEasternObservation() Observation {
	return Observation{
		Map:        celadonCityMap,
		MapName:    "CELADON_CITY",
		PartyCount: 1,
		Story:      ProgressState{{ID: redProgressHM01Acquired, Complete: true}},
		FieldCapabilities: []FieldCapability{{
			Name:       "cut",
			BadgeOwned: true,
			HMOwned:    true,
		}},
	}
}

// TestOfferSaffronGateBeforeFuchsiaCompletion is the regression for the third
// gym deadlock (run-jxh8lk19wv6on, run-1biaubd9xooqm). The guard drink is an
// ordinary ¥200 vending purchase, so this objective must be offered as soon as
// the run can be east of Cerulean — not after the Soul Badge/Surf/Strength
// slice it used to require. Saffron is the only corridor between Celadon and
// Vermilion, so withholding this objective while the Thunder Badge objective
// demanded a return to Vermilion left the run repeating an impossible route:
//
//	skill: SurgeProgression: return to Vermilion: skill: GoTo: no route from
//	map 12 at (5,14) to map 05 at (11,4): transition "red:saffron_guard_drink"
//	is missing capabilities [can_enter_saffron]
func TestOfferSaffronGateBeforeFuchsiaCompletion(t *testing.T) {
	obs := twoBadgeEasternObservation()
	if obs.Story.Has(redProgressFuchsiaProgressionComplete) {
		t.Fatal("fixture must not carry the Fuchsia slice this test removes")
	}
	got := redProgressionObjectives(obs)
	if !offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("Saffron gate objective missing for a two-badge run east of Cerulean: %v", got)
	}
}

// TestOfferSaffronGateWaitsForHM01 keeps the objective behind the one fact
// that means "the run has been to Vermilion and is roaming the east": before
// HM01 the drink is not part of the critical path, and offering it early would
// hand the planner a cross-Kanto journey it cannot yet route.
func TestOfferSaffronGateWaitsForHM01(t *testing.T) {
	before := twoBadgeEasternObservation()
	before.Story = nil
	if got := redProgressionObjectives(before); offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("Saffron gate offered before HM01: %v", got)
	}

	after := twoBadgeEasternObservation()
	if got := redProgressionObjectives(after); !offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("Saffron gate not offered with HM01 in hand: %v", got)
	}
}

func TestOfferSaffronGateStopsWhenOpen(t *testing.T) {
	obs := twoBadgeEasternObservation()
	obs.Story = append(obs.Story, ProgressFact{ID: ProgressSaffronGateOpen, Complete: true})
	if got := redProgressionObjectives(obs); offeredProgressID(got, ProgressSaffronGateOpen) {
		t.Fatalf("already-open Saffron gate was re-offered: %v", got)
	}
}

func TestRedProgressionAcceptsSaffronGateFact(t *testing.T) {
	if !redProgressionKnown(ProgressSaffronGateOpen) {
		t.Fatal("ProgressSaffronGateOpen is not registered as executable Red progression")
	}
}

// TestLiveSaffronBlockageAlwaysOffersItsOwner is the shared invariant behind
// the third gym deadlock, stated as a consistency rule rather than a story
// order: a live route blockage that names red:saffron_guard_drink must come
// with the objective that clears it. The runtime only offers the guard-drink
// transaction when the gate is shut, so if the offer is also gated on some
// later fact, the blockage can never be cleared — exactly the shape two farm
// runs hit, where can_enter_saffron was reported as the missing capability
// while the action that satisfies it stayed unoffered and the run repeated:
//
//	return to Vermilion ... transition "red:saffron_guard_drink" from
//	"route 7 gate" to "route 7" is missing capabilities [can_enter_saffron]
func TestLiveSaffronBlockageAlwaysOffersItsOwner(t *testing.T) {
	obs := twoBadgeEasternObservation()
	obs.RouteBlockages = []RouteBlockage{{
		Destination: "saffron city",
		Missing:     []CapabilityID{"can_enter_saffron"},
		Transitions: []string{"red:saffron_guard_drink"},
		Prerequisites: []RoutePrerequisiteLink{{
			Capability: "can_enter_saffron",
			Progress:   ProgressSaffronGateOpen,
		}},
	}}

	got := redProgressionObjectives(obs)
	if offeredProgressID(got, ProgressSaffronGateOpen) {
		return
	}
	t.Fatalf("live can_enter_saffron blockage was reported with no offered objective that clears it: %v", got)
}
