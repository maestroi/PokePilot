package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestYellowProgressionLeavesFreshOpeningToStarterProvider(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{GameID: yellowprofile.GameID}
	if got := a.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("fresh opening progression=%v, want starter provider ownership", got)
	}
}

func TestYellowProgressionResumesOpeningAfterStarterReceipt(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
		},
	}
	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != yellowprofile.ProgressYellowLabRivalResolved {
		t.Fatalf("opening recovery progression=%v, want lab rival resolution", got)
	}
}

func TestYellowProgressionContinuesToPokedexAfterLabRival(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
		},
	}
	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressPokedexAcquired {
		t.Fatalf("completed opening progression=%v, want Pokedex acquisition", got)
	}
}

func TestYellowProgressionKnownIsBoundedToImplementedSlice(t *testing.T) {
	for _, id := range []ProgressID{
		yellowprofile.ProgressYellowLabRivalResolved,
		gen1.ProgressPokedexAcquired,
		gen1.ProgressBoulderBadge,
		gen1.ProgressMtMoonFossilAcquired,
		yellowprofile.ProgressYellowMtMoonExitResolved,
		gen1.ProgressSSTicketAcquired,
		gen1.ProgressHM01Acquired,
		gen1.ProgressThunderBadge,
		gen1.ProgressPostSurgeLavenderReached,
		gen1.ProgressPostSurgeCeladonReady,
		gen1.ProgressRainbowBadge,
		gen1.ProgressSilphScopeAcquired,
		gen1.ProgressPokeFluteAcquired,
		gen1.ProgressFuchsiaProgressionComplete,
		gen1.ProgressSecretKeyOwned,
		gen1.ProgressVolcanoBadge,
		gen1.ProgressSaffronGateOpen,
		gen1.ProgressCardKeyOwned,
		gen1.ProgressSilphRescueComplete,
	} {
		if !yellowProgressionKnown(id) {
			t.Fatalf("%q progression must be executable", id)
		}
	}
	if yellowProgressionKnown("yellow_future_story_gate") {
		t.Fatal("unimplemented Yellow progression must fail closed")
	}
}

func TestYellowProgressionInsertsJessieJamesAfterMtMoonFossil(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			{ID: gen1.ProgressBoulderBadge, Complete: true},
		},
	}
	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressMtMoonFossilAcquired {
		t.Fatalf("post-Brock progression=%v, want Mt Moon fossil", got)
	}
	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressMtMoonFossilAcquired, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != yellowprofile.ProgressYellowMtMoonExitResolved {
		t.Fatalf("post-fossil progression=%v, want Yellow Jessie/James exit", got)
	}
	obs.Story = append(obs.Story, ProgressFact{ID: yellowprofile.ProgressYellowMtMoonExitResolved, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressSSTicketAcquired {
		t.Fatalf("post-MtMoon progression=%v, want S.S. Ticket", got)
	}
}

func TestYellowProgressionContinuesSharedBillAndHM01AfterMtMoon(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			{ID: gen1.ProgressBoulderBadge, Complete: true},
			{ID: gen1.ProgressMtMoonFossilAcquired, Complete: true},
			{ID: yellowprofile.ProgressYellowMtMoonExitResolved, Complete: true},
		},
	}
	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressSSTicketAcquired {
		t.Fatalf("post-MtMoon progression=%v, want S.S. Ticket", got)
	}
	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressSSTicketAcquired, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressHM01Acquired {
		t.Fatalf("post-Bill progression=%v, want HM01", got)
	}
	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressHM01Acquired, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindGoTo || got[0].Place != "cerulean gym" {
		t.Fatalf("post-HM01 progression=%v, want Cascade gym handoff", got)
	}
}

