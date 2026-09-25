package agent

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

func TestActionForObjectiveOutcome(t *testing.T) {
	cases := []struct {
		out  Outcome
		want runAction
	}{
		{OutcomeCompleted, actionContinue},
		{OutcomeBlocked, actionReplan},
		{OutcomeChoiceRequired, actionChoice},
		{OutcomeStabilizationFailed, actionStop},
		{OutcomeOwnershipFailure, actionStop},
		{OutcomeControllerUncertain, actionStop},
		{OutcomePostconditionFailed, actionReplan},
		{OutcomePostconditionUnavailable, actionStop},
		{OutcomeUnknownFailure, actionStop},
		{Outcome("future-value"), actionStop},
	}
	for _, tc := range cases {
		if got := actionFor(tc.out); got != tc.want {
			t.Errorf("actionFor(%q) = %d, want %d", tc.out, got, tc.want)
		}
	}
}

func TestClassifyObjectiveOutcomePrecedence(t *testing.T) {
	clean := Observation{Controllable: true}
	o := Objective{Kind: KindGoTo, Place: "pewter city"}

	lastLeg := fmt.Errorf("north edge: %w", skill.ErrLegUnwalkable)
	replan := fmt.Errorf("%w: %w", skill.ErrReplanExhausted, lastLeg)
	if got := classifyObjectiveOutcome(o, replan, clean); got != OutcomeBlocked {
		t.Fatalf("replan exhaustion = %q, want blocked", got)
	}

	joinedChoice := errors.Join(world.ErrNoPath, ErrObjectiveBoundaryChoice)
	if got := classifyObjectiveOutcome(o, joinedChoice, clean); got != OutcomeChoiceRequired {
		t.Fatalf("joined choice = %q, want choice_required", got)
	}

	joinedController := errors.Join(skill.ErrMenuStuck, ErrObjectiveBoundaryDirty)
	if got := classifyObjectiveOutcome(o, joinedController, clean); got != OutcomeStabilizationFailed {
		t.Fatalf("controller + dirty boundary = %q, want stabilization_failed", got)
	}
	joinedBlocked := errors.Join(world.ErrNoPath, ErrObjectiveBoundaryDirty)
	if got := classifyObjectiveOutcome(o, joinedBlocked, clean); got != OutcomeStabilizationFailed {
		t.Fatalf("blocked + dirty boundary = %q, want stabilization_failed", got)
	}

	postFailed := fmt.Errorf("wrapped: %w", ErrObjectivePostconditionFailed)
	if got := classifyObjectiveOutcome(o, postFailed, clean); got != OutcomePostconditionFailed {
		t.Fatalf("postcondition failed = %q, want postcondition_failed", got)
	}
	postUnavailable := fmt.Errorf("wrapped: %w", ErrObjectivePostconditionUnavailable)
	if got := classifyObjectiveOutcome(o, postUnavailable, clean); got != OutcomePostconditionUnavailable {
		t.Fatalf("postcondition unavailable = %q, want postcondition_unavailable", got)
	}

	if got := classifyObjectiveOutcome(o, skill.ErrBattleInterrupted, clean); got != OutcomeOwnershipFailure {
		t.Fatalf("raw battle interruption = %q, want ownership_failure", got)
	}
	if got := classifyObjectiveOutcome(o, skill.ErrMenuStuck, clean); got != OutcomeBlocked {
		t.Fatalf("menu stuck = %q, want blocked", got)
	}
	if got := classifyObjectiveOutcome(o, emu.ErrFrameDeadline, clean); got != OutcomeBlocked {
		t.Fatalf("frame deadline = %q, want blocked", got)
	}
}

