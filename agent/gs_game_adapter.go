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
		gsprofile.ProgressHM03SurfAcquired,
		gsprofile.ProgressLighthouseSicknessExplained,
		gsprofile.ProgressSecretPotionAcquired,
		gsprofile.ProgressJasmineReturnedToGym,
		gsprofile.ProgressHM04StrengthAcquired,
		gsprofile.ProgressStormBadgeEarned,
		gsprofile.ProgressHM02FlyAcquired,
		gsprofile.ProgressMineralBadgeEarned,
		gsprofile.ProgressRedGyaradosDefeated,
		gsprofile.ProgressLanceHelpAccepted,
		gsprofile.ProgressMahoganyStairsOpened,
		gsprofile.ProgressRocketLanceHealing,
		gsprofile.ProgressRocketPasswordsLearned,
		gsprofile.ProgressRocketOfficeOpened,
		gsprofile.ProgressRocketExecutiveBeaten,
		gsprofile.ProgressRocketMurkrowPassword,
		gsprofile.ProgressRocketTransmitterDoorOpened,
		gsprofile.ProgressRocketExecutiveFBeaten,
		gsprofile.ProgressRocketHideoutCleared,
		// Experimental 7/8-badge attempts may run these not-yet-ROM-qualified
		// controllers; missing Ice Path and Clair remain unsupported.
		gsprofile.ProgressGlacierBadgeEarned,
		gsprofile.ProgressRadioTowerFakeDirectorBeaten,
		gsprofile.ProgressRadioTowerBasementUnlocked,
		gsprofile.ProgressRadioTowerCardKeyReceived,
		gsprofile.ProgressRadioTowerShutterOpened,
		gsprofile.ProgressRadioTowerExecutiveM2Beaten,
		gsprofile.ProgressRadioTowerExecutiveF1Beaten,
		gsprofile.ProgressRadioTowerTakeoverSettled,
		gsprofile.ProgressIcePathWaterfallAcquired,
		gsprofile.ProgressIcePathBouldersDropped,
		gsprofile.ProgressBlackthornGymBouldersDropped,
		gsprofile.ProgressClairDefeated,
		gsprofile.ProgressRisingBadgeEarned,
		// Experimental League approach and the first event-verified Elite Four challenge.
		gsprofile.ProgressIndigoPlateauReached,
		gsprofile.ProgressLeagueWillDefeated:
		return true
	default:
		return false
	}
}

// SupportsObjective withholds offers the Gen2 controller cannot run yet.
func (a *gsObjectiveAdapter) SupportsObjective(o Objective) bool {
	return !errors.Is(a.Validate(o, Observation{}), errGSControllerUnavailable)
}