func TestYellowProgressionContinuesThroughSurgeLavenderCeladonAndErika(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 1,
		Badges:     []string{"Boulder", "Cascade"},
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			{ID: gen1.ProgressBoulderBadge, Complete: true},
			{ID: gen1.ProgressMtMoonFossilAcquired, Complete: true},
			{ID: yellowprofile.ProgressYellowMtMoonExitResolved, Complete: true},
			{ID: gen1.ProgressSSTicketAcquired, Complete: true},
			{ID: gen1.ProgressHM01Acquired, Complete: true},
		},
	}

	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressThunderBadge {
		t.Fatalf("post-Cascade progression=%v, want Thunder Badge", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressThunderBadge, Complete: true})
	obs.Badges = append(obs.Badges, "Thunder")
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressPostSurgeLavenderReached {
		t.Fatalf("post-Surge progression=%v, want Lavender checkpoint", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressPostSurgeLavenderReached, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressPostSurgeCeladonReady {
		t.Fatalf("post-Lavender progression=%v, want Celadon recovery", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressPostSurgeCeladonReady, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressRainbowBadge {
		t.Fatalf("post-Celadon progression=%v, want Rainbow Badge", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressRainbowBadge, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressSilphScopeAcquired {
		t.Fatalf("post-Erika progression=%v, want Silph Scope", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressSilphScopeAcquired, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressPokeFluteAcquired {
		t.Fatalf("post-Hideout progression=%v, want Poké Flute", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressPokeFluteAcquired, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressFuchsiaProgressionComplete {
		t.Fatalf("post-Flute progression=%v, want Fuchsia progression", got)
	}
}

func TestYellowSharedMiddleProgressionUsesGen1CutPrerequisites(t *testing.T) {
	a := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	obs := Observation{GameID: yellowprofile.GameID}
	for _, id := range []ProgressID{
		gen1.ProgressThunderBadge,
		gen1.ProgressPostSurgeLavenderReached,
		gen1.ProgressRainbowBadge,
	} {
		o := Objective{Kind: KindProgress, Progress: id}
		if err := a.Validate(o, obs); err == nil {
			t.Fatalf("%q validated without usable Cut", id)
		}
		withCut := obs
		withCut.FieldCapabilities = []FieldCapability{{Name: "cut", Usable: true, HMOwned: true, BadgeOwned: true}}
		if err := a.Validate(o, withCut); err != nil {
			t.Fatalf("%q rejected with usable Cut: %v", id, err)
		}
	}
}

func TestYellowPostFuchsiaPreparesSurfThenRunsCinnabarStages(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 3,
		Badges:     []string{"Boulder", "Cascade", "Thunder", "Rainbow", "Soul"},
		Location:   PlaceID(yellowLocationID(yellowprofile.GameID, 0x07)),
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			{ID: gen1.ProgressBoulderBadge, Complete: true},
			{ID: gen1.ProgressMtMoonFossilAcquired, Complete: true},
			{ID: yellowprofile.ProgressYellowMtMoonExitResolved, Complete: true},
			{ID: gen1.ProgressSSTicketAcquired, Complete: true},
			{ID: gen1.ProgressHM01Acquired, Complete: true},
			{ID: gen1.ProgressThunderBadge, Complete: true},
			{ID: gen1.ProgressPostSurgeLavenderReached, Complete: true},
			{ID: gen1.ProgressPostSurgeCeladonReady, Complete: true},
			{ID: gen1.ProgressRainbowBadge, Complete: true},
			{ID: gen1.ProgressSilphScopeAcquired, Complete: true},
			{ID: gen1.ProgressPokeFluteAcquired, Complete: true},
			{ID: gen1.ProgressFuchsiaProgressionComplete, Complete: true},
		},
	}

	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindRepairFieldCapability || got[0].FieldCapability != "surf" {
		t.Fatalf("post-Fuchsia progression=%v, want Surf repair", got)
	}

	// Strength is deliberately still unprepared: Fuchsia positively owns HM04,
	// but the Cinnabar route itself must not invent a Strength prerequisite.
	obs.FieldCapabilities = []FieldCapability{
		{Name: "surf", BadgeOwned: true, HMOwned: true, Learned: true, Usable: true},
		{Name: "strength", BadgeOwned: true, HMOwned: true, Learned: false, Usable: false},
	}
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != gen1.ProgressSecretKeyOwned {
		t.Fatalf("post-Surf progression=%v, want resumable Secret Key transaction", got)
	}

	// An interrupted Mansion run must keep the Secret Key objective instead of
	// being pushed back out to a Cinnabar-center travel handoff.
	obs.Location = PlaceID(yellowLocationID(yellowprofile.GameID, 0xd8))
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressSecretKeyOwned {
		t.Fatalf("Mansion resume progression=%v, want Secret Key", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressSecretKeyOwned, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != gen1.ProgressVolcanoBadge {
		t.Fatalf("post-Secret-Key progression=%v, want Volcano Badge", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressVolcanoBadge, Complete: true})
	obs.FieldCapabilities = nil // Surf repair must not gate the unrelated Saffron leg once Cinnabar is complete.
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != gen1.ProgressSaffronGateOpen {
		t.Fatalf("post-Blaine progression=%v, want Saffron gate", got)
	}
}

func TestYellowFuchsiaUsesSharedGen1Executor(t *testing.T) {
	a := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	o := Objective{Kind: KindProgress, Progress: gen1.ProgressFuchsiaProgressionComplete}
	if err := a.Validate(o, Observation{GameID: yellowprofile.GameID}); err != nil {
		t.Fatalf("shared Fuchsia progression rejected on Yellow: %v", err)
	}
}

func TestYellowCinnabarValidationDoesNotRequireSilphOrMarsh(t *testing.T) {
	a := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	obs := Observation{
		GameID: yellowprofile.GameID,
		Story: ProgressState{
			{ID: gen1.ProgressFuchsiaProgressionComplete, Complete: true},
		},
		FieldCapabilities: []FieldCapability{
			{Name: "surf", BadgeOwned: true, HMOwned: true, Learned: true, Usable: true},
		},
	}
	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressSecretKeyOwned}, obs); err != nil {
		t.Fatalf("Yellow Secret Key should need Fuchsia + Surf, not Silph/Marsh: %v", err)
	}

	withoutSurf := obs
	withoutSurf.FieldCapabilities = nil
	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressSecretKeyOwned}, withoutSurf); err == nil {
		t.Fatal("Yellow Secret Key validation accepted missing Surf")
	}

	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressVolcanoBadge}, obs); err == nil {
		t.Fatal("Yellow Blaine validation accepted missing Secret Key")
	}
	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressSecretKeyOwned, Complete: true})
	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressVolcanoBadge}, obs); err != nil {
		t.Fatalf("Yellow Blaine rejected with Secret Key: %v", err)
	}
}

