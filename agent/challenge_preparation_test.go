package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestProfiledProgressionChallengeIsEvaluatedBeforeFirstLoss(t *testing.T) {
	challenge := Objective{Kind: KindProgress, Progress: redProgressBoulderBadge}
	obs := Observation{
		PartyCount: 1,
		Party: []PartyMon{{
			Species: "charmander", Level: 7, HP: 25, MaxHP: 25,
		}},
		Catalog: ObjectiveCatalog{ChallengeProfiles: []CatalogChallengeProfile{{
			Objective: challenge.Key(),
			Readiness: ChallengeReadinessProfile{MinimumReadiness: redReadinessFloor(14)},
		}}},
	}
	offer := ObjectiveOffer{Candidates: []Objective{challenge}}
	got := challengeReadinessForOffer(obs, NewKnowledge(nil), offer)
	if len(got) != 1 {
		t.Fatalf("readiness = %+v, want one profiled progression challenge", got)
	}
	if got[0].Action != ChallengeTrain || got[0].Losses != 0 {
		t.Fatalf("readiness = %+v, want proactive train before any loss", got[0])
	}
	if got[0].CurrentReadiness != 28 || got[0].TargetReadiness != 56 {
		t.Fatalf("readiness score = %d/%d, want 28/56", got[0].CurrentReadiness, got[0].TargetReadiness)
	}
}

func TestProactiveChallengePreparationChoosesLocalTraining(t *testing.T) {
	challenge := Objective{Kind: KindGym, Place: "test gym"}
	train := Objective{Kind: KindTrain, Level: 12}
	readiness := []ChallengeReadiness{{
		Objective: challenge.Key(), Action: ChallengeTrain,
		CurrentReadiness: 36, TargetReadiness: 56,
	}}
	got, assessment, ok := proactiveChallengePreparationObjective(
		Observation{}, []Objective{challenge, train}, readiness, NewKnowledge(nil),
	)
	if !ok || got.Key() != train.Key() {
		t.Fatalf("preparation = %v ok=%v, want local training", got, ok)
	}
	if assessment.Action != ChallengeTrain {
		t.Fatalf("assessment = %+v", assessment)
	}
}

func TestProactiveChallengePreparationRoutesToSelectedTrainingArea(t *testing.T) {
	current := LocationID("current")
	training := LocationID("training")
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		current:  {training},
		training: {current},
	}})
	known.TrainingAreas[training] = TrainingAreaKnowledge{
		Location: training, Place: "training route", MinLevel: 12, MaxLevel: 15,
	}
	estimate := TrainingEstimate{
		CurrentLevel: 8, TargetLevel: 12, XPPerEncounter: 100,
		EstimatedEncounters: 8, SessionBudget: 20, Viability: TrainingViable, Method: TrainingDirect,
	}
	obs := Observation{
		Location: PlaceID(current),
		Party:    []PartyMon{{Species: "testmon", Level: 8, HP: 30, MaxHP: 30}},
		TrainingAreaChoices: []TrainingAreaAssessment{{
			Place: "training route", Location: training, Selected: true, Routable: true,
			Estimate: estimate, TotalCost: 900,
		}},
	}
	challenge := Objective{Kind: KindProgress, Progress: redProgressBoulderBadge}
	journey := Objective{Kind: KindGoTo, Place: "training route", Flee: true}
	readiness := []ChallengeReadiness{{
		Objective: challenge.Key(), Action: ChallengeTrain,
		CurrentReadiness: 32, TargetReadiness: 56,
	}}
	got, _, ok := proactiveChallengePreparationObjective(obs, []Objective{challenge, journey}, readiness, known)
	if !ok || got.Kind != KindGoTo || got.Place != "training route" {
		t.Fatalf("preparation = %+v ok=%v, want journey to selected training area", got, ok)
	}
}

