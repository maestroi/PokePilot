package agent

import (
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

// run-d6dokr184ky81 (88/139): post-Champion, every trainer beaten, ¥192, only
// a MASTER BALL and nothing sellable that covers five balls. The League
// rematch is the only way to raise money again.
func TestIncomeObjectivesOfferLeagueRematchWhenSalesCannotFund(t *testing.T) {
	obs := Observation{
		Money:        192,
		RestockStock: []string{"pokeball"},
		Dex:          DexCatalog{Targets: []DexEntry{{Species: "rattata"}}},
		Bag:          []Item{{Name: "master ball", Quantity: 1}, {Name: "guard spec", Quantity: 1}},
		Story:        ProgressState{{ID: ProgressMainStoryComplete, Complete: true}},
	}
	sources := redObjectiveCatalog(obs).IncomeSources
	got, blocked := incomeObjectives(obs, nil, sources)
	if len(got) != 1 || got[0].Kind != KindEarnMoney || got[0].Intent != redIncomeLeagueRematch || got[0].Validate() != nil || len(blocked) != 0 {
		t.Fatalf("income offers = %+v blocked = %+v, want one valid League rematch", got, blocked)
	}
	if err := newRedObjectiveAdapter(nil, nil).Validate(got[0], obs); err != nil {
		t.Fatalf("Red adapter rejected its own income source: %v", err)
	}

	// A lost rematch is withheld until readiness changes, like any challenge.
	known := NewKnowledge(nil)
	known.Failed(got[0], fmt.Errorf("lost: %w", skill.ErrTrainerBlackedOut))
	if got, blocked := incomeObjectives(obs, known, sources); len(got) != 0 || len(blocked) != 1 {
		t.Fatalf("after a loss: offers = %+v blocked = %+v, want the rematch withheld", got, blocked)
	}

	// A covering sale is cheaper and safer than a rematch.
	obs.Bag = append(obs.Bag, Item{Name: "nugget", Quantity: 1})
	if got, _ := incomeObjectives(obs, nil, sources); len(got) != 0 {
		t.Fatalf("rematch offered although a NUGGET sale covers the shortfall: %+v", got)
	}
	// Funded runs and pre-Hall-of-Fame saves get no rematch.
	obs.Bag = append(obs.Bag[:2], Item{Name: "pokeball", Quantity: minimumCaptureStock})
	if got, _ := incomeObjectives(obs, nil, sources); len(got) != 0 {
		t.Fatalf("rematch offered to a stocked run: %+v", got)
	}
	if sources := redObjectiveCatalog(Observation{}).IncomeSources; len(sources) != 0 {
		t.Fatalf("pre-Hall-of-Fame catalog declares income %+v", sources)
	}
}

func TestEarnMoneyPostconditionRequiresMoreMoney(t *testing.T) {
	earn := Objective{Kind: KindEarnMoney, Intent: redIncomeLeagueRematch}
	if out, err := verifyObjectivePostcondition(earn, Observation{Money: 192}, Observation{Money: 9000, Controllable: true}, ObjectiveResult{}); out != OutcomeCompleted || err != nil {
		t.Fatalf("money rose: outcome %q err %v", out, err)
	}
	if out, _ := verifyObjectivePostcondition(earn, Observation{Money: 192}, Observation{Money: 96, Controllable: true}, ObjectiveResult{}); out != OutcomePostconditionFailed {
		t.Fatalf("money fell: outcome %q, want postcondition_failed", out)
	}
}