func TestYellowCinnabarUsesSharedGen1Executors(t *testing.T) {
	for _, id := range []ProgressID{gen1.ProgressSecretKeyOwned, gen1.ProgressVolcanoBadge} {
		if !yellowSharedStoryBeat(id) {
			t.Fatalf("%q must dispatch through the shared Gen-I executor", id)
		}
		if !yellowProgressionKnown(id) {
			t.Fatalf("%q must be registered as executable Yellow progression", id)
		}
	}
}

func TestYellowPostBlaineContinuesThroughSaffronSilphAndSabrina(t *testing.T) {
	a := &yellowObjectiveAdapter{}
	obs := Observation{
		GameID:     yellowprofile.GameID,
		PartyCount: 3,
		Badges:     []string{"Boulder", "Cascade", "Thunder", "Rainbow", "Soul", "Volcano"},
		Story: ProgressState{
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			{ID: gen1.ProgressBoulderBadge, Complete: true},
			{ID: gen1.ProgressMtMoonFossilAcquired, Complete: true},
			{ID: yellowprofile.ProgressYellowMtMoonExitResolved, Complete: true},
			{ID: gen1.ProgressSSTicketAcquired, Complete: true},
			{ID: gen1.ProgressHM01Acquired, Complete: true},
			{ID: gen1.ProgressThunderBadge, Complete: true},
			{ID: gen1.ProgressPostSurgeLavenderReached, Complete: true},
			{ID: gen1.ProgressPostSurgeCeladonReady, Complete: true},
			{ID: gen1.ProgressRainbowBadge, Complete: true},
			{ID: gen1.ProgressSilphScopeAcquired, Complete: true},
			{ID: gen1.ProgressPokeFluteAcquired, Complete: true},
			{ID: gen1.ProgressFuchsiaProgressionComplete, Complete: true},
			{ID: gen1.ProgressSecretKeyOwned, Complete: true},
			{ID: gen1.ProgressVolcanoBadge, Complete: true},
		},
		FieldCapabilities: []FieldCapability{
			{Name: "surf", BadgeOwned: true, HMOwned: true, Learned: true, Usable: true},
		},
	}

	got := a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindProgress || got[0].Progress != gen1.ProgressSaffronGateOpen {
		t.Fatalf("post-Blaine progression=%v, want Saffron gate", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressSaffronGateOpen, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressCardKeyOwned {
		t.Fatalf("post-Saffron-gate progression=%v, want Card Key", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressCardKeyOwned, Complete: true})
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Progress != gen1.ProgressSilphRescueComplete {
		t.Fatalf("post-Card-Key progression=%v, want Silph rescue", got)
	}

	obs.Story = append(obs.Story,
		ProgressFact{ID: gen1.ProgressSilphRescueComplete, Complete: true},
		ProgressFact{ID: yellowprofile.ProgressYellowSilphJessieJamesDefeated, Complete: true},
	)
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindGoTo || got[0].Place != "saffron gym" {
		t.Fatalf("post-Silph progression=%v, want Saffron Gym travel", got)
	}

	obs.Location = PlaceID("saffron gym")
	got = a.ProgressionObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindGym || got[0].Place != "saffron gym" {
		t.Fatalf("inside Saffron Gym progression=%v, want Sabrina gym objective", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressMarshBadge, Complete: true})
	if got = a.ProgressionObjectives(obs); len(got) != 0 {
		t.Fatalf("post-Marsh Saffron slice should stop cleanly: %v", got)
	}
}

func TestYellowSaffronValidationUsesYellowOrdering(t *testing.T) {
	a := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	obs := Observation{GameID: yellowprofile.GameID}

	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressSaffronGateOpen}, obs); err != nil {
		t.Fatalf("Saffron gate should have no Yellow story prerequisite: %v", err)
	}
	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressCardKeyOwned}, obs); err == nil {
		t.Fatal("Card Key validated before Saffron gate")
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressSaffronGateOpen, Complete: true})
	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressCardKeyOwned}, obs); err != nil {
		t.Fatalf("Card Key rejected after Saffron gate: %v", err)
	}
	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressSilphRescueComplete}, obs); err == nil {
		t.Fatal("Silph rescue validated before Card Key")
	}

	obs.Story = append(obs.Story, ProgressFact{ID: gen1.ProgressCardKeyOwned, Complete: true})
	if err := a.Validate(Objective{Kind: KindProgress, Progress: gen1.ProgressSilphRescueComplete}, obs); err != nil {
		t.Fatalf("Silph rescue rejected after gate + Card Key: %v", err)
	}
}

func TestYellowSilphRescueRequiresJessieJamesPostcondition(t *testing.T) {
	a := newYellowObjectiveAdapter(nil, nil, RoutePriorityConservative)
	o := Objective{Kind: KindProgress, Progress: gen1.ProgressSilphRescueComplete}
	initial := Observation{GameID: yellowprofile.GameID, Controllable: true}
	final := Observation{
		GameID:       yellowprofile.GameID,
		Controllable: true,
		Story: ProgressState{
			{ID: gen1.ProgressSilphRescueComplete, Complete: true},
		},
	}

	if err := a.VerifyPostcondition(o, initial, final, ObjectiveResult{Objective: o}); err == nil {
		t.Fatal("Silph rescue succeeded without Yellow Jessie/James completion")
	}

	final.Story = append(final.Story, ProgressFact{
		ID:       yellowprofile.ProgressYellowSilphJessieJamesDefeated,
		Complete: true,
	})
	if err := a.VerifyPostcondition(o, initial, final, ObjectiveResult{Objective: o}); err != nil {
		t.Fatalf("Silph rescue rejected with Yellow Jessie/James completion: %v", err)
	}
}
