package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
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
		gsprofile.ProgressZephyrBadgeEarned,
		gsprofile.ProgressTogepiEggReceived,
		gsprofile.ProgressSlowpokeWellCleared,
		gsprofile.ProgressHiveBadgeEarned,
		gsprofile.ProgressAzaleaRivalResolved,
		gsprofile.ProgressFarfetchdHerded,
		gsprofile.ProgressHM01CutAcquired,
		gsprofile.ProgressTM02HeadbuttAcquired,
		gsprofile.ProgressPlainBadgeEarned,
		gsprofile.ProgressSquirtBottleAcquired,
		gsprofile.ProgressSudowoodoCleared,
		gsprofile.ProgressTM08RockSmashAcquired,
		gsprofile.ProgressBurnedTowerCleared,
		gsprofile.ProgressFogBadgeEarned,
		gsprofile.ProgressHM03SurfAcquired:
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
	case KindUseItem:
		if _, ok := gsMachineNumberForItem(o.Item); !ok {
			return fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
		}
		return nil
	case KindTrain:
		// Only "train the lead": the Gen-II grinder levels party slot 0.
		if o.Species != "" || o.Slot != 0 || o.Intent != "" {
			return fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
		}
		return nil
	default:
		return fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
	}
}

// NormalizeBoundary is deliberately fail-closed except for the exact opening
// and early-Johto corridors this adapter owns. A retry may resume in one of
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
	if facts.GaveMysteryEggToElm && (gsFirstBadgeOwnedMap(facts.NativeMapID) || gsSecondBadgeOwnedMap(facts.NativeMapID)) {
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
		case gsprofile.ProgressTogepiEggReceived:
			err = executeGSTogepiEggPickup(a.m, a.romData)
		case gsprofile.ProgressSlowpokeWellCleared:
			err = executeGSSlowpokeWell(a.m, a.romData)
		case gsprofile.ProgressHiveBadgeEarned:
			err = executeGSBugsy(a.m, a.romData)
		case gsprofile.ProgressAzaleaRivalResolved:
			err = executeGSAzaleaRival(a.m, a.romData)
		case gsprofile.ProgressFarfetchdHerded:
			err = executeGSFarfetchd(a.m, a.romData)
		case gsprofile.ProgressHM01CutAcquired:
			err = executeGSHM01Cut(a.m, a.romData)
		case gsprofile.ProgressTM02HeadbuttAcquired:
			err = executeGSTM02Headbutt(a.m, a.romData)
		case gsprofile.ProgressPlainBadgeEarned:
			err = executeGSWhitney(a.m, a.romData)
		case gsprofile.ProgressSquirtBottleAcquired:
			err = executeGSSquirtBottle(a.m, a.romData)
		case gsprofile.ProgressSudowoodoCleared:
			err = executeGSSudowoodo(a.m, a.romData)
		case gsprofile.ProgressTM08RockSmashAcquired:
			err = executeGSTM08RockSmash(a.m, a.romData)
		case gsprofile.ProgressBurnedTowerCleared:
			err = executeGSBurnedTower(a.m, a.romData)
		case gsprofile.ProgressFogBadgeEarned:
			err = executeGSMorty(a.m, a.romData)
		case gsprofile.ProgressHM03SurfAcquired:
			err = executeGSHM03Surf(a.m, a.romData)
		default:
			result.Outcome = OutcomeBlocked
			return result, fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
		}
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindTrain:
		if err := executeGSTrain(a.m, a.romData, o.Level); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindUseItem:
		if err := executeGSTeachMachine(a.m, a.romData, o); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		// TeachNativeMachine returns only after the carrier knows the move.
		result.ItemEffectVerified = true
		return result, nil
	default:
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w", o, errGSControllerUnavailable)
	}
}

const gsFirstBadgeObjectiveFrameBudget uint64 = 1_500_000

