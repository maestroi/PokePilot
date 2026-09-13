package agent

import (
	"strings"
	"testing"
)

func TestAdventureSingleGenericFailureDoesNotStartBroadFallback(t *testing.T) {
	obs := Observation{
		PartyCount: 2,
		History: []RoundRecord{{
			Objective: "talk at (4,4)",
			Outcome:   "blocked: dialogue interrupted",
		}},
		Failures: []Failure{{Objective: "talk at (4,4)", Times: 1}},
	}
	ctx := detectStallContext(obs)
	if ctx.Active {
		t.Fatalf("single generic failure activated fallback: %#v", ctx)
	}
}

func TestAdventureMissedNPCPrerequisiteBeatsImmediateFailedRetry(t *testing.T) {
	profile := PlayStyle("adventure")
	failed := Objective{Kind: KindProgress, Progress: ProgressID("enter_test_gate")}
	obs := Observation{
		X:          5,
		Y:          5,
		PartyCount: 2,
		History: []RoundRecord{{
			Objective: failed.String(),
			Outcome:   "blocked: the guard will not let us through",
		}},
		Failures: []Failure{{Objective: failed.String(), Times: 1}},
		Requirements: []Requirement{{
			Text: "You need permission before you can go through.",
		}},
	}
	talk := Objective{Kind: KindTalk, X: 6, Y: 5}

	ctx := detectStallContext(obs)
	if !ctx.Active || !ctx.Prerequisite {
		t.Fatalf("stall context = %#v, want active prerequisite search", ctx)
	}
	talkScore := ScoreObjective(obs, talk, profile)
	retryScore := ScoreObjective(obs, failed, profile)
	if talkScore.Total <= retryScore.Total {
		t.Fatalf("NPC search %.3f <= failed retry %.3f", talkScore.Total, retryScore.Total)
	}
	if !hasStallTag(talkScore.Natural.Stall, "prerequisite-npc") {
		t.Fatalf("talk stall tags = %v, want prerequisite-npc", talkScore.Natural.Stall.Tags)
	}
	if !hasStallTag(retryScore.Natural.Stall, "stalled-retry") {
		t.Fatalf("retry stall tags = %v, want stalled-retry", retryScore.Natural.Stall.Tags)
	}
}

func TestAdventureMissingCapabilityPrioritizesPreparation(t *testing.T) {
	profile := PlayStyle("adventure")
	failed := Objective{Kind: KindGoTo, Place: PlaceID("vermillion gym")}
	obs := Observation{
		PartyCount: 2,
		History: []RoundRecord{{
			Objective: failed.String(),
			Outcome:   "blocked: route gate closed",
		}},
		Failures: []Failure{{Objective: failed.String(), Times: 1}},
		RouteBlockages: []RouteBlockage{{
			Destination: PlaceID("vermillion gym"),
			Missing:     []CapabilityID{CapabilityID("can_cut")},
		}},
	}
	prepare := Objective{Kind: KindUseItem, Item: ItemID("hm01"), Slot: 1}

	ctx := detectStallContext(obs)
	if !ctx.Active || !ctx.Navigation || !ctx.Prerequisite {
		t.Fatalf("stall context = %#v, want navigation+prerequisite", ctx)
	}
	score := ScoreObjective(obs, prepare, profile)
	if score.Natural.Stall.Bonus <= 0 || !hasStallTag(score.Natural.Stall, "prepare-capability") {
		t.Fatalf("HM preparation stall signal = %#v", score.Natural.Stall)
	}
}

func TestAdventureUnreachableTransitionRaisesFrontierSearch(t *testing.T) {
	profile := PlayStyle("adventure")
	failed := Objective{Kind: KindGoTo, Place: PlaceID("route 9")}
	obs := Observation{
		PartyCount: 2,
		History: []RoundRecord{{
			Objective: failed.String(),
			Outcome:   "blocked: no route",
		}},
		Failures:  []Failure{{Objective: failed.String(), Times: 1}},
		Unroutable: []string{"route 9"},
	}
	frontier := Objective{Kind: KindGoTo, Place: PlaceID("route 10"), Flee: true, Note: "(unvisited adjacent map)"}

	ctx := detectStallContext(obs)
	if !ctx.Active || !ctx.Navigation {
		t.Fatalf("stall context = %#v, want navigation fallback", ctx)
	}
	score := ScoreObjective(obs, frontier, profile)
	if !hasStallTag(score.Natural.Stall, "search-frontier") {
		t.Fatalf("frontier stall tags = %v, want search-frontier", score.Natural.Stall.Tags)
	}
}

func TestAdventureBattleFailureRaisesPreparationInsteadOfWandering(t *testing.T) {
	profile := PlayStyle("adventure")
	gym := Objective{Kind: KindGym, Place: PlaceID("cerulean gym")}
	obs := Observation{
		PartyCount: 2,
		Party: []PartyMon{
			{Species: SpeciesID("wartortle"), Level: 20, HP: 35, MaxHP: 50},
			{Species: SpeciesID("pikachu"), Level: 13, HP: 30, MaxHP: 30},
		},
		HasGrass: true,
		History: []RoundRecord{{
			Objective: gym.String(),
			Outcome:   "blocked: lost to the gym leader and blacked out",
		}},
		Failures: []Failure{{Objective: gym.String(), Times: 1}},
	}
	train := Objective{Kind: KindTrain, Species: SpeciesID("pikachu"), Slot: 1, Level: 15}
	frontier := Objective{Kind: KindGoTo, Place: PlaceID("route 5"), Flee: true, Note: "(unvisited adjacent map)"}

	ctx := detectStallContext(obs)
	if !ctx.Active || !ctx.Battle {
		t.Fatalf("stall context = %#v, want battle fallback", ctx)
	}
	trainScore := ScoreObjective(obs, train, profile)
	frontierScore := ScoreObjective(obs, frontier, profile)
	if !hasStallTag(trainScore.Natural.Stall, "prepare-battle-gate") {
		t.Fatalf("training stall tags = %v, want prepare-battle-gate", trainScore.Natural.Stall.Tags)
	}
	if hasStallTag(frontierScore.Natural.Stall, "search-frontier") {
		t.Fatalf("battle-only stall should not turn into random frontier wandering: %v", frontierScore.Natural.Stall.Tags)
	}
}

