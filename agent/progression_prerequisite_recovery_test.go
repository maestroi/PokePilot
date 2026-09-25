package agent

import (
	"errors"
	"reflect"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

type fakeProgressionPrerequisiteAdapter struct{}

func (fakeProgressionPrerequisiteAdapter) RecoveryForProgressionPrerequisite(
	id ProgressID,
	_ Observation,
) (PrerequisiteRecoveryLink, bool) {
	if id != "door_unlocked" {
		return PrerequisiteRecoveryLink{}, false
	}
	return PrerequisiteRecoveryLink{
		Objective:    Objective{Kind: KindProgress, Progress: id},
		RecoverySafe: true,
	}, true
}

func TestGenericProgressionPrerequisiteRecoveryCanUseNonRedAdapter(t *testing.T) {
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = []Prerequisite{{Progress: "door_unlocked"}}

	got, repaired, ok := policy.prerequisiteRecovery(
		Observation{},
		nil,
		fakeProgressionPrerequisiteAdapter{},
	)
	if !ok {
		t.Fatal("generic progression prerequisite did not synthesize adapter-approved recovery")
	}
	want := Objective{Kind: KindProgress, Progress: "door_unlocked"}
	if got.Key() != want.Key() {
		t.Fatalf("recovery objective = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(repaired, []Prerequisite{{Progress: "door_unlocked"}}) {
		t.Fatalf("repaired prerequisites = %+v", repaired)
	}

	// Recovery always re-evaluates the fresh observation. Once the fact is
	// present, stale pending evidence is pruned instead of replaying recovery.
	fresh := Observation{Story: ProgressState{{ID: "door_unlocked", Complete: true}}}
	if got, repaired, ok := policy.prerequisiteRecovery(fresh, nil, fakeProgressionPrerequisiteAdapter{}); ok {
		t.Fatalf("completed prerequisite replayed recovery %+v via %+v", got, repaired)
	}
}

func TestLeagueResetRecoversMissingStagesOneAtATime(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	obs := Observation{
		Story: ProgressState{
			{ID: redProgressIndigoPlateauReady, Complete: true},
		},
	}
	requested := Objective{Kind: KindProgress, Progress: redProgressLeagueBrunoDefeated}

	err := adapter.Validate(requested, obs)
	var missing *gameruntime.PrerequisiteMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("Bruno validation error = %v, want typed prerequisite error", err)
	}
	wantMissing := []gameruntime.Prerequisite{
		gameruntime.ProgressionPrerequisite(ProgressLeagueChallengeStarted),
		gameruntime.ProgressionPrerequisite(redProgressLeagueLoreleiDefeated),
	}
	if !reflect.DeepEqual(missing.Missing, wantMissing) {
		t.Fatalf("Bruno missing prerequisites = %+v, want %+v", missing.Missing, wantMissing)
	}

	failure := normalizeRedFailure(gameruntime.FailurePhaseValidation, err, obs)
	if failure.Cause != "progression_prerequisite_missing" {
		t.Fatalf("normalized cause = %q, want progression_prerequisite_missing", failure.Cause)
	}
	if !reflect.DeepEqual(failure.Prerequisites, wantMissing) {
		t.Fatalf("normalized prerequisites = %+v, want %+v", failure.Prerequisites, wantMissing)
	}

	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{
		Objective: requested,
		Outcome:   OutcomeBlocked,
		Failure:   &failure,
		Final:     obs,
	})

	first, repaired, ok := policy.prerequisiteRecovery(obs, redProgressionObjectives(obs), adapter)
	if !ok || first.Kind != KindProgress || first.Progress != ProgressLeagueChallengeStarted {
		t.Fatalf("first reset recovery = %+v repaired=%+v ok=%v, want League start", first, repaired, ok)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: ProgressLeagueChallengeStarted, Complete: true})
	policy.success()
	second, repaired, ok := policy.prerequisiteRecovery(obs, redProgressionObjectives(obs), adapter)
	if !ok || second.Kind != KindProgress || second.Progress != redProgressLeagueLoreleiDefeated {
		t.Fatalf("second reset recovery = %+v repaired=%+v ok=%v, want Lorelei", second, repaired, ok)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: redProgressLeagueLoreleiDefeated, Complete: true})
	policy.success()
	if got, repaired, ok := policy.prerequisiteRecovery(obs, redProgressionObjectives(obs), adapter); ok {
		t.Fatalf("completed reset chain retained recovery %+v via %+v", got, repaired)
	}
}

func TestSecretKeyValidationReportsStructuredStoryPrerequisites(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	obs := Observation{
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
		},
	}
	err := adapter.Validate(Objective{Kind: KindProgress, Progress: ProgressSecretKeyOwned}, obs)
	var missing *gameruntime.PrerequisiteMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("Secret Key validation error = %v, want typed prerequisite error", err)
	}
	want := []gameruntime.Prerequisite{
		gameruntime.ProgressionPrerequisite(redProgressSilphRescueComplete),
		gameruntime.ProgressionPrerequisite(redProgressMarshBadge),
	}
	if !reflect.DeepEqual(missing.Missing, want) {
		t.Fatalf("Secret Key prerequisites = %+v, want %+v", missing.Missing, want)
	}
}

