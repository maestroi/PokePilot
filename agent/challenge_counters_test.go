package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

// counterTestObservation: a Pikachu-style party (Electric only) that lost to
// a Ground team, with synthetic ROM facts so the policy stays ROM-free.
func counterTestObservation(challenge Objective) Observation {
	m := game.Matchup{
		Opponents: []game.ChallengeOpponent{{Species: "rhydon", Level: 20, Types: []game.TypeID{"ground", "rock"}}},
		MaxLevel:  20,
		Preferred: []game.TypeID{"water", "grass"},
		Useless:   []game.TypeID{"electric"},
	}
	return Observation{
		PartyCount: 2,
		Party: []PartyMon{
			withMoves(PartyMon{Species: "pikachu", Level: 30, HP: 60, MaxHP: 60}, PartyMove{Move: Move{Power: 40, Type: "electric"}, PP: 30}),
			withMoves(PartyMon{Species: "rattata", Level: 8, HP: 20, MaxHP: 20}, PartyMove{Move: Move{Power: 35, Type: "normal"}, PP: 30}),
		},
		ChallengeProfiles: []CatalogChallengeProfile{{Objective: challenge.Key(), Readiness: matchupReadinessProfile(m)}},
		Learnsets: map[SpeciesID][]LearnableMove{
			"squirtle": {{Level: 1, Type: "normal", Power: 35}, {Level: 8, Type: "water", Power: 20}},
			"oddish":   {{Level: 15, Type: "grass", Power: 20}},
			"spearow":  {{Level: 1, Type: "flying", Power: 35}},
		},
	}
}

var errTestLoss = errors.New("lost the test challenge")

func lostTo(challenge Objective, obs Observation) *Knowledge {
	known := NewKnowledge(nil)
	known.FailedResult(ObjectiveResult{
		Objective: challenge, Final: obs,
		Battle: &BattleEvidence{Result: "lost"},
	}, errTestLoss)
	return known
}

func TestCounterCampaignCatchesTheEarliestCounter(t *testing.T) {
	challenge := Objective{Kind: KindGym, Place: "test gym"}
	obs := counterTestObservation(challenge)
	known := lostTo(challenge, obs)
	if f := known.Failures[combatLossFailureKey(challenge)]; !f.CounterGap {
		t.Fatalf("loss without a counter not stamped: %+v", f)
	}
	offered := []Objective{
		{Kind: KindCatch, Species: "spearow", Place: "route 1"},
		{Kind: KindCatch, Species: "oddish", Place: "route 24"},
		{Kind: KindCatch, Species: "squirtle", Place: "route 25"},
		{Kind: KindTrain, Level: 33},
	}
	got, ok := combatPreparationObjective(obs, offered, known)
	if !ok || got.Kind != KindCatch || got.Species != "squirtle" {
		t.Fatalf("prep = %v %v, want the Squirtle catch (Water at L8)", got, ok)
	}
}

func TestCounterCampaignReleasesTheFightOnceACounterIsReady(t *testing.T) {
	challenge := Objective{Kind: KindGym, Place: "test gym"}
	obs := counterTestObservation(challenge)
	known := lostTo(challenge, obs)

	after := obs
	after.Party = append(append([]PartyMon(nil), obs.Party...),
		withMoves(PartyMon{Species: "squirtle", Level: 18, HP: 50, MaxHP: 50}, PartyMove{Move: Move{Power: 40, Type: "water"}, PP: 25}))
	after.PartyCount = 3
	known.notePartyCombatResult(obs, after, ObjectiveResult{Objective: Objective{Kind: KindTrain, Slot: 2}})

	key := combatRecoveryObjective(challenge).Key()
	if !combatRetryKeys(known)[key] {
		t.Fatalf("ready counter did not release the fight: %+v", known.Failures)
	}
	// The party's level score never reached the loss target; the counter
	// release must survive the next offer's stale-retry demotion.
	OfferWithEvidence(after, known)
	if !combatRetryKeys(known)[key] {
		t.Fatalf("counter release demoted back into training: %+v", known.Failures)
	}
}

