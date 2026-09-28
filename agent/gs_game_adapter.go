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

func gsSupportedProgress(id ProgressID) bool {
	switch id {
	case gsprofile.ProgressMysteryEggReturned,
		gsprofile.ProgressSproutTowerCleared,
		gsprofile.ProgressZephyrBadgeEarned:
		return true
	default:
		return false
	}
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
		if !gsSupportedProgress(o.Progress) {
			return fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
		}
		return nil
	default:
		return fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
	}
}

// NormalizeBoundary is deliberately fail-closed except for the exact opening
// and first-badge corridor this adapter owns. A retry may resume in one of
// those deterministic scripts; answering its prompts belongs to ExecuteOwned,
// not generic boundary cleanup.
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
	if facts.GaveMysteryEggToElm && gsFirstBadgeOwnedMap(facts.NativeMapID) {
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
		var err error
		switch o.Progress {
		case gsprofile.ProgressMysteryEggReturned:
			err = executeGSPostStarterErrand(a.m, a.romData)
		case gsprofile.ProgressSproutTowerCleared:
			err = executeGSSproutTower(a.m, a.romData)
		case gsprofile.ProgressZephyrBadgeEarned:
			err = executeGSFalkner(a.m, a.romData)
		default:
			result.Outcome = OutcomeBlocked
			return result, fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
		}
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	default:
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
	}
}

const gsFirstBadgeObjectiveFrameBudget uint64 = 1_500_000

func gsObjectiveFrameBudget(o Objective) uint64 {
	if o.Kind == KindProgress {
		switch o.Progress {
		case gsprofile.ProgressSproutTowerCleared, gsprofile.ProgressZephyrBadgeEarned:
			return gsFirstBadgeObjectiveFrameBudget
		}
	}
	return objectiveFrameBudgetFor(o)
}

func (a *gsObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	if a.m == nil {
		return fmt.Errorf("agent: %s: nil emulator", o)
	}
	deadline := a.m.FrameCount() + gsObjectiveFrameBudget(o)
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
	if o.Kind != KindStarter && !(o.Kind == KindProgress && gsSupportedProgress(o.Progress)) {
		return fmt.Errorf("%w: %s has no Gold/Silver verifier yet", ErrObjectivePostconditionUnavailable, o)
	}
	_, err := verifyObjectivePostcondition(o, initial, final, result)
	return err
}

func (a *gsObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, _ Observation) gameruntime.Failure {
	var required *skill.RequiredBattleError
	switch {
	case errors.As(err, &required):
		cause := failureCauseCombatNotWon
		if required.Outcome.Result == gameruntime.BattleLost {
			cause = failureCauseCombatDefeat
		}
		context := []string{}
		if required.Outcome.Encounter != "" {
			context = []string{required.Outcome.Encounter}
		}
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassBlocked,
			Cause: cause, Context: context, Recoverable: true,
		}
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
	case errors.Is(err, errGSFirstBadgeStalled):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "gen2_first_badge_stalled", Recoverable: false,
		}
	case errors.Is(err, errGSFirstBadgeUnexpectedState):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "gen2_first_badge_unexpected_state", Recoverable: false,
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
	switch {
	case !obs.Story.Has(gsprofile.ProgressMysteryEggReturned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressMysteryEggReturned,
			Note:     "(visit Mr. Pokemon, receive Oak's Pokedex, resolve the Cherrygrove rival and officer name sequence, then return the Mystery Egg to Elm)",
		}}
	case !obs.Story.Has(gsprofile.ProgressSproutTowerCleared):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressSproutTowerCleared,
			Note:     "(travel to Violet City, climb Sprout Tower, defeat Sage Li, and receive HM05 Flash)",
		}}
	case !obs.Story.Has(gsprofile.ProgressZephyrBadgeEarned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressZephyrBadgeEarned,
			Note:     "(enter Violet Gym, defeat the gym trainers and Falkner, and earn the Zephyr Badge)",
		}}
	default:
		return nil
	}
}
