package agent

import "testing"

func TestNormalizePlayStyleKeepsLegacyEmptyAsSpeedrun(t *testing.T) {
	cases := map[string]string{
		"":              PlayStyleSpeedrun,
		"speedrun":      PlayStyleSpeedrun,
		"ADVENTURE":     PlayStyleAdventure,
		"completionist": PlayStyleCompletionist,
		"team-builder":  PlayStyleTeamBuilder,
		"teambuilder":   PlayStyleTeamBuilder,
		"unknown":       PlayStyleSpeedrun,
	}
	for in, want := range cases {
		if got := NormalizePlayStyle(in); got != want {
			t.Fatalf("NormalizePlayStyle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRoutePriorityForPlayStyle(t *testing.T) {
	if got := RoutePriorityForPlayStyle(PlayStyle(PlayStyleSpeedrun)); got != RoutePriorityFastest {
		t.Fatalf("speedrun route priority = %v, want fastest", got)
	}
	if got := RoutePriorityForPlayStyle(PlayStyle(PlayStyleAdventure)); got != RoutePriorityConservative {
		t.Fatalf("adventure route priority = %v, want conservative", got)
	}
	if got := routePriorityForPlanner(NewStyledLLMPlanner(nil, PlayStyleSpeedrun)); got != RoutePriorityFastest {
		t.Fatalf("styled speedrun planner route priority = %v, want fastest", got)
	}
}

func TestPlayStylesAreIndependentDataProfiles(t *testing.T) {
	adventure := PlayStyle(PlayStyleAdventure)
	completionist := PlayStyle(PlayStyleCompletionist)
	teamBuilder := PlayStyle(PlayStyleTeamBuilder)

	adventure.Weights[DriveProgression] = 99
	if got := PlayStyle(PlayStyleAdventure).Weights[DriveProgression]; got == 99 {
		t.Fatal("PlayStyle returned shared mutable weights")
	}
	if completionist.DetourPenalty >= adventure.DetourPenalty {
		t.Fatalf("completionist detour penalty %.2f >= adventure %.2f", completionist.DetourPenalty, adventure.DetourPenalty)
	}
	if teamBuilder.PartyScale <= adventure.PartyScale {
		t.Fatalf("team builder party scale %.2f <= adventure %.2f", teamBuilder.PartyScale, adventure.PartyScale)
	}
}

func TestProfileRankingsChangeDeterministically(t *testing.T) {
	obs := Observation{Map: 0xff, PartyCount: 6}
	progress := Objective{Kind: KindProgress, Progress: ProgressID("test_progress")}
	frontier := Objective{Kind: KindGoTo, Place: PlaceID("unknown frontier"), Flee: true, Note: "(unvisited adjacent map)"}

	speedProgress := ScoreObjective(obs, progress, PlayStyle(PlayStyleSpeedrun))
	speedFrontier := ScoreObjective(obs, frontier, PlayStyle(PlayStyleSpeedrun))
	if speedProgress.Total <= speedFrontier.Total {
		t.Fatalf("speedrun should prefer direct progress: progress %.3f frontier %.3f", speedProgress.Total, speedFrontier.Total)
	}

	adventureProgress := ScoreObjective(obs, progress, PlayStyle(PlayStyleAdventure))
	adventureFrontier := ScoreObjective(obs, frontier, PlayStyle(PlayStyleAdventure))
	if adventureFrontier.Total <= adventureProgress.Total {
		t.Fatalf("adventure should value a fresh frontier: frontier %.3f progress %.3f", adventureFrontier.Total, adventureProgress.Total)
	}

	completionistProgress := ScoreObjective(obs, progress, PlayStyle(PlayStyleCompletionist))
	completionistFrontier := ScoreObjective(obs, frontier, PlayStyle(PlayStyleCompletionist))
	if completionistFrontier.Total <= completionistProgress.Total {
		t.Fatalf("completionist should strongly prefer fresh optional coverage: frontier %.3f progress %.3f", completionistFrontier.Total, completionistProgress.Total)
	}

	teamProgress := ScoreObjective(obs, progress, PlayStyle(PlayStyleTeamBuilder))
	teamFrontier := ScoreObjective(obs, frontier, PlayStyle(PlayStyleTeamBuilder))
	if teamProgress.Total <= teamFrontier.Total {
		t.Fatalf("team builder should not become an explorer profile: progress %.3f frontier %.3f", teamProgress.Total, teamFrontier.Total)
	}
}

func TestCompletionistValuesOptionalInteractionMost(t *testing.T) {
	obs := Observation{X: 0, Y: 0, PartyCount: 6}
	talk := Objective{Kind: KindTalk, X: 10, Y: 10}

	adventure := ScoreObjective(obs, talk, PlayStyle(PlayStyleAdventure))
	completionist := ScoreObjective(obs, talk, PlayStyle(PlayStyleCompletionist))
	teamBuilder := ScoreObjective(obs, talk, PlayStyle(PlayStyleTeamBuilder))
	if completionist.Total <= adventure.Total || completionist.Total <= teamBuilder.Total {
		t.Fatalf("optional NPC scores: completionist %.3f adventure %.3f team_builder %.3f", completionist.Total, adventure.Total, teamBuilder.Total)
	}
}

func TestTeamBuilderValuesCatchUpTrainingMost(t *testing.T) {
	obs := Observation{
		PartyCount: 2,
		Party: []PartyMon{
			{Species: SpeciesID("wartortle"), Level: 24, HP: 60, MaxHP: 60},
			{Species: SpeciesID("pikachu"), Level: 15, HP: 35, MaxHP: 35},
		},
	}
	train := Objective{Kind: KindTrain, Species: SpeciesID("pikachu"), Slot: 1, Level: 17}

	adventure := ScoreObjective(obs, train, PlayStyle(PlayStyleAdventure))
	completionist := ScoreObjective(obs, train, PlayStyle(PlayStyleCompletionist))
	teamBuilder := ScoreObjective(obs, train, PlayStyle(PlayStyleTeamBuilder))
	if teamBuilder.Total <= adventure.Total || teamBuilder.Total <= completionist.Total {
		t.Fatalf("training scores: team_builder %.3f adventure %.3f completionist %.3f", teamBuilder.Total, adventure.Total, completionist.Total)
	}
}

func TestSpeedrunAnnotationRemainsExactNoOp(t *testing.T) {
	obs := Observation{PartyCount: 2}
	offered := []Objective{{Kind: KindProgress, Progress: ProgressID("test_progress"), Note: "existing note"}}
	got := AnnotatePlayStyle(obs, offered, PlayStyle(""))
	if got[0] != offered[0] {
		t.Fatalf("legacy empty play style changed offered objective: got %#v want %#v", got[0], offered[0])
	}
}