func TestCounterCampaignWithNoCandidateDoesNotLockTheEscape(t *testing.T) {
	challenge := Objective{Kind: KindGym, Place: "test gym"}
	obs := counterTestObservation(challenge)
	known := lostTo(challenge, obs)
	if combatPreparationNeedsCoverage(obs, known) {
		t.Fatal("no counter catch and no XP here still locked the fight")
	}
	obs.CounterCandidates = []SpeciesID{"squirtle"}
	if !combatPreparationNeedsCoverage(obs, known) {
		t.Fatal("a known counter catch did not keep the campaign open")
	}
}

// Level-100 members gain no experience, so they are never offered as a
// training target.
func TestPartyTrainingNeverTargetsALevel100Member(t *testing.T) {
	obs := Observation{
		HasGrass: true,
		Party: []PartyMon{
			{Species: "venusaur", Level: 100, HP: 300, MaxHP: 300},
			{Species: "pidgey", Level: 100, HP: 200, MaxHP: 200},
			{Species: "rattata", Level: 20, HP: 50, MaxHP: 50},
		},
	}
	estimate := func(slot, target int) (TrainingEstimate, error) {
		return TrainingEstimate{Viability: TrainingViable, XPPerEncounter: 50}, nil
	}
	for _, o := range insertPartyTrainingObjectives(obs, NewKnowledge(nil), nil, estimate) {
		if o.Kind == KindTrain && obs.Party[o.Slot].Level >= 100 {
			t.Fatalf("offered training for a level-100 member: %+v", o)
		}
	}
}

// The live Yellow failure (run-j1lgrfvixz7j1bgxlgsfptofu): an L65 Pikachu
// lead lost to Giovanni. From Yellow's own ROM the Earth Badge team is
// Ground-heavy (Electric useless) and a Water catch is a counter.
func TestYellowGiovanniLossAsksForAROMCounter(t *testing.T) {
	romData := gen1TestROM(t, "POKEMON_YELLOW_ROM")
	challenge := Objective{Kind: KindProgress, Progress: redProgressEarthBadge}
	obs := Observation{
		PartyCount:        2,
		ChallengeProfiles: gen1ChallengeProfiles(romData),
		Learnsets:         gen1Learnsets(romData),
		Party: []PartyMon{
			withMoves(PartyMon{Species: "pikachu", Level: 65, HP: 160, MaxHP: 168},
				PartyMove{Move: Move{Power: 95, Type: "electric"}, PP: 15}),
			withMoves(PartyMon{Species: "rattata", Level: 11, HP: 30, MaxHP: 30},
				PartyMove{Move: Move{Power: 40, Type: "normal"}, PP: 30}),
		},
	}
	profile := challengeProfileFor(obs, challenge)
	if profile.Matchup.MaxLevel != 55 || !profile.Matchup.IsUseless("electric") {
		t.Fatalf("Yellow Earth Badge matchup = %+v, want L55 with electric useless", profile.Matchup)
	}
	known := lostTo(challenge, obs)
	need, ok := counterNeedFor(obs, known)
	if !ok || need.Action != ChallengeAcquireCounter {
		t.Fatalf("counter need = %+v %v, want acquire_counter", need, ok)
	}
	if mv, ok := counterLearnLevel(obs.Learnsets, "squirtle", need.Matchup, need.Target); !ok || mv.Type != "water" {
		t.Fatalf("Squirtle not a Water counter by L%d: %+v", need.Target, obs.Learnsets["squirtle"])
	}
	if _, ok := counterLearnLevel(obs.Learnsets, "pidgey", need.Matchup, need.Target); ok {
		t.Fatal("Pidgey (Normal/Flying) wrongly treated as a Giovanni counter")
	}
}
