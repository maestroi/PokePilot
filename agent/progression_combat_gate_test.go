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
