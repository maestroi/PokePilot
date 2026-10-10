package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

var errYellowControllerUnavailable = errors.New("Pokémon Yellow objective controller is not implemented yet")

// yellowObjectiveAdapter runs Yellow objectives. The verbs the shared Gen-I
// engine owns (travel, talk, trainers, gyms, healing, training, capture,
// items, shopping, field-move repair) execute through that engine: Yellow's
// cartridge binds the canonical memory view and its ROM tables, so the same
// controllers drive it. The opening and story progression are Yellow's own
// (Pikachu, Jessie & James, a different rival) and stay Yellow-owned; until a
// Yellow controller exists they fail as a typed, non-recoverable block rather
// than replaying Red's story scripts. The first executable Yellow-owned slice
// is the resumable Pikachu opening through the lab-rival boundary.
type yellowObjectiveAdapter struct {
	m       *emu.Emu
	romData []byte
	gen1    *redObjectiveAdapter
}

func newYellowObjectiveAdapter(m *emu.Emu, romData []byte, priority RoutePriority) *yellowObjectiveAdapter {
	gen1 := newRedObjectiveAdapterWithRoutePriority(m, romData, priority)
	gen1.gameID = yellowprofile.GameID
	return &yellowObjectiveAdapter{m: m, romData: romData, gen1: gen1}
}

func init() {
	registerObjectiveAdapterFactory(yellowprofile.GameID, func(m *emu.Emu, romData []byte, priority RoutePriority) ObjectiveGameAdapter {
		return newYellowObjectiveAdapter(m, romData, priority)
	})
	registerObjectiveCatalogProvider(yellowprofile.GameID, &yellowObjectiveAdapter{})
}

// yellowOwnedKind reports the objective kinds whose semantics are Yellow's
// story rather than the shared Gen-I engine's.
func yellowOwnedKind(kind Kind) bool {
	return kind == KindStarter || kind == KindProgress
}

// Observe is Yellow's own observation: the profile's Yellow story projection
// must not be replaced by Red's story decoder (redObjectiveAdapter.Observe).
func (a *yellowObjectiveAdapter) Observe() (Observation, error) {
	return ObserveChecked(a.m, a.romData)
}

func (a *yellowObjectiveAdapter) Validate(o Objective, obs Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if o.Kind == KindStarter && o.Species != "" && o.Species != "pikachu" {
		return fmt.Errorf("agent: %s: Yellow starter must be pikachu, got %q", o, o.Species)
	}
	if o.Kind == KindProgress {
		if !yellowProgressionKnown(o.Progress) {
			return fmt.Errorf("agent: %s: Yellow progression goal %q is not implemented yet", o, o.Progress)
		}
		// Shared Kanto transactions use the same semantic prerequisite contract
		// as Red/Blue (Cut for Surge/Rock Tunnel/Erika, etc.). Yellow-specific
		// story facts remain locally validated above.
		if yellowSharedStoryBeat(o.Progress) {
			if handled, err := yellowSharedProgressionPrerequisites(o.Progress, obs); handled {
				return err
			}
			return a.gen1.Validate(o, obs)
		}
	}
	if yellowOwnedKind(o.Kind) {
		return nil
	}
	return a.gen1.Validate(o, obs)
}

func (a *yellowObjectiveAdapter) NormalizeBoundary() error {
	return a.gen1.NormalizeBoundary()
}

// ObserveBattleTurns implements BattleTurnObservingAdapter.
func (a *yellowObjectiveAdapter) ObserveBattleTurns(observer BattleTurnObserver) {
	a.gen1.ObserveBattleTurns(observer)
}

// ControlBattleMoves implements BattleMoveControllingAdapter through the
// shared Gen-I execution adapter.
func (a *yellowObjectiveAdapter) ControlBattleMoves(controller BattleMoveController) {
	a.gen1.ControlBattleMoves(controller)
}

func (a *yellowObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	result := ObjectiveResult{Objective: o}
	switch o.Kind {
	case KindStarter:
		if err := executeYellowOpening(a.m, a.romData); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindProgress:
		if o.Progress == yellowprofile.ProgressYellowLabRivalResolved {
			if err := executeYellowOpening(a.m, a.romData); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			return result, nil
		}
		if o.Progress == yellowprofile.ProgressYellowMtMoonExitResolved {
			if err := executeScriptedProgressTrigger(a.m, a.romData, yellowMtMoonExitTrigger()); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			return result, nil
		}
		if yellowSharedStoryBeat(o.Progress) {
			return a.gen1.ExecuteOwned(o)
		}
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w", o, errYellowControllerUnavailable)
	default:
		return a.gen1.ExecuteOwned(o)
	}
}

