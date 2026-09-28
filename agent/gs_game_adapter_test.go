package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

func TestGSObjectiveCatalogOffersThreeSemanticStarters(t *testing.T) {
	provider, ok := objectiveCatalogProviderFor(gsprofile.GoldGameID)
	if !ok {
		t.Fatal("Gold objective catalog provider is not registered")
	}
	catalog := provider.ObjectiveCatalog(Observation{GameID: gsprofile.GoldGameID, PartyCount: 0})
	want := []CatalogStarter{
		{Starter: skill.StarterChikorita, Species: "chikorita"},
		{Starter: skill.StarterCyndaquil, Species: "cyndaquil"},
		{Starter: skill.StarterTotodile, Species: "totodile"},
	}
	if len(catalog.Starters) != len(want) {
		t.Fatalf("starters = %+v, want %+v", catalog.Starters, want)
	}
	for i := range want {
		if catalog.Starters[i] != want[i] {
			t.Fatalf("starter %d = %+v, want %+v", i, catalog.Starters[i], want[i])
		}
	}
}

func TestGSObjectiveCatalogStopsOfferingStarterAfterPartyExists(t *testing.T) {
	provider, _ := objectiveCatalogProviderFor(gsprofile.SilverGameID)
	catalog := provider.ObjectiveCatalog(Observation{GameID: gsprofile.SilverGameID, PartyCount: 1})
	if len(catalog.Starters) != 0 {
		t.Fatalf("starters = %+v, want none", catalog.Starters)
	}
}

func TestGSStarterValidationBindsChoiceToSemanticSpecies(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	for _, tc := range []struct {
		starter skill.Starter
		species SpeciesID
	}{
		{skill.StarterChikorita, "chikorita"},
		{skill.StarterCyndaquil, "cyndaquil"},
		{skill.StarterTotodile, "totodile"},
	} {
		if err := adapter.Validate(Objective{Kind: KindStarter, Starter: tc.starter, Species: tc.species}, Observation{}); err != nil {
			t.Fatalf("%s: Validate: %v", tc.species, err)
		}
	}
	err := adapter.Validate(Objective{Kind: KindStarter, Starter: skill.StarterCyndaquil, Species: "totodile"}, Observation{})
	if err == nil {
		t.Fatal("mismatched starter species validated")
	}
}

func TestGSStarterSpecsMatchElmBallLayout(t *testing.T) {
	tests := []struct {
		starter skill.Starter
		species game.SpeciesID
		x       uint8
	}{
		{skill.StarterCyndaquil, "cyndaquil", 6},
		{skill.StarterTotodile, "totodile", 7},
		{skill.StarterChikorita, "chikorita", 8},
	}
	for _, tc := range tests {
		spec, ok := gsStarterSpecFor(tc.starter)
		if !ok {
			t.Fatalf("%s: no starter spec", tc.species)
		}
		if spec.Species != tc.species || spec.BallX != tc.x || spec.BallY != 3 ||
			spec.ApproachX != tc.x || spec.ApproachY != 4 {
			t.Fatalf("%s spec = %+v", tc.species, spec)
		}
	}
}

func TestGSUnimplementedObjectiveNormalizesAsTypedBlock(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	err := adapter.Validate(Objective{Kind: KindGoTo, Place: "violet-city"}, Observation{})
	if !errors.Is(err, errGSControllerUnavailable) {
		t.Fatalf("Validate error = %v, want controller unavailable", err)
	}
	failure := adapter.NormalizeFailure(game.FailurePhaseValidation, err, Observation{})
	if failure.Class != game.FailureClassBlocked || failure.Recoverable || failure.Cause != "gen2_controller_unavailable" {
		t.Fatalf("failure = %+v, want non-recoverable typed block", failure)
	}
}

func TestGSObjectiveAdapterFactoriesAreRegistered(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		if _, err := objectiveAdapterFactoryFor(id); err != nil {
			t.Fatalf("%s objective adapter: %v", id, err)
		}
	}
}

func TestStarterObjectiveForSpeciesUsesActiveGoldCatalog(t *testing.T) {
	obs := Observation{GameID: gsprofile.GoldGameID, PartyCount: 0}
	got, ok := StarterObjectiveForSpecies(obs, "cyndaquil")
	if !ok {
		t.Fatal("cyndaquil starter did not resolve")
	}
	want := Objective{Kind: KindStarter, Starter: skill.StarterCyndaquil, Species: "cyndaquil"}
	if got != want {
		t.Fatalf("objective = %+v, want %+v", got, want)
	}
	if _, ok := StarterObjectiveForSpecies(obs, "squirtle"); ok {
		t.Fatal("Gold catalog resolved Red starter squirtle")
	}
}