func (a *gsObjectiveAdapter) Validate(o Objective, obs Observation) error {
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
		return fieldCapabilityPrerequisiteError(gsMissingFieldCapabilities(o, obs))
	case KindRepairFieldCapability:
		if _, ok := gsFieldMoveForCapability(o.FieldCapability); !ok {
			return fmt.Errorf("agent: %s: %w (unknown field capability)", o, errGSControllerUnavailable)
		}
		return nil
	case KindTrain:
		if o.Slot != 0 || o.Species != "" || o.Level > gsCombatRecoveryMaxLevel {
			return fmt.Errorf("agent: %s: %w (Gen2 supports bounded lead training only)", o, errGSControllerUnavailable)
		}
		return nil
	case KindHeal:
		if o.Place != "" {
			return fmt.Errorf("agent: %s: %w (Gen2 resolves nearest native Center)", o, errGSControllerUnavailable)
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
	case KindHeal:
		if err := executeGSCombatRecoveryHeal(a.m, a.romData); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindRepairFieldCapability:
		if err := executeGSRepairFieldCapability(a.m, a.romData, o.FieldCapability); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindTrain:
		if err := executeGSCombatRecoveryTrain(a.m, a.romData, o); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
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
		case gsprofile.ProgressLighthouseSicknessExplained:
			err = executeGSLighthouseSickness(a.m, a.romData)
		case gsprofile.ProgressSecretPotionAcquired:
			err = executeGSSecretPotion(a.m, a.romData)
		case gsprofile.ProgressJasmineReturnedToGym:
			err = executeGSJasmineCured(a.m, a.romData)
		case gsprofile.ProgressHM04StrengthAcquired:
			err = executeGSHM04Strength(a.m, a.romData)
		case gsprofile.ProgressStormBadgeEarned:
			err = executeGSChuck(a.m, a.romData)
		case gsprofile.ProgressHM02FlyAcquired:
			err = executeGSHM02Fly(a.m, a.romData)
		case gsprofile.ProgressMineralBadgeEarned:
			err = executeGSJasmineBadge(a.m, a.romData)
		case gsprofile.ProgressRedGyaradosDefeated:
			err = executeGSRedGyarados(a.m, a.romData)
		case gsprofile.ProgressLanceHelpAccepted:
			err = executeGSLakeOfRageLance(a.m, a.romData)
		case gsprofile.ProgressMahoganyStairsOpened:
			err = executeGSMahoganyStairs(a.m, a.romData)
		case gsprofile.ProgressRocketLanceHealing:
			err = executeGSRocketBaseLanceHeal(a.m, a.romData)
		case gsprofile.ProgressRocketPasswordsLearned:
			err = executeGSRocketPasswords(a.m, a.romData)
		case gsprofile.ProgressRocketOfficeOpened:
			err = executeGSRocketOfficeDoor(a.m, a.romData)
		case gsprofile.ProgressRocketExecutiveBeaten:
			err = executeGSRocketExecutiveM4(a.m, a.romData)
		case gsprofile.ProgressRocketMurkrowPassword:
			err = executeGSRocketMurkrow(a.m, a.romData)
		case gsprofile.ProgressRocketTransmitterDoorOpened:
			err = executeGSRocketTransmitterDoor(a.m, a.romData)
		case gsprofile.ProgressRocketExecutiveFBeaten:
			err = executeGSRocketExecutiveF(a.m, a.romData)
		case gsprofile.ProgressRocketHideoutCleared:
			err = executeGSRocketElectrodes(a.m, a.romData)
		case gsprofile.ProgressGlacierBadgeEarned:
			err = executeGSPryce(a.m, a.romData)
		case gsprofile.ProgressRadioTowerFakeDirectorBeaten:
			err = executeGSRadioTowerFakeDirector(a.m, a.romData)
		case gsprofile.ProgressRadioTowerBasementUnlocked:
			err = executeGSRadioTowerBasementDoor(a.m, a.romData)
		case gsprofile.ProgressRadioTowerCardKeyReceived:
			err = executeGSRadioTowerCardKey(a.m, a.romData)
		case gsprofile.ProgressRadioTowerShutterOpened:
			err = executeGSRadioTowerShutter(a.m, a.romData)
		case gsprofile.ProgressRadioTowerExecutiveM2Beaten:
			err = executeGSRadioTowerExecutiveM2(a.m, a.romData)
		case gsprofile.ProgressRadioTowerExecutiveF1Beaten:
			err = executeGSRadioTowerExecutiveF1(a.m, a.romData)
		case gsprofile.ProgressRadioTowerTakeoverSettled:
			err = executeGSRadioTowerFinalBoss(a.m, a.romData)
		case gsprofile.ProgressIcePathWaterfallAcquired:
			err = executeGSIcePathWaterfall(a.m, a.romData)
		case gsprofile.ProgressIcePathBouldersDropped:
			err = executeGSIcePathBoulders(a.m, a.romData)
		case gsprofile.ProgressBlackthornGymBouldersDropped:
			err = executeGSBlackthornGymBoulders(a.m, a.romData)
		case gsprofile.ProgressClairDefeated:
			err = executeGSClair(a.m, a.romData)
		case gsprofile.ProgressRisingBadgeEarned:
			err = executeGSDragonFangRising(a.m, a.romData)
		case gsprofile.ProgressIndigoPlateauReached:
			err = executeGSIndigoPlateau(a.m, a.romData)
		case gsprofile.ProgressLeagueWillDefeated:
			err = executeGSLeagueWill(a.m, a.romData)
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
	if o.Kind == KindTrain || o.Kind == KindHeal || o.Kind == KindRepairFieldCapability {
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
			gsprofile.ProgressHM03SurfAcquired,
			gsprofile.ProgressLighthouseSicknessExplained,
			gsprofile.ProgressSecretPotionAcquired,
			gsprofile.ProgressJasmineReturnedToGym,
			gsprofile.ProgressHM04StrengthAcquired,
			gsprofile.ProgressStormBadgeEarned,
			gsprofile.ProgressHM02FlyAcquired,
			gsprofile.ProgressMineralBadgeEarned,
			gsprofile.ProgressRedGyaradosDefeated,
			gsprofile.ProgressLanceHelpAccepted,
			gsprofile.ProgressMahoganyStairsOpened,
			gsprofile.ProgressRocketLanceHealing,
			gsprofile.ProgressRocketPasswordsLearned,
			gsprofile.ProgressRocketOfficeOpened,
			gsprofile.ProgressRocketExecutiveBeaten,
			gsprofile.ProgressRocketMurkrowPassword,
			gsprofile.ProgressRocketTransmitterDoorOpened,
			gsprofile.ProgressRocketExecutiveFBeaten,
			gsprofile.ProgressRocketHideoutCleared,
			gsprofile.ProgressGlacierBadgeEarned,
			gsprofile.ProgressRadioTowerFakeDirectorBeaten,
			gsprofile.ProgressRadioTowerBasementUnlocked,
			gsprofile.ProgressRadioTowerCardKeyReceived,
			gsprofile.ProgressRadioTowerShutterOpened,
			gsprofile.ProgressRadioTowerExecutiveM2Beaten,
			gsprofile.ProgressRadioTowerExecutiveF1Beaten,
			gsprofile.ProgressRadioTowerTakeoverSettled,
			gsprofile.ProgressIcePathWaterfallAcquired,
			gsprofile.ProgressIcePathBouldersDropped,
			gsprofile.ProgressBlackthornGymBouldersDropped,
			gsprofile.ProgressClairDefeated,
			gsprofile.ProgressRisingBadgeEarned,
			gsprofile.ProgressIndigoPlateauReached,
			gsprofile.ProgressLeagueWillDefeated:
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
	if o.Kind == KindHeal {
		return verifyGSCombatRecoveryHeal(final)
	}
	if o.Kind != KindStarter && o.Kind != KindTrain && o.Kind != KindRepairFieldCapability &&
		!(o.Kind == KindProgress && gsSupportedProgress(o.Progress)) {
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
	var prerequisite *gameruntime.PrerequisiteMissingError
	switch {
	case errors.As(err, &prerequisite):
		return gameruntime.Failure{
			Phase: phase, Class: gameruntime.FailureClassBlocked,
			Cause: "field_capability_prerequisite_missing", Recoverable: true,
			Prerequisites: append([]gameruntime.Prerequisite(nil), prerequisite.Missing...),
		}
	case errors.Is(err, errGSFieldCarrierUnavailable), errors.Is(err, errGSPCRefused):
		return normalizeGSNavigationFailure(phase, "gen2_field_carrier_unavailable", final)
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

func (a *gsObjectiveAdapter) ProgressionObjectives(obs Observation) (out []Objective) {
	if obs.PartyCount == 0 || !obs.Story.Has(gsprofile.ProgressStarterReceived) {
		return nil
	}
	// A combat-lost story objective is withheld by the portable combat gate.
	// Preserve that gate and append a separate executable training objective
	// rather than retrying the same losing boss or returning an empty menu.
	defer func() {
		if len(out) > 0 {
			out = append(out, gsProgressionCombatRecovery(obs, out[0])...)
		}
	}()
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
	case !obs.Story.Has(gsprofile.ProgressLighthouseSicknessExplained):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressLighthouseSicknessExplained,
			Note:     "(travel Route 38/39 to Olivine, climb the Lighthouse, and hear Jasmine's request to cure Ampharos)",
		}}
	case !obs.Story.Has(gsprofile.ProgressSecretPotionAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressSecretPotionAcquired,
			Note:     "(surf Route 40/41 to Cianwood, reach the pharmacy, and receive SecretPotion for Ampharos)",
		}}
	case !obs.Story.Has(gsprofile.ProgressJasmineReturnedToGym):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressJasmineReturnedToGym,
			Note:     "(return to Lighthouse 6F, give SecretPotion to Jasmine, and see her return to Olivine Gym)",
		}}
	case !obs.Story.Has(gsprofile.ProgressHM04StrengthAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressHM04StrengthAcquired,
			Note:     "(visit the Olivine Café sailor and claim HM04 Strength for Cianwood Gym's boulder puzzle)",
		}}
	case !obs.Story.Has(gsprofile.ProgressStormBadgeEarned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressStormBadgeEarned,
			Note:     "(surf to Cianwood, use Strength to clear the Gym boulders, defeat Chuck, and earn Storm Badge)",
		}}
	case !obs.Story.Has(gsprofile.ProgressHM02FlyAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressHM02FlyAcquired,
			Note:     "(speak to Chuck's wife outside Cianwood Gym after receiving Storm Badge and claim HM02 Fly)",
		}}
	case !obs.Story.Has(gsprofile.ProgressMineralBadgeEarned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressMineralBadgeEarned,
			Note:     "(return to Olivine Gym, defeat Jasmine, and earn the Mineral Badge)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRedGyaradosDefeated):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRedGyaradosDefeated,
			Note:     "(reach Mahogany and Lake of Rage via Route 42/43, Surf to Red Gyarados, win the battle and complete its Red Scale script)",
		}}
	case !obs.Story.Has(gsprofile.ProgressLanceHelpAccepted):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressLanceHelpAccepted,
			Note:     "(speak with Lance at Lake of Rage and agree to investigate the Team Rocket radio signal)",
		}}
	case !obs.Story.Has(gsprofile.ProgressMahoganyStairsOpened):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressMahoganyStairsOpened,
			Note:     "(meet Lance at Mahogany Mart, follow his Dragonite scene, and reveal the real Rocket Base staircase)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRocketLanceHealing):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRocketLanceHealing,
			Note:     "(descend Rocket Base B1F, survive the security cameras, and receive Lance's scripted party heal on B2F)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRocketPasswordsLearned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRocketPasswordsLearned,
			Note:     "(fight the two Rocket password grunts on B3F and learn SlowpokeTail and RaticateTail)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRocketOfficeOpened):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRocketOfficeOpened,
			Note:     "(use the two learned passwords to open Giovanni's B3F office door)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRocketExecutiveBeaten):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRocketExecutiveBeaten,
			Note:     "(defeat the Rocket executive inside Giovanni's B3F office)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRocketMurkrowPassword):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRocketMurkrowPassword,
			Note:     "(learn the transmitter password from Giovanni's Murkrow)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRocketTransmitterDoorOpened):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRocketTransmitterDoorOpened,
			Note:     "(use Murkrow's password to open the B2F radio transmitter door)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRocketExecutiveFBeaten):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRocketExecutiveFBeaten,
			Note:     "(defeat the B2F Rocket executive with Lance and settle the retail post-battle script)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRocketHideoutCleared):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRocketHideoutCleared,
			Note:     "(defeat all three B2F Electrodes and finish Lance's HM06 Whirlpool reward scene)",
		}}
	case !obs.Story.Has(gsprofile.ProgressGlacierBadgeEarned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressGlacierBadgeEarned,
			Note:     "(experimental: defeat Pryce at Mahogany Gym and earn Glacier Badge)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRadioTowerFakeDirectorBeaten):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRadioTowerFakeDirectorBeaten,
			Note:     "(experimental: defeat the fake Director at Radio Tower 5F and claim Basement Key)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRadioTowerBasementUnlocked):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRadioTowerBasementUnlocked,
			Note:     "(experimental: use Basement Key to open Goldenrod Underground)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRadioTowerCardKeyReceived):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRadioTowerCardKeyReceived,
			Note:     "(experimental: rescue the real Director through the switch puzzle and receive Card Key)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRadioTowerShutterOpened):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRadioTowerShutterOpened,
			Note:     "(experimental: open the 3F Card Key shutter)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRadioTowerExecutiveM2Beaten):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRadioTowerExecutiveM2Beaten,
			Note:     "(experimental: defeat the Radio Tower 4F male Rocket executive)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRadioTowerExecutiveF1Beaten):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRadioTowerExecutiveF1Beaten,
			Note:     "(experimental: defeat the Radio Tower 5F female Rocket executive)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRadioTowerTakeoverSettled):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRadioTowerTakeoverSettled,
			Note:     "(experimental: defeat the final Rocket boss and finish the Director's Gold/Silver wing gift)",
		}}
	case !obs.Story.Has(gsprofile.ProgressIcePathBouldersDropped):
		return []Objective{{Kind: KindProgress, Progress: gsprofile.ProgressIcePathBouldersDropped,
			Note: "(experimental: cross Ice Path to B1F and push four boulders into assigned holes)"}}
	case !obs.Story.Has(gsprofile.ProgressIcePathWaterfallAcquired):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressIcePathWaterfallAcquired,
			Note:     "(experimental: traverse Ice Path's forced ice to receive HM07 Waterfall near Blackthorn)",
		}}
	case !obs.Story.Has(gsprofile.ProgressBlackthornGymBouldersDropped):
		return []Objective{{Kind: KindProgress, Progress: gsprofile.ProgressBlackthornGymBouldersDropped,
			Note: "(experimental: cross Ice Path and drop the three Blackthorn Gym 2F boulders)"}}
	case !obs.Story.Has(gsprofile.ProgressClairDefeated):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressClairDefeated,
			Note:     "(experimental: cross Ice Path, solve Blackthorn Gym Strength holes, defeat Clair; badge is withheld)",
		}}
	case !obs.Story.Has(gsprofile.ProgressRisingBadgeEarned):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressRisingBadgeEarned,
			Note:     "(experimental: Surf/Whirlpool into Dragon's Den, claim Dragon Fang and settle Clair's Rising Badge)",
		}}
	case !obs.Story.Has(gsprofile.ProgressIndigoPlateauReached):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressIndigoPlateauReached,
			Note:     "(experimental: fly to New Bark, Surf Route 27 through Tohjo Falls, pass the badge gate, beat the rival in Victory Road and reach Indigo Plateau)",
		}}
	case !obs.Story.Has(gsprofile.ProgressLeagueWillDefeated):
		return []Objective{{
			Kind:     KindProgress,
			Progress: gsprofile.ProgressLeagueWillDefeated,
			Note:     "(experimental: heal at Indigo Plateau, enter the first Elite Four chamber and defeat Will; later rounds remain unsupported)",
		}}
	default:
		return nil
	}
}

// CombatRecoveryAvailable reports whether Gen2 can offer an executable
// preparation action after a combat loss at this story boundary. The answer is
// the same fact the recovery objective itself needs: a non-egg, below-cap lead
// and a proven wild-grind site that can still raise readiness. Before Slowpoke
// Well / Route 37 no site is proven, so a first-badge loss has no training to
// offer and the generic gate must promote it to retry
// (run-36loaokprjdtw2j8om666lswgf). When the lead is already at every open
// loss's readiness ceiling, lead XP cannot pay the remaining deficit either
// (run-3v7t9gqr8gnks2efu8haxz0j2c), so the corridor is reported unavailable
// and the same portable promote-to-retry escape fires.
func (a *gsObjectiveAdapter) CombatRecoveryAvailable(obs Observation) bool {
	if obs.PartyCount == 0 || len(obs.Party) == 0 {
		return false
	}
	lead := obs.Party[0]
	if lead.IsEgg || lead.Level == 0 || lead.Level >= gsCombatRecoveryMaxLevel {
		return false
	}
	if _, _, _, err := gsRecoveryGrindSite(obs); err != nil {
		return false
	}
	return gsLeadOnlyRecoveryCanAdvance(obs)
}
