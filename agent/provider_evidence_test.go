package agent

import (
	"os"
	"strings"
	"testing"
)

func TestProviderBlockRequirementsProjectsDeterministically(t *testing.T) {
	blocked := []ObjectiveBlockEvidence{
		{Family: ObjectiveFamilyTravel, Reason: "route_prerequisite", Place: "route 10", Requirement: "cut"},
		{Family: ObjectiveFamilyCollection, Reason: "missing_resource", Requirement: "pokeball"},
		{Family: ObjectiveFamilyTravel, Reason: "route_prerequisite", Place: "route 10", Requirement: "cut"},
	}

	got := providerBlockRequirements(blocked)
	if len(got) != 2 {
		t.Fatalf("requirements = %+v, want two deduplicated projections", got)
	}
	if got[0].Text != "collection blocked: missing_resource; requires pokeball" || got[0].Place != "" {
		t.Fatalf("first requirement = %+v", got[0])
	}
	if got[1].Text != "travel blocked: route_prerequisite; requires cut" || got[1].Place != "route 10" {
		t.Fatalf("second requirement = %+v", got[1])
	}
	if got[0].Times != 1 || got[1].Times != 1 {
		t.Fatalf("provider evidence should be current-round only: %+v", got)
	}
}

func TestProviderBlockRequirementsAreBounded(t *testing.T) {
	blocked := make([]ObjectiveBlockEvidence, 0, requirementCap+4)
	for i := 0; i < requirementCap+4; i++ {
		blocked = append(blocked, ObjectiveBlockEvidence{
			Family: ObjectiveFamilyTravel,
			Reason: "route_unroutable",
			Place:  PlaceID(string(rune('a' + i))),
		})
	}
	if got := providerBlockRequirements(blocked); len(got) != requirementCap {
		t.Fatalf("projected %d provider requirements, want cap %d", len(got), requirementCap)
	}
}

func TestProductionRunKeepsProviderEvidence(t *testing.T) {
	run, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(run)
	for _, want := range []string{
		"offerWithTMHMEvidence(m, romData, last, known)",
		"providerBlockRequirements(offer.Blocked)",
		"engine.failures.filter(last, offer.Candidates)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("production Run no longer wires provider evidence through %q", want)
		}
	}

	tmhm, err := os.ReadFile("tmhm.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tmhm), "OfferWithProgressionEvidence(obs, known") {
		t.Fatal("Red offering discarded portable provider evidence before enrichment")
	}
}