func TestClassifyObjectiveOutcomeKnownBlockageRequiresStableOverworld(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "pewter city"}
	if got := classifyObjectiveOutcome(o, world.ErrNoPath, Observation{Controllable: true}); got != OutcomeBlocked {
		t.Fatalf("clean no-path = %q, want blocked", got)
	}
	if got := classifyObjectiveOutcome(o, world.ErrNoPath, Observation{Controllable: false}); got != OutcomeStabilizationFailed {
		t.Fatalf("dirty no-path = %q, want stabilization_failed", got)
	}
	if got := classifyObjectiveOutcome(o, world.ErrNoPath, Observation{Controllable: true, InBattle: true}); got != OutcomeStabilizationFailed {
		t.Fatalf("battle no-path = %q, want stabilization_failed", got)
	}
}

func TestClassifyObjectiveOutcomeGameplayRecovery(t *testing.T) {
	clean := Observation{Controllable: true}
	o := Objective{Kind: KindGoTo, Place: "pewter city"}
	for _, err := range []error{
		skill.ErrBlackedOut,
		skill.ErrCatchBlackout,
		skill.ErrCatchHuntExhausted,
		skill.ErrFishingHuntExhausted,
		skill.ErrFishingNoShoreline,
		skill.ErrFishingNoFishHere,
		skill.ErrTrainRetreat,
		skill.ErrTrainProgress,
		skill.ErrCantAfford,
		skill.ErrNotInStock,
		skill.ErrBagNotRisen,
		skill.ErrFieldRosterNoBalls,
		skill.ErrFieldRosterPrerequisite,
	} {
		if got := classifyObjectiveOutcome(o, err, clean); got != OutcomeBlocked {
			t.Errorf("%v = %q, want blocked", err, got)
		}
	}
}

func TestClassifyObjectiveOutcomeGatedPathIsBlocked(t *testing.T) {
	clean := Observation{Controllable: true}
	gate := fmt.Errorf("agent: go to route 9: %w", &skill.ErrRouteGateClosed{Text: "Oh wait there, the road's closed."})
	if got := classifyObjectiveOutcome(Objective{Kind: KindGoTo, Place: "route 9"}, gate, clean); got != OutcomeBlocked {
		t.Fatalf("closed route gate = %q, want blocked so the run replans", got)
	}
	if got := classifyObjectiveOutcome(Objective{Kind: KindGoTo, Place: "route 9"}, gate, Observation{}); got != OutcomeStabilizationFailed {
		t.Fatalf("closed route gate on a dirty boundary = %q, want stabilization_failed", got)
	}

	cut := fmt.Errorf("agent: beat the gym leader here: skill: Gym: reach LT. SURGE: skill: EnterVermilionGym: %w",
		fmt.Errorf("%w: CUT requires the Cascade Badge", skill.ErrFieldMovePrerequisite))
	if got := classifyObjectiveOutcome(Objective{Kind: KindGym, Place: "vermilion gym"}, cut, clean); got != OutcomeBlocked {
		t.Fatalf("missing field-move badge = %q, want blocked so the run replans", got)
	}

	// Farm run-1knjy11ahg0w63n4csckdmd4tz: progress thunder_badge died as
	// unknown_failure because Cut repair reported a missing Cascade badge
	// without the replan sentinel. A dirty start-menu boundary must still
	// replan — stabilization_failed would stop the run.
	roster := fmt.Errorf("skill: SurgeProgression: prepare Cut carrier: %w",
		fmt.Errorf("%w: %w: CUT badge=false HM=true", skill.ErrFieldMovePrerequisite, skill.ErrFieldRosterPrerequisite))
	if got := classifyObjectiveOutcome(Objective{Kind: KindProgress, Progress: redProgressThunderBadge}, roster, Observation{}); got != OutcomeBlocked {
		t.Fatalf("missing Cut roster prerequisite = %q, want blocked so the run replans", got)
	}
	if actionFor(OutcomeBlocked) != actionReplan {
		t.Fatal("blocked must replan")
	}
}

