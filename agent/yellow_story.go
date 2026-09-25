package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowstory "github.com/maestroi/pokepilot/yellow/story"
)

// yellowProgressionExecutor owns the Yellow mechanics for one progression
// goal. Like Red's, it never decides success: the objective runtime verifies
// the same-ID fact yellow/profile projects into Observation.Story.
type yellowProgressionExecutor func(m *emu.Emu, romData []byte, policy skill.MovePolicy) error

func yellowOpeningExecutor(target yellowstory.OpeningMilestone) yellowProgressionExecutor {
	return func(m *emu.Emu, romData []byte, policy skill.MovePolicy) error {
		return yellowstory.Opening(m, romData, policy, target)
	}
}

// yellowProgressionExecutors is Yellow's ProgressID registry: the story goals
// a Yellow controller can drive today. A goal that is projected but not
// registered here is not offered and does not validate, so the planner never
// selects Yellow story work nothing can execute.
// TestYellowProgressionRegistryHasVerifiers keeps every entry verifiable.
var yellowProgressionExecutors = map[ProgressID]yellowProgressionExecutor{
	yellowprofile.ProgressYellowStarterReceived:  yellowOpeningExecutor(yellowstory.MilestoneStarterReceived),
	yellowprofile.ProgressYellowLabRivalResolved: yellowOpeningExecutor(yellowstory.MilestoneLabRivalResolved),
}

// yellowSharedStoryBeats are the Gen-I story beats whose Yellow scripts run
// the same way as Red's, checked against the vendored decomps: Oak's parcel
// (ViridianMart.asm and OaksLab.asm give the parcel on mart entry and swap it
// for the Pokedex with Oak at (5,2); Yellow only adds the later old-man
// toggle) and the Boulder Badge (the same Viridian Forest route and Brock's
// gym). Yellow owns the decision to use them: the shared Gen-I executor runs
// the mechanics through the canonical memory view, the offer reuses the
// shared availability rule, and the positive verifier is Yellow's own
// projection of the same ProgressID. Beats Yellow rewrites (Mt. Moon, the
// Rocket Hideout, Pokemon Tower and Silph Co. all add Jessie & James) are
// deliberately absent and stay a typed controller-unavailable block.
var yellowSharedStoryBeats = []ProgressID{
	gen1.ProgressPokedexAcquired,
	gen1.ProgressBoulderBadge,
}

func init() {
	for _, id := range yellowSharedStoryBeats {
		shared, ok := redProgressionExecutors[id]
		if !ok {
			panic("agent: Yellow shared story beat " + string(id) + " has no Gen-I executor")
		}
		yellowProgressionExecutors[id] = yellowProgressionExecutor(shared)
	}
}

func yellowSharedStoryBeat(id ProgressID) bool {
	for _, shared := range yellowSharedStoryBeats {
		if shared == id {
			return true
		}
	}
	return false
}

func yellowProgressionKnown(id ProgressID) bool {
	_, ok := yellowProgressionExecutors[id]
	return ok
}

// yellowStarterSpecies is the one starter Yellow's opening gives.
const yellowStarterSpecies SpeciesID = "pikachu"

// executeYellowOwned runs the objective kinds whose semantics are Yellow's
// story. The Starter objective is the whole opening, through the lab rival
// battle, matching the shared Starter contract (a controllable party-holding
// boundary with the opening behind it).
func executeYellowOwned(m *emu.Emu, romData []byte, o Objective) (ObjectiveResult, error) {
	result := ObjectiveResult{Objective: o}
	var run yellowProgressionExecutor
	switch o.Kind {
	case KindStarter:
		run = yellowOpeningExecutor(yellowstory.MilestoneLabRivalResolved)
	case KindProgress:
		run = yellowProgressionExecutors[o.Progress]
	}
	if run == nil {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w", o, errYellowControllerUnavailable)
	}
	if err := run(m, romData, skill.StatAwareMove(romData)); err != nil {
		return result, fmt.Errorf("agent: %s: %w", o, err)
	}
	return result, nil
}