func gsObjectiveFrameBudget(o Objective) uint64 {
	if o.Kind == KindTrain {
		return gsFirstBadgeObjectiveFrameBudget
	}
	if o.Kind == KindProgress {
		switch o.Progress {
		case gsprofile.ProgressSproutTowerCleared, gsprofile.ProgressZephyrBadgeEarned,
			gsprofile.ProgressSlowpokeWellCleared, gsprofile.ProgressHiveBadgeEarned,
			gsprofile.ProgressAzaleaRivalResolved, gsprofile.ProgressFarfetchdHerded,
			gsprofile.ProgressTM02HeadbuttAcquired, gsprofile.ProgressPlainBadgeEarned,
			gsprofile.ProgressSudowoodoCleared, gsprofile.ProgressTM08RockSmashAcquired,
			gsprofile.ProgressBurnedTowerCleared, gsprofile.ProgressFogBadgeEarned,
			gsprofile.ProgressHM03SurfAcquired:
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
	if o.Kind != KindStarter && o.Kind != KindTrain && o.Kind != KindUseItem && !(o.Kind == KindProgress && gsSupportedProgress(o.Progress)) {
		return fmt.Errorf("%w: %s has no Gold/Silver verifier yet", ErrObjectivePostconditionUnavailable, o)
	}
	_, err := verifyObjectivePostcondition(o, initial, final, result)
	return err
}

func normalizeGSNavigationFailure(phase gameruntime.FailurePhase, cause string, final Observation) gameruntime.Failure {
	failure := gameruntime.Failure{
		Phase: phase, Cause: cause,
		Class:       gameruntime.FailureClassControllerUncertain,
		Recoverable: false,
	}
	if stableObjectiveBoundary(final) {
		failure.Class = gameruntime.FailureClassBlocked
		failure.Recoverable = true
	}
	return failure
}

func (a *gsObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, final Observation) gameruntime.Failure {
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
	case errors.Is(err, errGSSecondBadgeStalled):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "gen2_second_badge_stalled", Recoverable: false,
		}
	case errors.Is(err, errGSSecondBadgeUnexpectedState):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassControllerUncertain,
			Cause: "gen2_second_badge_unexpected_state", Recoverable: false,
		}
	case errors.Is(err, skill.ErrReplanExhausted):
		// Check exhaustion before the wrapped last-leg cause. Replan errors keep
		// that lower-level identity for diagnostics, but policy must see that
		// the bounded route search itself is exhausted.
		return normalizeGSNavigationFailure(phase, "route_replan_exhausted", final)
	case errors.Is(err, skill.ErrNavigationStalled):
		return normalizeGSNavigationFailure(phase, "navigation_stalled", final)
	case errors.Is(err, world.ErrNoPath):
		return normalizeGSNavigationFailure(phase, "no_path", final)
	case errors.Is(err, world.ErrNoRoute):
		return normalizeGSNavigationFailure(phase, "no_route", final)
	case errors.Is(err, skill.ErrLegUnwalkable):
		return normalizeGSNavigationFailure(phase, "leg_unwalkable", final)
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
	out := append(gsStoryObjectives(obs), gsTrainingObjectives(obs)...)
	if obs.PartyCount > 0 && obs.Controllable && !obs.InBattle {
		out = append(out, gsMachineObjectives(a.m, a.romData)...)
	}
	return out
}

func gsStoryObjectives(obs Observation) []Objective {
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
	case !obs.Story.Has(gsprofile.ProgressTogepiEggReceived):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressTogepiEggReceived,
			Note:     "(answer Elm's post-Falkner call, meet his aide in Violet Pokemon Center, and accept the Togepi Egg that opens Route 32)",
		}}
	case !obs.Story.Has(gsprofile.ProgressSlowpokeWellCleared):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressSlowpokeWellCleared,
			Note:     "(travel Route 32 through Union Cave to Azalea, recruit Kurt, defeat Team Rocket in Slowpoke Well, and restore the Slowpoke)",
		}}
	case !obs.Story.Has(gsprofile.ProgressHiveBadgeEarned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressHiveBadgeEarned,
			Note:     "(enter Azalea Gym, defeat Bugsy, and earn the Hive Badge)",
		}}
	case !obs.Story.Has(gsprofile.ProgressAzaleaRivalResolved):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressAzaleaRivalResolved,
			Note:     "(leave Azalea toward Ilex Forest, defeat the rival, and let the post-battle scene fully reset)",
		}}
	case !obs.Story.Has(gsprofile.ProgressFarfetchdHerded):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressFarfetchdHerded,
			Note:     "(enter Ilex Forest and herd the Charcoal apprentice's Farfetchd back through the facing puzzle)",
		}}
	case !obs.Story.Has(gsprofile.ProgressHM01CutAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressHM01CutAcquired,
			Note:     "(claim HM01 Cut from the Charcoal Master after returning Farfetchd)",
		}}
	case !obs.Story.Has(gsprofile.ProgressTM02HeadbuttAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressTM02HeadbuttAcquired,
			Note:     "(teach and use Cut to cross north Ilex Forest, then receive TM02 Headbutt from the tutor)",
		}}
	case !obs.Story.Has(gsprofile.ProgressPlainBadgeEarned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressPlainBadgeEarned,
			Note:     "(continue through Route 34 to Goldenrod, defeat Whitney, settle her crying scene, and receive the Plain Badge)",
		}}
	case !obs.Story.Has(gsprofile.ProgressSquirtBottleAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressSquirtBottleAcquired,
			Note:     "(visit the Goldenrod Flower Shop after Whitney and receive the SquirtBottle)",
		}}
	case !obs.Story.Has(gsprofile.ProgressSudowoodoCleared):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressSudowoodoCleared,
			Note:     "(take the north route from Goldenrod, water the Route 36 Sudowoodo, and win the required battle)",
		}}
	case !obs.Story.Has(gsprofile.ProgressTM08RockSmashAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressTM08RockSmashAcquired,
			Note:     "(talk to the Route 36 Rock Smash guy after Sudowoodo; Burned Tower's 1F rocks need TM08)",
		}}
	case !obs.Story.Has(gsprofile.ProgressBurnedTowerCleared):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressBurnedTowerCleared,
			Note:     "(reach Ecruteak, defeat the Burned Tower rival, descend to B1F, and release the legendary beasts)",
		}}
	case !obs.Story.Has(gsprofile.ProgressFogBadgeEarned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressFogBadgeEarned,
			Note:     "(cross Ecruteak Gym's invisible floor, defeat Morty, and earn the Fog Badge)",
		}}
	case !obs.Story.Has(gsprofile.ProgressHM03SurfAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressHM03SurfAcquired,
			Note:     "(defeat all five Ecruteak Dance Theater Kimono Girls and receive HM03 Surf from the gentleman)",
		}}
	default:
		return nil
	}
}
