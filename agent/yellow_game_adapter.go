package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	yellowcontroller "github.com/maestroi/pokepilot/yellow/controller"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

var errYellowControllerUnavailable = errors.New("Pokémon Yellow objective controller is not implemented yet")

type yellowObjectiveAdapter struct {
	m       *emu.Emu
	romData []byte
}

func init() {
	registerObjectiveAdapter(yellowprofile.GameID, func(m *emu.Emu, romData []byte) ObjectiveGameAdapter {
		return &yellowObjectiveAdapter{m: m, romData: romData}
	})
	registerObjectiveCatalogProvider(yellowprofile.GameID, &yellowObjectiveAdapter{})
}

func (a *yellowObjectiveAdapter) Observe() (Observation, error) {
	return ObserveChecked(a.m, a.romData)
}

func (a *yellowObjectiveAdapter) Validate(o Objective, _ Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if o.Kind == KindStarter && o.Species != "" && o.Species != "pikachu" {
		return fmt.Errorf("agent: %s: Yellow starter must be pikachu, got %q", o, o.Species)
	}
	return nil
}

func (a *yellowObjectiveAdapter) NormalizeBoundary() error {
	obs, err := a.Observe()
	if err != nil {
		return err
	}
	if obs.Controllable && !obs.InBattle {
		return nil
	}
	return fmt.Errorf("%w: Yellow player is not at a stable controllable boundary", ErrObjectiveBoundaryDirty)
}

func (a *yellowObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	result := ObjectiveResult{Objective: o}
	switch o.Kind {
	case KindStarter:
		if err := yellowcontroller.GetPikachuStarter(a.m, a.romData); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	default:
		return result, fmt.Errorf("agent: %s: %w", o, errYellowControllerUnavailable)
	}
}

func (a *yellowObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	deadline := a.m.FrameCount() + objectiveFrameBudget
	err := a.m.WithFrameDeadline(deadline, fn)
	if errors.Is(err, emu.ErrFrameDeadline) {
		return fmt.Errorf("agent: %s: objective frame watchdog: %w", o, err)
	}
	return err
}

func (a *yellowObjectiveAdapter) SettlePostcondition(Objective) error { return nil }

func (a *yellowObjectiveAdapter) VerifyPostcondition(o Objective, initial, final Observation, result ObjectiveResult) error {
	_, err := verifyObjectivePostcondition(o, initial, final, result)
	return err
}

func (a *yellowObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, _ Observation) gameruntime.Failure {
	failure := gameruntime.Failure{
		Phase: phase, Class: gameruntime.FailureClassUnknown, Cause: "yellow_objective_failure",
	}
	if errors.Is(err, errYellowControllerUnavailable) {
		failure.Class = gameruntime.FailureClassBlocked
		failure.Cause = "yellow_controller_unavailable"
		failure.Recoverable = false
	}
	return failure
}

func (a *yellowObjectiveAdapter) CaptureFailure(Objective, error) error {
	// Red's RAM forensics decoder must never run against Yellow. Phase 5 can
	// attach Yellow-native controller evidence once those controllers exist.
	return nil
}

func (a *yellowObjectiveAdapter) ObjectiveCatalog(obs Observation) ObjectiveCatalog {
	catalog := ObjectiveCatalog{
		CurrentCenter: strings.Contains(strings.ToUpper(obs.MapName), "POKECENTER"),
	}
	if obs.PartyCount == 0 {
		catalog.Starters = []CatalogStarter{{Species: "pikachu"}}
	}
	return catalog
}