// validateYellowOwned checks the Yellow story vocabulary before execution.
func validateYellowOwned(o Objective) error {
	switch o.Kind {
	case KindStarter:
		if o.Species != "" && o.Species != yellowStarterSpecies {
			return fmt.Errorf("agent: %s: Yellow starter must be %s, got %q", o, yellowStarterSpecies, o.Species)
		}
	case KindProgress:
		if !yellowProgressionKnown(o.Progress) {
			return fmt.Errorf("agent: %s: unknown Yellow progression goal %q", o, o.Progress)
		}
	}
	return nil
}

// verifyYellowStarterPostcondition proves the opening's positive facts: a
// stable boundary, Pikachu in the party, and Yellow's own story flags for
// the starter and the lab rival. A Red starter index is meaningless here.
func verifyYellowStarterPostcondition(o Objective, final Observation) error {
	if !stableObjectiveBoundary(final) {
		return fmt.Errorf(
			"%w: %s ended at %s (%d,%d), controllable=%v inBattle=%v",
			ErrObjectivePostconditionUnavailable, o, final.Location, final.X, final.Y, final.Controllable, final.InBattle)
	}
	hasPikachu := false
	for _, mon := range final.Party {
		if mon.Species == yellowStarterSpecies {
			hasPikachu = true
			break
		}
	}
	if !hasPikachu {
		return fmt.Errorf("%w: %s finished but party does not contain %s",
			ErrObjectivePostconditionFailed, o, yellowStarterSpecies)
	}
	for _, id := range []ProgressID{yellowprofile.ProgressYellowStarterReceived, yellowprofile.ProgressYellowLabRivalResolved} {
		fact, ok := final.Story.Lookup(id)
		if !ok {
			return fmt.Errorf("%w: %w: %s needs %q", ErrObjectivePostconditionUnavailable, ErrProgressVerifierMissing, o, id)
		}
		if !fact.Complete {
			return fmt.Errorf("%w: %s finished but %q is false", ErrObjectivePostconditionFailed, o, id)
		}
	}
	return nil
}

// yellowProgressionObjectives offers the Yellow story goals that are
// currently meaningful. The Starter objective covers a fresh game; once
// Pikachu is in the party but the lab rival is unresolved (an interrupted
// opening), the remaining half is offered on its own.
func yellowProgressionObjectives(obs Observation) []Objective {
	var out []Objective
	if obs.Story.Has(yellowprofile.ProgressYellowStarterReceived) && !obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved) {
		out = append(out, Objective{
			Kind:     KindProgress,
			Progress: yellowprofile.ProgressYellowLabRivalResolved,
			Note:     "(finish Yellow's opening: walk toward the lab exit so the rival challenges, then battle)",
		})
	}
	for _, o := range redProgressionObjectives(obs) {
		if o.Kind == KindProgress && yellowSharedStoryBeat(o.Progress) {
			out = append(out, o)
		}
	}
	return out
}

// normalizeYellowStoryFailure classifies the Yellow story controller's typed
// errors. ok is false for errors the controller does not own.
func normalizeYellowStoryFailure(phase gameruntime.FailurePhase, err error) (gameruntime.Failure, bool) {
	switch {
	case errors.Is(err, errYellowControllerUnavailable):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassBlocked,
			Cause: "yellow_controller_unavailable", Recoverable: false,
		}, true
	case errors.Is(err, yellowstory.ErrOpeningChoicePrompt):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassChoiceRequired,
			Cause: "yellow_opening_choice_prompt", Recoverable: false,
		}, true
	case errors.Is(err, yellowstory.ErrOpeningStalled):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "yellow_opening_stalled", Recoverable: false,
		}, true
	case errors.Is(err, yellowstory.ErrOpeningUnexpectedState):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "yellow_opening_unexpected_state", Recoverable: false,
		}, true
	}
	return gameruntime.Failure{}, false
}