func TestAdventureRouteLoopSearchesAlternativeAndPenalizesLoopRoute(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{
		PartyCount: 2,
		History: []RoundRecord{
			{Objective: "go to route 9", Outcome: "done"},
			{Objective: "go to cerulean city", Outcome: "done"},
			{Objective: "go to route 9", Outcome: "done"},
			{Objective: "go to cerulean city", Outcome: "done"},
		},
	}
	loopRoute := Objective{Kind: KindGoTo, Place: PlaceID("route 9")}
	frontier := Objective{Kind: KindGoTo, Place: PlaceID("route 10"), Flee: true, Note: "(unvisited adjacent map)"}

	ctx := detectStallContext(obs)
	if !ctx.Active || !ctx.Loop {
		t.Fatalf("stall context = %#v, want route loop", ctx)
	}
	loopScore := ScoreObjective(obs, loopRoute, profile)
	frontierScore := ScoreObjective(obs, frontier, profile)
	if !hasStallTag(loopScore.Natural.Stall, "loop-route") {
		t.Fatalf("loop route tags = %v, want loop-route", loopScore.Natural.Stall.Tags)
	}
	if !hasStallTag(frontierScore.Natural.Stall, "search-frontier") {
		t.Fatalf("frontier tags = %v, want search-frontier", frontierScore.Natural.Stall.Tags)
	}
	if frontierScore.Total <= loopScore.Total {
		t.Fatalf("frontier %.3f <= loop route %.3f", frontierScore.Total, loopScore.Total)
	}
}

func TestAdventureRepeatedFallbackAlternativeLosesPriority(t *testing.T) {
	profile := PlayStyle("adventure")
	frontier := Objective{Kind: KindGoTo, Place: PlaceID("route 10"), Flee: true, Note: "(unvisited adjacent map)"}
	obs := Observation{
		PartyCount: 2,
		History: []RoundRecord{
			{Objective: frontier.String(), Outcome: "done"},
			{Objective: frontier.String(), Outcome: "done"},
			{Objective: frontier.String(), Outcome: "done"},
			{Objective: frontier.String(), Outcome: "done"},
		},
	}
	progress := Objective{Kind: KindProgress, Progress: ProgressID("test_progress")}

	frontierScore := ScoreObjective(obs, frontier, profile)
	progressScore := ScoreObjective(obs, progress, profile)
	if !hasStallTag(frontierScore.Natural.Stall, "exhausted-alternative") || !hasStallTag(frontierScore.Natural.Stall, "loop-route") {
		t.Fatalf("repeated frontier stall tags = %v", frontierScore.Natural.Stall.Tags)
	}
	if frontierScore.Total >= progressScore.Total {
		t.Fatalf("exhausted frontier %.3f >= normal progression %.3f", frontierScore.Total, progressScore.Total)
	}
}

func TestAdventureFreshProgressResetsOldStallPressure(t *testing.T) {
	failed := "progress enter_test_gate"
	obs := Observation{
		History: []RoundRecord{
			{Objective: failed, Outcome: "blocked: missing permission"},
			{Objective: failed, Outcome: "blocked: missing permission"},
			{Objective: "progress obtained_permission", Outcome: "done"},
		},
		Failures: []Failure{{Objective: failed, Times: 2}},
		Requirements: []Requirement{{Text: "You need permission."}},
	}
	ctx := detectStallContext(obs)
	if ctx.Active {
		t.Fatalf("fresh progression did not reset fallback: %#v", ctx)
	}
}

func TestAdventureAnnotationExplainsStallFallback(t *testing.T) {
	profile := PlayStyle("adventure")
	failed := Objective{Kind: KindGoTo, Place: PlaceID("route 9")}
	obs := Observation{
		PartyCount: 2,
		History: []RoundRecord{{
			Objective: failed.String(),
			Outcome:   "blocked: no route",
		}},
		Failures:  []Failure{{Objective: failed.String(), Times: 1}},
		Unroutable: []string{"route 9"},
	}
	frontier := Objective{Kind: KindGoTo, Place: PlaceID("route 10"), Flee: true, Note: "(unvisited adjacent map)"}
	got := AnnotatePlayStyle(obs, []Objective{frontier}, profile)
	if len(got) != 1 || !strings.Contains(got[0].Note, "stall L") || !strings.Contains(got[0].Note, "due") {
		t.Fatalf("annotation does not explain stalled progression fallback: %q", got[0].Note)
	}
}

func hasStallTag(signal StallFallbackSignal, want string) bool {
	for _, tag := range signal.Tags {
		if tag == want {
			return true
		}
	}
	return false
}