func TestClassifyObjectiveOutcomeDoesNotParseLegacyGameplayProse(t *testing.T) {
	clean := Observation{Controllable: true}
	legacy := []struct {
		o   Objective
		err error
	}{
		{
			Objective{Kind: KindGym},
			fmt.Errorf("agent: beat the gym leader here: lost to the gym leader (blacked out to the center)"),
		},
		{
			Objective{Kind: KindTrain, Level: 20},
			fmt.Errorf("agent: train the lead to level 20: target level 20 not reached (ended level 18 after 20 battles)"),
		},
		{
			Objective{Kind: KindCatch, Species: "pidgey"},
			fmt.Errorf("agent: catch a PIDGEY here: no PIDGEY caught (outcome out of balls, balls=5, encounters=1)"),
		},
		{
			Objective{Kind: KindCatch, Species: "pidgey"},
			fmt.Errorf("agent: catch a PIDGEY here: skill: Catch: 500 grass legs and 26 encounters without a wanted species (map 0x33)"),
		},
	}
	for _, tc := range legacy {
		if got := classifyObjectiveOutcome(tc.o, tc.err, clean); got != OutcomeUnknownFailure {
			t.Errorf("legacy prose %s / %v = %q, want unknown_failure", tc.o, tc.err, got)
		}
	}
}

func TestObjectivePostconditionGoToUsesSemanticLocation(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "room-b"}
	good := Observation{Location: o.Place, X: 2, Y: 3, Controllable: true}
	if out, err := objectivePostcondition(o, good); err != nil || out != OutcomeCompleted {
		t.Fatalf("semantic destination = %q, %v; want completed, nil", out, err)
	}

	wrong := good
	wrong.Location = "room-c"
	out, err := objectivePostcondition(o, wrong)
	if out != OutcomePostconditionFailed || !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("wrong semantic location = %q, %v; want postcondition_failed sentinel", out, err)
	}

	unreadable := good
	unreadable.Controllable = false
	out, err = objectivePostcondition(o, unreadable)
	if out != OutcomePostconditionUnavailable || !errors.Is(err, ErrObjectivePostconditionUnavailable) {
		t.Fatalf("unreadable state = %q, %v; want postcondition_unavailable sentinel", out, err)
	}

	inBattle := good
	inBattle.InBattle = true
	out, err = objectivePostcondition(o, inBattle)
	if out != OutcomePostconditionUnavailable || !errors.Is(err, ErrObjectivePostconditionUnavailable) {
		t.Fatalf("battle state = %q, %v; want postcondition_unavailable sentinel", out, err)
	}
}

func TestObjectivePostconditionCatchUsesPokedexOwned(t *testing.T) {
	o := Objective{Kind: KindCatch, Species: "pidgey"}
	owned := Observation{Controllable: true, PokedexOwned: []SpeciesID{"pidgey"}}
	if out, err := objectivePostcondition(o, owned); err != nil || out != OutcomeCompleted {
		t.Fatalf("owned catch = %q, %v; want completed", out, err)
	}

	out, err := objectivePostcondition(o, Observation{Controllable: true, Party: []PartyMon{{Species: "pidgey"}}})
	if out != OutcomePostconditionFailed || !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("party-only catch = %q, %v; want postcondition_failed", out, err)
	}
}

