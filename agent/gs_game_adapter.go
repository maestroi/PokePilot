package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

var (
	errGSControllerUnavailable = errors.New("Gold/Silver objective controller is not implemented for this objective yet")
	errGSBoundaryUnsafe        = errors.New("Gold/Silver objective boundary is not safe")
)

type gsObjectiveAdapter struct {
	m       *emu.Emu
	romData []byte
	gameID  gameruntime.GameID
}

func newGSObjectiveAdapter(m *emu.Emu, romData []byte, id gameruntime.GameID) *gsObjectiveAdapter {
	return &gsObjectiveAdapter{m: m, romData: romData, gameID: id}
}

func init() {
	for _, id := range []gameruntime.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		gameID := id
		registerObjectiveAdapterFactory(gameID, func(m *emu.Emu, romData []byte, _ RoutePriority) ObjectiveGameAdapter {
			return newGSObjectiveAdapter(m, romData, gameID)
		})
		registerObjectiveCatalogProvider(gameID, &gsObjectiveAdapter{gameID: gameID})
	}
}

func (a *gsObjectiveAdapter) Observe() (Observation, error) {
	return ObserveChecked(a.m, a.romData)
}

func (a *gsObjectiveAdapter) Validate(o Objective, _ Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	switch o.Kind {
	case KindStarter:
		spec, ok := gsStarterSpecFor(o.Starter)
		if !ok {
			return fmt.Errorf("agent: %s: unsupported Gold/Silver starter %d", o, o.Starter)
		}
		if o.Species != "" && gameruntime.SpeciesID(o.Species) != spec.Species {
			return fmt.Errorf("agent: %s: starter slot %s contains %s, got semantic species %q",
				o, starterName(o.Starter), spec.Species, o.Species)
		}
		return nil
	case KindProgress:
		if o.Progress != gsprofile.ProgressMysteryEggReturned {
			return fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
		}
		return nil
	default:
		return fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
	}
}

// NormalizeBoundary is deliberately fail-closed except for the exact opening
// scripts this adapter currently owns. A retry may resume in Mom's or Elm's
// script; answering those prompts belongs to ExecuteOwned, not to generic
// boundary cleanup.
func (a *gsObjectiveAdapter) NormalizeBoundary() error {
	if a.m == nil {
		return fmt.Errorf("%w: nil emulator", errGSBoundaryUnsafe)
	}
	profile, err := gsOpeningProfile(a.romData)
	if err != nil {
		return err
	}
	facts := profile.DecodeOpening(a.m)
	if facts.Controllable && !facts.InBattle {
		return nil
	}
	if facts.InBattle && facts.GotStarter && !facts.GaveMysteryEggToElm {
		return nil
	}
	if !facts.InBattle && (gsOpeningScriptMap(facts.NativeMapID) || gsErrandScriptMap(facts.NativeMapID)) {
		return nil
	}
	return fmt.Errorf("%w: map=%#04x at (%d,%d) controllable=%v battle=%v script=%v",
		errGSBoundaryUnsafe, facts.NativeMapID, facts.X, facts.Y, facts.Controllable, facts.InBattle, facts.ScriptActive)
}

func (a *gsObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	result := ObjectiveResult{Objective: o}
	switch o.Kind {
	case KindStarter:
		if err := executeGSOpening(a.m, a.romData, o.Starter); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindProgress:
		if o.Progress != gsprofile.ProgressMysteryEggReturned {
			result.Outcome = OutcomeBlocked
			return result, fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
		}
		if err := executeGSPostStarterErrand(a.m, a.romData); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	default:
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
	}
}

func (a *gsObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	if a.m == nil {
		return fmt.Errorf("agent: %s: nil emulator", o)
	}
	deadline := a.m.FrameCount() + objectiveFrameBudgetFor(o)
	err := a.m.WithFrameDeadline(deadline, fn)
	if errors.Is(err, emu.ErrFrameDeadline) {
		return fmt.Errorf("agent: %s: objective frame watchdog: %w", o, err)
	}
	return err
}

func (a *gsObjectiveAdapter) SettlePostcondition(Objective) error {
	// The opening controller itself does not return until the chosen starter is
	// present and ordinary overworld control has returned. No passive settle is
	// needed, and this hook must not send input.
	return nil
}

func (a *gsObjectiveAdapter) VerifyPostcondition(o Objective, initial, final Observation, result ObjectiveResult) error {
	if o.Kind != KindStarter && !(o.Kind == KindProgress && o.Progress == gsprofile.ProgressMysteryEggReturned) {
		return fmt.Errorf("%w: %s has no Gold/Silver verifier yet", ErrObjectivePostconditionUnavailable, o)
	}
	_, err := verifyObjectivePostcondition(o, initial, final, result)
	return err
}

func (a *gsObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, _ Observation) gameruntime.Failure {
	switch {
	case errors.Is(err, errGSControllerUnavailable):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassBlocked,
			Cause: "gen2_controller_unavailable", Recoverable: false,
		}
	case errors.Is(err, errGSBoundaryUnsafe):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassOwnershipFailure,
			Cause: "gen2_boundary_unsafe", Recoverable: false,
		}
	case errors.Is(err, errGSOpeningStalled):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "gen2_opening_stalled", Recoverable: false,
		}
	case errors.Is(err, errGSOpeningUnexpectedState):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "gen2_opening_unexpected_state", Recoverable: false,
		}
	default:
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassUnknown,
			Cause: "gen2_unknown_error", Recoverable: false,
		}
	}
}

func (a *gsObjectiveAdapter) CaptureFailure(Objective, error) error {
	// Gen-I forensic summaries decode Red RAM. Until a Gen-II forensic payload
	// exists, do not corrupt diagnostics by running that decoder on Gold/Silver.
	return nil
}

func (a *gsObjectiveAdapter) ObjectiveCatalog(obs Observation) ObjectiveCatalog {
	if obs.PartyCount != 0 {
		return ObjectiveCatalog{}
	}
	return ObjectiveCatalog{Starters: []CatalogStarter{
		{Starter: skill.StarterChikorita, Species: "chikorita"},
		{Starter: skill.StarterCyndaquil, Species: "cyndaquil"},
		{Starter: skill.StarterTotodile, Species: "totodile"},
	}}
}


func (a *gsObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	if obs.PartyCount == 0 || !obs.Story.Has(gsprofile.ProgressStarterReceived) {
		return nil
	}
	if obs.Story.Has(gsprofile.ProgressMysteryEggReturned) {
		return nil
	}
	return []Objective{{
		Kind:     KindProgress,
		Progress: gsprofile.ProgressMysteryEggReturned,
		Note:     "(visit Mr. Pokemon, receive Oak's Pokedex, resolve the Cherrygrove rival and officer name sequence, then return the Mystery Egg to Elm)",
	}}
}