func TestGSProgressionOffersEarlyJohtoStagesInOrder(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	obs := Observation{
		GameID:     gsprofile.GoldGameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: gsprofile.ProgressStarterReceived, Complete: true},
			{ID: gsprofile.ProgressMysteryEggReturned, Complete: false},
		},
	}
	got := adapter.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != gsprofile.ProgressMysteryEggReturned {
		t.Fatalf("progression = %+v, want Mystery Egg return objective", got)
	}

	obs.Story = ProgressState{
		{ID: gsprofile.ProgressStarterReceived, Complete: true},
		{ID: gsprofile.ProgressMysteryEggReturned, Complete: true},
	}
	got = adapter.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gsprofile.ProgressSproutTowerCleared {
		t.Fatalf("after Egg return progression = %+v, want Sprout Tower", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressSproutTowerCleared, Complete: true})
	got = adapter.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gsprofile.ProgressZephyrBadgeEarned {
		t.Fatalf("after Sprout Tower progression = %+v, want Zephyr Badge", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressZephyrBadgeEarned, Complete: true})
	got = adapter.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gsprofile.ProgressTogepiEggReceived {
		t.Fatalf("after Zephyr Badge progression = %+v, want Togepi Egg handoff", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressTogepiEggReceived, Complete: true})
	got = adapter.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gsprofile.ProgressSlowpokeWellCleared {
		t.Fatalf("after Togepi Egg progression = %+v, want Slowpoke Well", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressSlowpokeWellCleared, Complete: true})
	got = adapter.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gsprofile.ProgressHiveBadgeEarned {
		t.Fatalf("after Slowpoke Well progression = %+v, want Hive Badge", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressHiveBadgeEarned, Complete: true})
	if got := adapter.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("completed Bugsy slice still offered: %+v", got)
	}
}

func TestGSProgressionDoesNotSkipDurableStarterBoundary(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	obs := Observation{
		GameID:     gsprofile.GoldGameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: gsprofile.ProgressStarterReceived, Complete: false},
		},
	}
	if got := adapter.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("errand offered before starter event completed: %+v", got)
	}
}

func TestGSPostStarterProgressValidationIsNarrow(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	for _, progress := range []ProgressID{
		gsprofile.ProgressMysteryEggReturned,
		gsprofile.ProgressSproutTowerCleared,
		gsprofile.ProgressZephyrBadgeEarned,
		gsprofile.ProgressTogepiEggReceived,
		gsprofile.ProgressSlowpokeWellCleared,
		gsprofile.ProgressHiveBadgeEarned,
	} {
		if err := adapter.Validate(Objective{Kind: KindProgress, Progress: progress}, Observation{}); err != nil {
			t.Fatalf("Validate %s: %v", progress, err)
		}
	}
	err := adapter.Validate(Objective{Kind: KindProgress, Progress: "gs_future_goal"}, Observation{})
	if !errors.Is(err, errGSControllerUnavailable) {
		t.Fatalf("future progress validation = %v, want controller unavailable", err)
	}
}

func TestGSPostStarterProgressUsesGenericStoryVerifier(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	o := Objective{Kind: KindProgress, Progress: gsprofile.ProgressMysteryEggReturned}
	final := Observation{
		Controllable: true,
		Story: ProgressState{
			{ID: gsprofile.ProgressMysteryEggReturned, Complete: true},
		},
	}
	if err := adapter.VerifyPostcondition(o, Observation{}, final, ObjectiveResult{}); err != nil {
		t.Fatalf("VerifyPostcondition: %v", err)
	}
}

func TestGSFirstBadgeProgressUsesGenericStoryVerifier(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	for _, progress := range []ProgressID{
		gsprofile.ProgressSproutTowerCleared,
		gsprofile.ProgressZephyrBadgeEarned,
		gsprofile.ProgressTogepiEggReceived,
		gsprofile.ProgressSlowpokeWellCleared,
		gsprofile.ProgressHiveBadgeEarned,
	} {
		o := Objective{Kind: KindProgress, Progress: progress}
		final := Observation{
			Controllable: true,
			Story:        ProgressState{{ID: progress, Complete: true}},
		}
		if err := adapter.VerifyPostcondition(o, Observation{}, final, ObjectiveResult{}); err != nil {
			t.Fatalf("VerifyPostcondition(%s): %v", progress, err)
		}
	}
}

func TestGSRequiredBattleLossNormalizesAsCombatDefeat(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	err := skill.RequireTrainerBattleWin("gym:falkner", game.BattleLost)
	failure := adapter.NormalizeFailure(game.FailurePhaseExecution, err, Observation{})
	if failure.Class != game.FailureClassBlocked || !failure.Recoverable {
		t.Fatalf("failure = %+v, want recoverable blocked", failure)
	}
	if failure.Cause != failureCauseCombatDefeat {
		t.Fatalf("cause = %q, want %q", failure.Cause, failureCauseCombatDefeat)
	}
	if len(failure.Context) != 1 || failure.Context[0] != "gym:falkner" {
		t.Fatalf("context = %+v, want Falkner encounter", failure.Context)
	}
}

func TestGSFirstBadgeStagesGetBattleSizedWatchdog(t *testing.T) {
	for _, progress := range []ProgressID{
		gsprofile.ProgressSproutTowerCleared,
		gsprofile.ProgressZephyrBadgeEarned,
		gsprofile.ProgressSlowpokeWellCleared,
		gsprofile.ProgressHiveBadgeEarned,
	} {
		got := gsObjectiveFrameBudget(Objective{Kind: KindProgress, Progress: progress})
		if got != gsFirstBadgeObjectiveFrameBudget {
			t.Fatalf("budget(%s) = %d, want %d", progress, got, gsFirstBadgeObjectiveFrameBudget)
		}
	}
	for _, progress := range []ProgressID{gsprofile.ProgressMysteryEggReturned, gsprofile.ProgressTogepiEggReceived} {
		if got := gsObjectiveFrameBudget(Objective{Kind: KindProgress, Progress: progress}); got != objectiveFrameBudget {
			t.Fatalf("%s budget = %d, want ordinary %d", progress, got, objectiveFrameBudget)
		}
	}
}
