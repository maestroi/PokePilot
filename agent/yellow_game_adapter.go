package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
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
	if o.Kind == KindProgress && !yellowProgressionKnown(o.Progress) {
		return fmt.Errorf("agent: %s: Yellow progression goal %q is not implemented yet", o, o.Progress)
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
		if _, ok := gen1EarlyProgressionExecutor(o.Progress); ok {
			if err := executeYellowEarlyProgression(a.m, a.romData, o); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			return result, nil
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
	if yellowOwnedKind(o.Kind) {
		_, err := verifyObjectivePostcondition(o, initial, final, result)
		return err
	}
	return a.gen1.VerifyPostcondition(o, initial, final, result)
}

func (a *yellowObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, final Observation) gameruntime.Failure {
	if errors.Is(err, errYellowControllerUnavailable) {
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassBlocked,
			Cause: "yellow_controller_unavailable", Recoverable: false,
		}
	}
	return a.gen1.NormalizeFailure(phase, err, final)
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
// its map vocabulary, and the scripted Pikachu opening in place of Oak's
// three-ball choice. Yellow's story challenges are not offered until a
// Yellow progression controller owns them.
func yellowObjectiveCatalog(obs Observation) ObjectiveCatalog {
	facts := gen1CatalogFacts{Location: yellowLocationID}
	if obs.PartyCount == 0 {
		facts.Starters = []CatalogStarter{{Species: "pikachu"}}
	}
	return gen1ObjectiveCatalog(obs, facts)
}