func (a *yellowObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	return a.gen1.WithinObjectiveBudget(o, fn)
}

func (a *yellowObjectiveAdapter) SettlePostcondition(o Objective) error {
	return a.gen1.SettlePostcondition(o)
}

func (a *yellowObjectiveAdapter) VerifyPostcondition(o Objective, initial, final Observation, result ObjectiveResult) error {
	if o.Kind == KindStarter {
		if _, err := verifyObjectivePostcondition(o, initial, final, result); err != nil {
			return err
		}
		for _, id := range []ProgressID{
			yellowprofile.ProgressYellowStarterReceived,
			yellowprofile.ProgressYellowLabRivalResolved,
		} {
			fact, ok := final.Story.Lookup(id)
			if !ok {
				return fmt.Errorf("%w: %s needs Yellow story verifier %q", ErrObjectivePostconditionUnavailable, o, id)
			}
			if !fact.Complete {
				return fmt.Errorf("%w: %s finished before Yellow story fact %q completed", ErrObjectivePostconditionFailed, o, id)
			}
		}
		return nil
	}
	if yellowOwnedKind(o.Kind) {
		if _, err := verifyObjectivePostcondition(o, initial, final, result); err != nil {
			return err
		}
		var required ProgressID
		switch o.Progress {
		case gen1.ProgressSilphScopeAcquired:
			required = yellowprofile.ProgressYellowRocketJessieJamesDefeated
		case gen1.ProgressPokeFluteAcquired:
			required = yellowprofile.ProgressYellowTowerJessieJamesDefeated
		case gen1.ProgressSilphRescueComplete:
			required = yellowprofile.ProgressYellowSilphJessieJamesDefeated
		}
		if required != "" {
			fact, ok := final.Story.Lookup(required)
			if !ok {
				return fmt.Errorf("%w: %s needs Yellow story verifier %q", ErrObjectivePostconditionUnavailable, o, required)
			}
			if !fact.Complete {
				return fmt.Errorf("%w: %s finished before Yellow story fact %q completed", ErrObjectivePostconditionFailed, o, required)
			}
		}
		return nil
	}
	return a.gen1.VerifyPostcondition(o, initial, final, result)
}

func (a *yellowObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, final Observation) gameruntime.Failure {
	switch {
	case errors.Is(err, errYellowControllerUnavailable):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassBlocked,
			Cause: "yellow_controller_unavailable", Recoverable: false,
		}
	case errors.Is(err, errYellowOpeningChoiceRequired):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassChoiceRequired,
			Cause: "yellow_opening_choice_required", Recoverable: false,
		}
	case errors.Is(err, errYellowOpeningStalled):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "yellow_opening_stalled", Recoverable: false,
		}
	case errors.Is(err, errYellowOpeningUnexpectedState):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "yellow_opening_unexpected_state", Recoverable: false,
		}
	default:
		return a.gen1.NormalizeFailure(phase, err, final)
	}
}

func (a *yellowObjectiveAdapter) CaptureFailure(o Objective, err error) error {
	// Forensic dumps record native RAM, and the decoded summary reads the
	// canonical view, so the shared Gen-I capture is safe on Yellow.
	return a.gen1.CaptureFailure(o, err)
}

func (a *yellowObjectiveAdapter) ObjectiveCatalog(obs Observation) ObjectiveCatalog {
	return yellowObjectiveCatalog(obs)
}

// yellowObjectiveCatalog is the shared Gen-I catalog with Yellow's own facts:
// its map vocabulary and the scripted Pikachu opening in place of Oak's
// three-ball choice. Challenge readiness is read from Yellow's own ROM teams
// by the shared observation (gen1ChallengeProfiles).
func yellowObjectiveCatalog(obs Observation) ObjectiveCatalog {
	facts := gen1CatalogFacts{Location: yellowLocationID}
	if obs.PartyCount == 0 {
		facts.Starters = []CatalogStarter{{Species: "pikachu"}}
	}
	return gen1ObjectiveCatalog(obs, facts)
}