func TestVerifyObjectivePostconditionRepresentativeEvidence(t *testing.T) {
	cases := []struct {
		name    string
		o       Objective
		initial Observation
		final   Observation
		result  ObjectiveResult
	}{
		{
			name:   "talk",
			o:      Objective{Kind: KindTalk, X: 4, Y: 5},
			final:  Observation{Controllable: true},
			result: ObjectiveResult{InteractionPresses: 2},
		},
		{
			name:  "trainer",
			o:     Objective{Kind: KindTrainer, X: 7, Y: 8},
			final: Observation{Controllable: true, MapObjects: []MapObject{{X: 7, Y: 8, Kind: "trainer", Defeated: true}}},
		},
		{
			name:  "starter",
			o:     Objective{Kind: KindStarter, Starter: skill.StarterSquirtle},
			final: Observation{Controllable: true, Party: []PartyMon{{Species: "squirtle", Level: 5}}},
		},
		{
			name:  "train",
			o:     Objective{Kind: KindTrain, Slot: 1, Level: 20},
			final: Observation{Controllable: true, Party: []PartyMon{{Level: 25}, {Level: 20}}},
		},
		{
			name:  "heal",
			o:     Objective{Kind: KindHeal},
			final: Observation{Controllable: true, Party: []PartyMon{{HP: 30, MaxHP: 30}, {HP: 22, MaxHP: 22}}},
		},
		{
			name:    "gym",
			o:       Objective{Kind: KindGym},
			initial: Observation{Badges: []string{"boulder"}},
			final:   Observation{Controllable: true, Badges: []string{"boulder", "cascade"}},
			result:  ObjectiveResult{Battle: &BattleEvidence{Result: "won", Won: true}},
		},
		{
			name:    "pickup",
			o:       Objective{Kind: KindPickup, Item: "potion", X: 3, Y: 4},
			initial: Observation{Bag: []Item{{Name: "potion", Quantity: 1}}},
			final:   Observation{Controllable: true, Bag: []Item{{Name: "potion", Quantity: 2}}},
		},
		{
			name:   "item",
			o:      Objective{Kind: KindUseItem, Item: "potion", Slot: 0},
			final:  Observation{Controllable: true},
			result: ObjectiveResult{ItemEffectVerified: true},
		},
		{
			name:    "buy",
			o:       Objective{Kind: KindBuy, Item: "poke ball", Qty: 3},
			initial: Observation{Bag: []Item{{Name: "poke ball", Quantity: 2}}},
			final:   Observation{Controllable: true, Bag: []Item{{Name: "poke ball", Quantity: 5}}},
		},
		{
			name:  "progress",
			o:     Objective{Kind: KindProgress, Progress: ProgressID("door_unlocked")},
			final: Observation{Controllable: true, Story: ProgressState{{ID: ProgressID("door_unlocked"), Complete: true}}},
		},
		{
			name: "field capability repair",
			o:    Objective{Kind: KindRepairFieldCapability, FieldCapability: "surf"},
			final: Observation{Controllable: true, FieldCapabilities: []FieldCapability{{
				Name: "surf", BadgeOwned: true, HMOwned: true, Learned: true, Usable: true,
			}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := verifyObjectivePostcondition(tc.o, tc.initial, tc.final, tc.result)
			if err != nil || out != OutcomeCompleted {
				t.Fatalf("postcondition = %q, %v; want completed", out, err)
			}
		})
	}
}

func TestVerifyObjectivePostconditionFailsClosedWithoutEvidence(t *testing.T) {
	stable := Observation{Controllable: true}
	fieldRepair := Objective{Kind: KindRepairFieldCapability, FieldCapability: "surf"}
	out, err := verifyObjectivePostcondition(fieldRepair, Observation{}, stable, ObjectiveResult{})
	if out != OutcomePostconditionFailed || !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("field repair without usable capability = %q, %v; want postcondition_failed", out, err)
	}

	out, err = verifyObjectivePostcondition(
		Objective{Kind: KindUseItem, Item: "potion", Slot: 0},
		Observation{}, stable, ObjectiveResult{},
	)
	if out != OutcomePostconditionFailed || !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("item without effect evidence = %q, %v; want postcondition_failed", out, err)
	}

	out, err = verifyObjectivePostcondition(Objective{Kind: Kind(250)}, Observation{}, stable, ObjectiveResult{})
	if out != OutcomePostconditionUnavailable || !errors.Is(err, ErrObjectivePostconditionUnavailable) {
		t.Fatalf("unknown kind = %q, %v; want fail-closed postcondition_unavailable", out, err)
	}
	if actionFor(out) != actionStop {
		t.Fatal("missing verifier must be terminal")
	}
}