func TestProactiveChallengePreparationHealsBeforeChallenge(t *testing.T) {
	challenge := Objective{Kind: KindGym, Place: "test gym"}
	heal := Objective{Kind: KindHeal, Place: "test pokemon center"}
	readiness := []ChallengeReadiness{{
		Objective: challenge.Key(), Action: ChallengeHeal,
		CurrentReadiness: 80, TargetReadiness: 80,
	}}
	got, _, ok := proactiveChallengePreparationObjective(
		Observation{}, []Objective{challenge, heal}, readiness, NewKnowledge(nil),
	)
	if !ok || got.Key() != heal.Key() {
		t.Fatalf("preparation = %+v ok=%v, want heal", got, ok)
	}
}

func TestProactiveChallengePreparationRestocksFromOfferedShopWork(t *testing.T) {
	challenge := Objective{Kind: KindGym, Place: "test gym"}
	buy := Objective{Kind: KindBuy, Item: "super potion", Qty: 2}
	readiness := []ChallengeReadiness{{
		Objective: challenge.Key(), Action: ChallengeRestock,
		CurrentReadiness: 80, TargetReadiness: 80,
	}}
	got, _, ok := proactiveChallengePreparationObjective(
		Observation{}, []Objective{challenge, buy}, readiness, NewKnowledge(nil),
	)
	if !ok || got.Key() != buy.Key() {
		t.Fatalf("preparation = %+v ok=%v, want healing restock", got, ok)
	}
}

func TestProactiveChallengePreparationDoesNotInventPartyChange(t *testing.T) {
	challenge := Objective{Kind: KindGym, Place: "test gym"}
	catch := Objective{Kind: KindCatch, Species: "pidgey"}
	readiness := []ChallengeReadiness{{
		Objective: challenge.Key(), Action: ChallengeChangeParty,
		CurrentReadiness: 80, TargetReadiness: 80,
	}}
	if got, _, ok := proactiveChallengePreparationObjective(
		Observation{}, []Objective{challenge, catch}, readiness, NewKnowledge(nil),
	); ok {
		t.Fatalf("invented deterministic party change %v; strategist must choose actual composition work", got)
	}
}

func TestOrdinaryUnprofiledTrainerDoesNotForceProactivePreparation(t *testing.T) {
	challenge := Objective{Kind: KindTrainer, Location: "route", X: 1, Y: 2}
	train := Objective{Kind: KindTrain, Level: 12}
	readiness := []ChallengeReadiness{{
		Objective: challenge.Key(), Action: ChallengeTrain,
		CurrentReadiness: 20, TargetReadiness: 0, Losses: 0,
	}}
	if got, _, ok := proactiveChallengePreparationObjective(
		Observation{}, []Objective{challenge, train}, readiness, NewKnowledge(nil),
	); ok {
		t.Fatalf("ordinary unprofiled trainer forced proactive work: %v", got)
	}
}

func TestRedGymCatalogCarriesDocumentedReadinessFloor(t *testing.T) {
	obs := Observation{Map: 0x36, Story: ProgressState{}}
	catalog := redObjectiveCatalog(obs)
	if len(catalog.Challenges) != 1 {
		t.Fatalf("challenges = %+v, want Pewter gym", catalog.Challenges)
	}
	got := catalog.Challenges[0]
	if got.Readiness.MinimumReadiness != redReadinessFloor(14) {
		t.Fatalf("Boulder readiness = %+v, want level-14 weighted floor", got.Readiness)
	}
	if got.Complete {
		t.Fatal("fresh Boulder challenge unexpectedly complete")
	}

	obs.Story.Badges = 1 << uint8(state.BadgeBoulder)
	catalog = redObjectiveCatalog(obs)
	if !catalog.Challenges[0].Complete {
		t.Fatal("Boulder challenge should reflect owned badge")
	}
}

func TestRedLeagueProfileUsesChampionCeiling(t *testing.T) {
	catalog := redObjectiveCatalog(Observation{})
	want := (Objective{Kind: KindProgress, Progress: ProgressLeagueChampionDefeated}).Key()
	for _, profile := range catalog.ChallengeProfiles {
		if profile.Objective == want {
			if profile.Readiness.MinimumReadiness != redReadinessFloor(65) {
				t.Fatalf("Champion readiness = %+v, want level-65 weighted floor", profile.Readiness)
			}
			return
		}
	}
	t.Fatal("Champion challenge profile missing")
}
