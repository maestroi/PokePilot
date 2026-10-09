package agent

import "testing"

// run-1auv5rq62ou1i16n25pxcc2izv lost the Burned Tower rival 70 times: the
// adapter's progression objective was appended after the combat-loss gate
// ran, so an unchanged party was re-offered the same fight every round.
func TestOfferWithProgressionGatesCombatLostProgression(t *testing.T) {
	fight := Objective{Kind: KindProgress, Progress: "gs_burned_tower_cleared"}
	planner := fixedProgressionPlanner{fight}
	obs := Observation{GameID: testGameID, PartyCount: 1, Party: []PartyMon{{Level: 31, HP: 86, MaxHP: 86}}}
	known := NewKnowledge(nil)

	offered := func() bool {
		for _, o := range OfferWithProgressionEvidence(obs, known, planner).Candidates {
			if o.Key() == fight.Key() {
				return true
			}
		}
		return false
	}
	if !offered() {
		t.Fatal("test assumption wrong: progression not offered before any loss")
	}

	known.Failures[combatLossFailureKey(fight)] = Failure{Objective: fight.String(), Times: 1}
	if offered() {
		t.Fatal("combat-lost progression re-offered with an unchanged party")
	}

	known.promoteCombatLossesToRetry()
	if !offered() {
		t.Fatal("progression still withheld after its combat retry became due")
	}
}

// run-a4wn4o17ztx91zgnezctug9t3: Agatha's room after a Lance blackout. The
// League exit gate refuses every preparation journey, local grass is gone,
// and the combat-loss gate would otherwise empty the menu into
// "nothing is possible from here". A sealed topology must promote the owed
// fight to retry instead of stranding the planner.
func TestOfferWithProgressionPromotesCombatLossWhenSealed(t *testing.T) {
	fight := Objective{Kind: KindProgress, Progress: redProgressLeagueLanceDefeated}
	planner := fixedProgressionPlanner{fight}
	obs := Observation{
		GameID:     testGameID,
		PartyCount: 1,
		Party:      []PartyMon{{Level: 71, HP: 204, MaxHP: 231}},
		Unroutable: []string{"indigo plateau pokemon center", "celadon gym"},
		RouteBlockages: []RouteBlockage{{
			Destination: "celadon gym",
			Missing:     []CapabilityID{"can_leave_league"},
		}},
	}
	known := NewKnowledge(nil)
	known.Failures[combatLossFailureKey(fight)] = Failure{
		Objective:         fight.String(),
		Times:             2,
		ReadinessBaseline: 385,
		ReadinessTarget:   405,
	}

	offer := OfferWithProgressionEvidence(obs, known, planner)
	found := false
	for _, o := range offer.Candidates {
		if o.Key() == fight.Key() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("sealed combat-loss did not promote the owed fight; candidates=%v", offer.Candidates)
	}
	if _, ok := known.Failures[combatLossFailureKey(fight)]; ok {
		t.Fatal("sealed promote left the combat-loss marker in place")
	}
}
