package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

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

// run-2g0fgqbf2fqfn3mok4ad4yua94 (regression of #2514, triage 1b855928d45232a8):
// Agatha's room after a Lance defeat where the party already carries a counter
// that sits under the team's level target. The counter campaign then asks to
// TRAIN that counter, and the sealed room has no grass and no routable habitat,
// so the campaign cannot act. The old code treated a train-counter campaign as
// always actionable, which kept the fight locked and emptied the menu into
// "nothing is possible from here". A sealed room must still promote the owed
// fight to retry.
func TestOfferWithProgressionPromotesSealedTrainCounterLoss(t *testing.T) {
	fight := Objective{Kind: KindProgress, Progress: redProgressLeagueLanceDefeated}
	planner := fixedProgressionPlanner{fight}

	m := game.Matchup{
		Opponents: []game.ChallengeOpponent{{Species: "dragonite", Level: 70, Types: []game.TypeID{"dragon", "flying"}}},
		MaxLevel:  70,
		Preferred: []game.TypeID{"ice", "dragon", "rock"},
		Useless:   []game.TypeID{"fire", "water", "electric"},
	}
	obs := Observation{
		GameID:     testGameID,
		PartyCount: 3,
		// The Ice counter sits at L60, under the matchup's level target (70-3=67),
		// so the campaign asks to train it. Party readiness 60*4+55*2+50=400
		// stays below the owed target 420, so the loss still owes readiness.
		Party: []PartyMon{
			withMoves(PartyMon{Species: "nidoran", Level: 60, HP: 100, MaxHP: 100}, PartyMove{Move: Move{Power: 80, Type: "ice"}, PP: 20}),
			withMoves(PartyMon{Species: "pidgeot", Level: 55, HP: 110, MaxHP: 110}, PartyMove{Move: Move{Power: 60, Type: "normal"}, PP: 20}),
			withMoves(PartyMon{Species: "rhydon", Level: 50, HP: 120, MaxHP: 120}, PartyMove{Move: Move{Power: 85, Type: "ground"}, PP: 20}),
		},
		ChallengeProfiles: []CatalogChallengeProfile{{Objective: fight.Key(), Readiness: matchupReadinessProfile(m)}},
		Unroutable:        []string{"indigo plateau pokemon center", "celadon gym"},
		RouteBlockages: []RouteBlockage{{
			Destination: "celadon gym",
			Missing:     []CapabilityID{"can_leave_league"},
		}},
	}
	known := NewKnowledge(nil)
	known.Failures[combatLossFailureKey(fight)] = Failure{
		Objective:         fight.String(),
		Times:             2,
		ReadinessBaseline: 400,
		ReadinessTarget:   420,
	}

	// The scenario is only a regression if the counter campaign is open and asks
	// to train (not acquire): that is the branch the old code made unconditionally
	// actionable.
	need, ok := counterNeedFor(obs, known)
	if !ok || need.Action != ChallengeTrainCounter {
		t.Fatalf("test assumption wrong: counter need = %+v %v, want train_counter", need, ok)
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
		t.Fatalf("sealed train-counter combat-loss did not promote the owed fight; candidates=%v", offer.Candidates)
	}
	if _, ok := known.Failures[combatLossFailureKey(fight)]; ok {
		t.Fatal("sealed promote left the combat-loss marker in place")
	}
}