func TestClassifyObjectiveOutcomePCStorageIsBlocked(t *testing.T) {
	clean := Observation{Controllable: true}
	if got := classifyObjectiveOutcome(Objective{Kind: KindCatch, Species: "pidgey"}, skill.ErrPCBoxFull, clean); got != OutcomeBlocked {
		t.Fatalf("full box = %q, want blocked", got)
	}
	if got := classifyObjectiveOutcome(Objective{Kind: KindCatch, Species: "vulpix"}, skill.ErrPCNoKnownCenter, clean); got != OutcomeBlocked {
		t.Fatalf("no known center = %q, want blocked", got)
	}
	if got := classifyObjectiveOutcome(Objective{Kind: KindCatch, Species: "pidgey"}, skill.ErrFieldRosterNoRecovery, clean); got != OutcomeBlocked {
		t.Fatalf("no safe deposit = %q, want blocked", got)
	}
	if got := classifyObjectiveOutcome(Objective{Kind: KindRepairFieldCapability, FieldCapability: "surf"}, skill.ErrFieldRosterCatch, clean); got != OutcomeBlocked {
		t.Fatalf("field roster catch failure = %q, want blocked", got)
	}
}

func TestObjectivePostconditionNoLongerDefersUnknownKindsToExecutorNil(t *testing.T) {
	out, err := objectivePostcondition(Objective{Kind: KindUseItem, Item: "potion"}, Observation{Controllable: true})
	if out != OutcomePostconditionFailed || !errors.Is(err, ErrObjectivePostconditionFailed) {
		t.Fatalf("UseItem without evidence = %q, %v; want postcondition_failed", out, err)
	}
}

func TestClassifyObjectiveOutcomePostconditionAndPrompt(t *testing.T) {
	clean := Observation{Controllable: true}
	o := Objective{Kind: KindUseItem, Item: "potion", Slot: 0}
	if got := classifyObjectiveOutcome(o, skill.ErrFieldItemNoEffect, clean); got != OutcomePostconditionFailed {
		t.Fatalf("field item no effect = %q, want postcondition_failed", got)
	}
	if got := classifyObjectiveOutcome(o, skill.ErrFieldItemPrompt, clean); got != OutcomeChoiceRequired {
		t.Fatalf("field item prompt = %q, want choice_required", got)
	}
}

func TestClassifyObjectiveOutcomeUnknownIsTerminal(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "pewter city"}
	if got := classifyObjectiveOutcome(o, errors.New("surprise"), Observation{Controllable: true}); got != OutcomeUnknownFailure {
		t.Fatalf("unknown error = %q, want unknown_failure", got)
	}
	if actionFor(OutcomeUnknownFailure) != actionStop {
		t.Fatal("unknown failure must stop by default")
	}
}

func TestObjectiveResultHistoryText(t *testing.T) {
	if got := (ObjectiveResult{Outcome: OutcomeCompleted}).HistoryText(); got != "done" {
		t.Fatalf("completed history = %q, want done", got)
	}
	got := (ObjectiveResult{Outcome: OutcomeBlocked, Summary: "at Route 2"}).HistoryText()
	if got != "blocked: at Route 2" {
		t.Fatalf("blocked history = %q", got)
	}
}

func TestConciseObjectiveErrorIsOneLine(t *testing.T) {
	o := Objective{Kind: KindGoTo, Place: "pewter city"}
	err := errors.Join(
		fmt.Errorf("agent: %s: primary failure", o),
		fmt.Errorf("objective left invalid boundary: %w", ErrObjectiveBoundaryDirty),
	)
	got := conciseObjectiveError(o, err)
	if strings.Contains(got, "\n") {
		t.Fatalf("concise error contains newline: %q", got)
	}
	if !strings.Contains(got, "primary failure") || !strings.Contains(got, "objective left invalid boundary") {
		t.Fatalf("concise error lost joined evidence: %q", got)
	}
}