func TestViridianGymOpenRecoversThroughEarthProgressionOwner(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	obs := Observation{
		Story: ProgressState{
			{ID: redProgressVolcanoBadge, Complete: true},
		},
	}
	requested := Objective{Kind: KindGym, Place: "viridian gym"}
	err := adapter.Validate(requested, obs)
	var missing *gameruntime.PrerequisiteMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("Viridian Gym validation error = %v, want typed prerequisite error", err)
	}
	want := []gameruntime.Prerequisite{
		gameruntime.ProgressionPrerequisite(ProgressViridianGymOpen),
	}
	if !reflect.DeepEqual(missing.Missing, want) {
		t.Fatalf("Viridian Gym prerequisites = %+v, want %+v", missing.Missing, want)
	}

	failure := normalizeRedFailure(gameruntime.FailurePhaseValidation, err, obs)
	policy := newRunFailurePolicy(3)
	policy.record(ObjectiveResult{
		Objective: requested,
		Outcome:   OutcomeBlocked,
		Failure:   &failure,
		Final:     obs,
	})
	got, repaired, ok := policy.prerequisiteRecovery(obs, redProgressionObjectives(obs), adapter)
	if !ok {
		t.Fatal("Viridian Gym open prerequisite did not select Earth progression")
	}
	if got.Kind != KindProgress || got.Progress != redProgressEarthBadge {
		t.Fatalf("Viridian recovery = %+v, want Earth Badge progression", got)
	}
	if !reflect.DeepEqual(repaired, want) {
		t.Fatalf("Viridian repaired prerequisites = %+v, want %+v", repaired, want)
	}
}

func TestVictoryRoadOrderingIsStructuredAtAdapterBoundary(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	obs := Observation{
		Story: ProgressState{
			{ID: redProgressEarthBadge, Complete: true},
			{ID: ProgressRoute22RivalResolved, Complete: true},
		},
	}
	err := adapter.Validate(Objective{Kind: KindProgress, Progress: redProgressVictoryRoadCleared}, obs)
	var missing *gameruntime.PrerequisiteMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("Victory Road validation error = %v, want typed prerequisite error", err)
	}
	want := []gameruntime.Prerequisite{
		gameruntime.ProgressionPrerequisite(ProgressRoute23BadgeChecks),
	}
	if !reflect.DeepEqual(missing.Missing, want) {
		t.Fatalf("Victory Road prerequisites = %+v, want %+v", missing.Missing, want)
	}
}

func TestVictoryRoadValidationReportsStructuredFieldRequirements(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	obs := Observation{
		Story: ProgressState{{ID: ProgressRoute23BadgeChecks, Complete: true}},
		FieldCapabilities: []FieldCapability{
			{Name: "surf", BadgeOwned: true, HMOwned: true},
			{Name: "strength", BadgeOwned: true, HMOwned: true},
		},
	}
	err := adapter.Validate(Objective{Kind: KindProgress, Progress: redProgressVictoryRoadCleared}, obs)
	var missing *gameruntime.PrerequisiteMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("Victory Road field validation error = %v, want typed prerequisite error", err)
	}
	want := []gameruntime.Prerequisite{
		gameruntime.FieldCapabilityPrerequisite("surf"),
		gameruntime.FieldCapabilityPrerequisite("strength"),
	}
	if !reflect.DeepEqual(missing.Missing, want) {
		t.Fatalf("Victory Road field prerequisites = %+v, want %+v", missing.Missing, want)
	}
}

func TestCinnabarFlyPreferenceNeverBlocksSurfReadyProgression(t *testing.T) {
	adapter := &redObjectiveAdapter{}
	obs := Observation{
		Story: ProgressState{
			{ID: redProgressFuchsiaProgressionComplete, Complete: true},
			{ID: redProgressSilphRescueComplete, Complete: true},
			{ID: redProgressMarshBadge, Complete: true},
		},
		FieldCapabilities: []FieldCapability{
			{Name: "surf", BadgeOwned: true, HMOwned: true, Learned: true, Usable: true},
			{Name: "fly", BadgeOwned: true, HMOwned: false, Learned: false, Usable: false},
		},
	}
	requirements := redProgressionFieldCapabilityRequirements(
		Objective{Kind: KindProgress, Progress: ProgressSecretKeyOwned},
		obs,
	)
	if !reflect.DeepEqual(requirements.Required, []CapabilityID{"surf"}) {
		t.Fatalf("Cinnabar required field capabilities = %v, want [surf]", requirements.Required)
	}
	if !reflect.DeepEqual(requirements.Preferred, []CapabilityID{"fly"}) {
		t.Fatalf("Cinnabar preferred field capabilities = %v, want [fly]", requirements.Preferred)
	}
	if err := adapter.Validate(Objective{Kind: KindProgress, Progress: ProgressSecretKeyOwned}, obs); err != nil {
		t.Fatalf("Surf-ready Cinnabar progression was blocked by optional Fly: %v", err)
	}
}

func TestFlyProgressionRequirementSwitchesFromCutToFlyAfterHM02(t *testing.T) {
	obj := Objective{Kind: KindProgress, Progress: redProgressFlyReady}
	before := Observation{FieldCapabilities: []FieldCapability{
		{Name: "cut", BadgeOwned: true, HMOwned: true, Usable: true},
		{Name: "fly", BadgeOwned: true, HMOwned: false},
	}}
	if got := redProgressionFieldCapabilityRequirements(obj, before).Required; !reflect.DeepEqual(got, []CapabilityID{"cut"}) {
		t.Fatalf("Fly requirements before HM02 = %v, want [cut]", got)
	}

	after := before
	after.FieldCapabilities = append([]FieldCapability(nil), before.FieldCapabilities...)
	after.FieldCapabilities[1].HMOwned = true
	if got := redProgressionFieldCapabilityRequirements(obj, after).Required; !reflect.DeepEqual(got, []CapabilityID{"fly"}) {
		t.Fatalf("Fly requirements after HM02 = %v, want [fly]", got)
	}
}
