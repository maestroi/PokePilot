package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestRunInitialObservationReturnsErrorInsteadOfPanicking(t *testing.T) {
	_, err := observeRunInitial(nil, []byte("not-a-supported-pokemon-rom"))
	if err == nil {
		t.Fatal("invalid ROM produced no initial observation error")
	}
	if !strings.Contains(err.Error(), "agent: Run: initial observation") {
		t.Fatalf("initial observation error = %q, want Run phase context", err)
	}
}

func TestTravelRecoveryQuarantineIgnoresIrrelevantMoneyButExpiresOnRouteChange(t *testing.T) {
	failed := Objective{Kind: KindGoTo, Place: "route 1"}
	other := Objective{Kind: KindGoTo, Place: "pallet town"}
	obs := Observation{
		Location:     "viridian city",
		X:            10,
		Y:            12,
		Controllable: true,
		Money:        100,
		FieldCapabilities: []FieldCapability{{
			Name: "cut", Usable: false,
		}},
	}
	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{Objective: failed, Outcome: OutcomeBlocked, Cause: "navigation_stalled", Final: obs})

	moneyOnly := obs
	moneyOnly.Money++
	got := policy.filter(moneyOnly, []Objective{failed, other})
	if len(got) != 1 || got[0].Key() != other.Key() {
		t.Fatalf("money-only change expired travel quarantine: %+v", got)
	}

	routeOpened := moneyOnly
	routeOpened.FieldCapabilities = []FieldCapability{{Name: "cut", Usable: true}}
	got = policy.filter(routeOpened, []Objective{failed, other})
	if len(got) != 2 {
		t.Fatalf("route capability change did not expire travel quarantine: %+v", got)
	}
}

func TestCombatRecoveryFingerprintIncludesLeadPP(t *testing.T) {
	obj := Objective{Kind: KindTrain, Level: 20}
	a := ObjectiveResult{
		Objective: obj,
		Outcome:   OutcomeBlocked,
		Cause:     "train_retreat",
		Final: Observation{
			Location:     "route 6",
			Controllable: true,
			Party:        []PartyMon{{Species: "ivysaur", Level: 18, HP: 40, MaxHP: 50}},
			LeadPP:       []uint8{0, 0},
		},
	}
	b := a
	b.Final.LeadPP = []uint8{12, 0}
	if recoverableFailureKey(obj, a) == recoverableFailureKey(obj, b) {
		t.Fatal("PP recovery did not change combat failure fingerprint")
	}
}

func TestRedHealPostconditionRequiresPPRecoveryWhenInitiallyExhausted(t *testing.T) {
	o := Objective{Kind: KindHeal}
	initial := Observation{
		Party:  []PartyMon{{Species: "ivysaur", HP: 50, MaxHP: 50}},
		LeadPP: []uint8{0, 0},
	}
	final := initial
	final.Controllable = true

	adapter := &redObjectiveAdapter{}
	err := adapter.VerifyPostcondition(o, initial, final, ObjectiveResult{})
	if !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("unchanged exhausted PP heal = %v, want postcondition failure", err)
	}

	final.LeadPP = []uint8{10, 0}
	if err := adapter.VerifyPostcondition(o, initial, final, ObjectiveResult{}); err != nil {
		t.Fatalf("restored PP heal rejected: %v", err)
	}
}

func TestGymRecoveryPreservesMultipleScopedRetries(t *testing.T) {
	known := NewKnowledge(nil)
	for i, place := range []string{"pewter gym", "cerulean gym"} {
		o := Objective{Kind: KindGym, Place: PlaceID(place)}
		known.Failures[gymLossFailureKey(place)] = Failure{
			Objective: o.String(),
			Times:     i + 1,
			Last:      "lost",
		}
	}

	known.clearGymLossFailures()
	ready := gymRetryPlaces(known)
	if len(ready) != 2 || !ready["pewter gym"] || !ready["cerulean gym"] {
		t.Fatalf("multi-gym recovery collapsed retry state: %+v", ready)
	}
	if place, ok := gymRetryPending(known); !ok || place != "cerulean gym" {
		t.Fatalf("deterministic retry selection = %q, %v; want cerulean gym", place, ok)
	}

	// A new loss at Cerulean must suppress only Cerulean. Pewter's independent
	// trained retry remains due instead of being masked by an unrelated loss.
	cerulean := Objective{Kind: KindGym, Place: "cerulean gym"}
	known.Failures[gymLossFailureKey("cerulean gym")] = Failure{Objective: cerulean.String(), Times: 3, Last: "lost again"}
	ready = gymRetryPlaces(known)
	if len(ready) != 1 || !ready["pewter gym"] {
		t.Fatalf("fresh Cerulean loss masked Pewter retry: %+v", ready)
	}

	out := filterTrainerLossBlocked([]Objective{
		{Kind: KindTrain, Level: 22},
		{Kind: KindGoTo, Place: "pewter gym"},
		{Kind: KindGoTo, Place: "cerulean gym"},
		{Kind: KindGym, Place: "pewter gym"},
		cerulean,
	}, known)
	for _, candidate := range out {
		if candidate.Kind == KindTrain {
			t.Fatal("training remained offered while Pewter retry was due")
		}
		if candidate.Kind == KindGym && candidate.Place == "cerulean gym" {
			t.Fatal("freshly lost Cerulean gym was immediately re-offered")
		}
		if candidate.Kind == KindGym && candidate.Place == "pewter gym" && !strings.Contains(candidate.Note, "retry due") {
			t.Fatalf("Pewter retry lost its scoped note: %+v", candidate)
		}
	}
}
