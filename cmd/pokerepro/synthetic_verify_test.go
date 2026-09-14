package main

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

func TestUnderlyingObjectiveFromDiagnosticMatchesExactOfferedSentence(t *testing.T) {
	pickup := agent.Objective{Kind: agent.KindPickup, Item: agent.ItemID("potion"), X: 12, Y: 29}
	other := agent.Objective{Kind: agent.KindGoTo, Place: agent.PlaceID("pewter city")}
	diagnostic := "failure recovery budget was exhausted: agent: " + pickup.String() + ": skill: Pickup: battle failed"

	got, ok, err := underlyingObjectiveFromDiagnostic(diagnostic, []agent.Objective{other, pickup})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected nested objective match")
	}
	if got.String() != pickup.String() {
		t.Fatalf("objective=%q want=%q", got.String(), pickup.String())
	}
}

func TestUnderlyingObjectiveFromDiagnosticRejectsNearPrefix(t *testing.T) {
	plain := agent.Objective{Kind: agent.KindGoTo, Place: agent.PlaceID("route 3")}
	flee := plain
	flee.Flee = true
	diagnostic := "failure recovery budget was exhausted: agent: " + flee.String() + ": skill: Travel: blocked"

	got, ok, err := underlyingObjectiveFromDiagnostic(diagnostic, []agent.Objective{plain, flee})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.String() != flee.String() {
		t.Fatalf("got=%q ok=%v want=%q", got.String(), ok, flee.String())
	}
}

func TestUnderlyingObjectiveFromDiagnosticReturnsUnavailableWhenMenuChanged(t *testing.T) {
	offered := []agent.Objective{{Kind: agent.KindTrain, Level: 20}}
	_, ok, err := underlyingObjectiveFromDiagnostic("failure recovery budget was exhausted: agent: go to cerulean city: no route", offered)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("unexpected match")
	}
}

func TestSyntheticFailureBudgetNonWrapperStaysContractUnavailable(t *testing.T) {
	verdict, err := verifySyntheticFailureBudget(portableMaterialized{
		Manifest: farm.PortableReproManifest{
			Objective:  "make progress toward run goal",
			Diagnostic: "stagnation watchdog stopped the run",
		},
		Dir: t.TempDir(),
	}, "", portableReproVerdict{})
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Classification != verdictContractUnavailable {
		t.Fatalf("classification=%q", verdict.Classification)
	}
	if !strings.Contains(verdict.Diagnostic, "no structured failure-repro contract") {
		t.Fatalf("diagnostic=%q", verdict.Diagnostic)
	}
}
