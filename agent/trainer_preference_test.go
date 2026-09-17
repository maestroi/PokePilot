package agent

import "testing"

func TestTrainerPreferenceProfilesRewardFiniteTrainerXP(t *testing.T) {
	obs := Observation{
		PartyCount: 2,
		Party: []PartyMon{
			{Species: SpeciesID("wartortle"), Level: 24, HP: 60, MaxHP: 60},
			{Species: SpeciesID("pikachu"), Level: 15, HP: 35, MaxHP: 35},
		},
	}
	trainer := Objective{Kind: KindTrainer, X: 5, Y: 5}

	adventure := ScoreObjective(obs, trainer, PlayStyle(PlayStyleAdventure))
	teamBuilder := ScoreObjective(obs, trainer, PlayStyle(PlayStyleTeamBuilder))
	completionist := ScoreObjective(obs, trainer, PlayStyle(PlayStyleCompletionist))

	if !hasNaturalTag(adventure.Natural, "useful-training") {
		t.Fatalf("adventure trainer tags = %v, want useful-training", adventure.Natural.Tags)
	}
	if teamBuilder.Natural.Bonus <= adventure.Natural.Bonus {
		t.Fatalf("team builder trainer bonus %.3f <= adventure %.3f", teamBuilder.Natural.Bonus, adventure.Natural.Bonus)
	}
	if completionist.Natural.Bonus <= adventure.Natural.Bonus {
		t.Fatalf("completionist trainer bonus %.3f <= adventure %.3f", completionist.Natural.Bonus, adventure.Natural.Bonus)
	}
}

func TestTrainerPreferenceRewardsIncomeWhenEconomyNeedsResupply(t *testing.T) {
	profile := PlayStyle(PlayStyleAdventure)
	trainer := Objective{Kind: KindTrainer, X: 5, Y: 5}
	base := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{Species: SpeciesID("wartortle"), Level: 20, HP: 55, MaxHP: 55}},
	}
	needsMoney := base
	needsMoney.HasGrass = true
	needsMoney.WildGrass = []WildSpecies{{Name: "rattata", MinLevel: 3, MaxLevel: 5, Slots: 10}}

	baseScore := ScoreObjective(base, trainer, profile)
	moneyScore := ScoreObjective(needsMoney, trainer, profile)
	if !hasNaturalTag(moneyScore.Natural, "trainer-income") {
		t.Fatalf("trainer tags = %v, want trainer-income", moneyScore.Natural.Tags)
	}
	if moneyScore.Natural.Bonus <= baseScore.Natural.Bonus {
		t.Fatalf("money-pressure trainer bonus %.3f <= base %.3f", moneyScore.Natural.Bonus, baseScore.Natural.Bonus)
	}
}

func TestTrainerPreferenceStillPaysDistanceAndBattleRisk(t *testing.T) {
	profile := PlayStyle(PlayStyleTeamBuilder)
	obs := Observation{
		X:          5,
		Y:          5,
		PartyCount: 2,
		Party: []PartyMon{
			{Species: SpeciesID("wartortle"), Level: 24, HP: 60, MaxHP: 60},
			{Species: SpeciesID("pikachu"), Level: 15, HP: 35, MaxHP: 35},
		},
	}
	near := ScoreObjective(obs, Objective{Kind: KindTrainer, X: 6, Y: 5}, profile)
	far := ScoreObjective(obs, Objective{Kind: KindTrainer, X: 28, Y: 5}, profile)

	if far.Cost.Travel <= near.Cost.Travel {
		t.Fatalf("far travel cost %.3f <= near %.3f", far.Cost.Travel, near.Cost.Travel)
	}
	if far.Total >= near.Total {
		t.Fatalf("far trainer total %.3f >= near %.3f", far.Total, near.Total)
	}
	if near.Cost.EncounterRisk <= 0 || far.Cost.EncounterRisk <= 0 {
		t.Fatalf("trainer encounter risk missing: near %.3f far %.3f", near.Cost.EncounterRisk, far.Cost.EncounterRisk)
	}
}
