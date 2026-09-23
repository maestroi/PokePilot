package agent

import (
	"errors"
	"reflect"
	"sort"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// normalizeRedFailure is the Red-owned translation from native controller,
// world and emulator errors into the portable failure vocabulary consumed by
// generic run/recovery policy. The native error itself is intentionally kept
// by the transaction caller for adapter-owned diagnostics and forensics.
func normalizeRedFailure(phase gameruntime.FailurePhase, err error, final Observation) gameruntime.Failure {
	if err == nil {
		return gameruntime.Failure{}
	}
	out := classifyObjectiveOutcome(Objective{}, err, final)
	if phase == gameruntime.FailurePhaseInitialObservation || phase == gameruntime.FailurePhaseFinalObservation {
		out = OutcomeControllerUncertain
	}
	cause, context := failureCauseFor(err)
	return gameruntime.Failure{
		Phase:       phase,
		Class:       failureClassForOutcome(out),
		Cause:       string(cause),
		Recoverable: actionFor(out) == actionReplan,
		Context:     context,
	}
}

// classifyObjectiveOutcome remains as a Red-adapter helper because focused
// adapter tests exercise precedence directly. Generic run policy never calls
// it and never inspects concrete skill/world/emulator errors.
func classifyObjectiveOutcome(_ Objective, err error, final Observation) Outcome {
	if err == nil {
		return OutcomeCompleted
	}

	if errors.Is(err, ErrObjectiveBoundaryChoice) {
		return OutcomeChoiceRequired
	}
	var choice *skill.ErrDialogueChoice
	if errors.As(err, &choice) || errors.Is(err, skill.ErrFieldItemPrompt) {
		return OutcomeChoiceRequired
	}

	// A dirty finish dominates the error that caused it. errors.Join keeps the
	// original typed controller fault too, so this check must precede every
	// recoverable class or an unsafe boundary could be mislabeled blocked.
	if errors.Is(err, ErrObjectiveBoundaryDirty) || errors.Is(err, skill.ErrShopStabilization) {
		return OutcomeStabilizationFailed
	}

	if errors.Is(err, ErrObjectivePostconditionFailed) {
		return OutcomePostconditionFailed
	}
	if errors.Is(err, ErrObjectivePostconditionUnavailable) {
		return OutcomePostconditionUnavailable
	}

	if recoverableControllerFault(err) {
		if stableObjectiveBoundary(final) {
			return OutcomeBlocked
		}
		return OutcomeControllerUncertain
	}

	if errors.Is(err, skill.ErrBattle) || errors.Is(err, skill.ErrBattleInterrupted) {
		return OutcomeOwnershipFailure
	}

	if errors.Is(err, skill.ErrFieldItemNoEffect) {
		return OutcomePostconditionFailed
	}

	var requiredBattle *skill.RequiredBattleError
	if errors.As(err, &requiredBattle) {
		return OutcomeBlocked
	}

	if errors.Is(err, skill.ErrBlackedOut) ||
		errors.Is(err, skill.ErrCatchBlackout) ||
		errors.Is(err, skill.ErrCatchHuntExhausted) ||
		errors.Is(err, skill.ErrFishingHuntExhausted) ||
		errors.Is(err, skill.ErrFishingNoShoreline) ||
		errors.Is(err, skill.ErrFishingNoFishHere) ||
		errors.Is(err, skill.ErrTrainRetreat) ||
		errors.Is(err, skill.ErrTrainProgress) ||
		errors.Is(err, ErrTrainingInefficient) ||
		errors.Is(err, skill.ErrCantAfford) ||
		errors.Is(err, skill.ErrNotInStock) ||
		errors.Is(err, skill.ErrBagNotRisen) ||
		errors.Is(err, skill.ErrFieldRosterNoBalls) ||
		errors.Is(err, skill.ErrFieldRosterCatch) ||
		errors.Is(err, skill.ErrPCBoxFull) ||
		errors.Is(err, skill.ErrPCNoKnownCenter) ||
		errors.Is(err, skill.ErrFieldRosterPrerequisite) ||
		errors.Is(err, skill.ErrFieldRosterNoRecovery) {
		return OutcomeBlocked
	}

	var blocked *skill.ErrBlocked
	var gate *skill.ErrRouteGateClosed
	knownBlockage := errors.Is(err, world.ErrNoPath) ||
		errors.Is(err, world.ErrNoRoute) ||
		errors.Is(err, skill.ErrLegUnwalkable) ||
		errors.Is(err, skill.ErrNoDialogue) ||
		errors.Is(err, skill.ErrDialogueInterrupted) ||
		errors.Is(err, skill.ErrFieldMovePrerequisite) ||
		errors.As(err, &blocked) ||
		errors.As(err, &gate)
	if knownBlockage {
		if stableObjectiveBoundary(final) {
			return OutcomeBlocked
		}
		return OutcomeStabilizationFailed
	}

	return OutcomeUnknownFailure
}

func recoverableControllerFault(err error) bool {
	return errors.Is(err, emu.ErrFrameDeadline) ||
		errors.Is(err, skill.ErrNavigationStalled) ||
		errors.Is(err, skill.ErrReplanExhausted) ||
		errors.Is(err, skill.ErrEngagementsExhausted) ||
		errors.Is(err, skill.ErrMenuStuck) ||
		errors.Is(err, skill.ErrCutsceneTimeout) ||
		errors.Is(err, skill.ErrForcedChoiceStuck) ||
		errors.Is(err, skill.ErrPickupMenu) ||
		errors.Is(err, skill.ErrShopMenuTimeout) ||
		errors.Is(err, skill.ErrShopControllerStalled)
}

func failureCauseFor(err error) (FailureCauseID, []string) {
	if err == nil {
		return "", nil
	}

	// ErrReplanExhausted must classify before RouteBlockedError below: goto.go
	// wraps the LAST leg's error into the exhaustion message (newReplanExhaustedError),
	// and that last leg can legitimately be a genuine capability gate even
	// when every earlier re-plan in the same call failed on transient
	// leg_unwalkable/no_path facts (a live NPC parked on a one-tile connection
	// band, MEASURED on Route 13: run-1ttew0yypzkgt2p7r4bm5c61ur round 6).
	// errors.As walks the whole %w tree, so checking RouteBlockedError first
	// would mislabel that controller/pathing exhaustion as a missing badge/HM,
	// and the route-prerequisite quarantine scope then never expires because
	// the capability it is waiting on was never actually the problem.
	if errors.Is(err, skill.ErrReplanExhausted) {
		return "route_replan_exhausted", nil
	}
	var routeBlocked *world.RouteBlockedError
	if errors.As(err, &routeBlocked) {
		missing := routeBlocked.MissingCapabilities()
		ctx := make([]string, 0, len(missing))
		for _, id := range missing {
			ctx = append(ctx, string(id))
		}
		sort.Strings(ctx)
		return "route_prerequisite_missing", ctx
	}
	if errors.Is(err, ErrObjectiveBoundaryChoice) {
		return "objective_boundary_choice", nil
	}
	var choice *skill.ErrDialogueChoice
	if errors.As(err, &choice) {
		return "dialogue_choice_required", nil
	}
	if errors.Is(err, skill.ErrFieldItemPrompt) {
		return "field_item_prompt", nil
	}
	if errors.Is(err, ErrObjectivePostconditionFailed) {
		return "objective_postcondition_failed", nil
	}
	if errors.Is(err, ErrObjectivePostconditionUnavailable) {
		return "objective_postcondition_unavailable", nil
	}
	if errors.Is(err, ErrObjectiveBoundaryDirty) {
		return "objective_boundary_dirty", nil
	}
	if errors.Is(err, skill.ErrShopStabilization) {
		return "shop_stabilization_failed", nil
	}
	if errors.Is(err, skill.ErrLinkStalled) {
		return "link_stalled", nil
	}
	if errors.Is(err, emu.ErrFrameDeadline) {
		return "frame_deadline", nil
	}
	if errors.Is(err, skill.ErrNavigationStalled) {
		return "navigation_stalled", nil
	}
	if errors.Is(err, skill.ErrEngagementsExhausted) {
		return "travel_engagements_exhausted", nil
	}
	if errors.Is(err, skill.ErrShopMenuTimeout) {
		return "shop_menu_timeout", nil
	}
	if errors.Is(err, skill.ErrShopControllerStalled) {
		return "shop_controller_stalled", nil
	}
	if errors.Is(err, skill.ErrMenuStuck) {
		return "menu_stuck", nil
	}
	if errors.Is(err, skill.ErrCutsceneTimeout) {
		return "cutscene_timeout", nil
	}
	if errors.Is(err, skill.ErrForcedChoiceStuck) {
		return "forced_choice_stuck", nil
	}
	if errors.Is(err, skill.ErrPickupMenu) {
		return "pickup_menu_stuck", nil
	}
	if errors.Is(err, skill.ErrBattle) || errors.Is(err, skill.ErrBattleInterrupted) {
		return "battle_escaped_owner", nil
	}
	if errors.Is(err, skill.ErrFieldItemNoEffect) {
		return "field_item_no_effect", nil
	}
	var requiredBattle *skill.RequiredBattleError
	if errors.As(err, &requiredBattle) {
		context := []string(nil)
		if requiredBattle.Outcome.Encounter != "" {
			context = []string{requiredBattle.Outcome.Encounter}
		}
		if requiredBattle.Outcome.Result == state.ResultLost {
			return failureCauseCombatDefeat, context
		}
		return failureCauseCombatNotWon, context
	}
	if errors.Is(err, skill.ErrTrainerBlackedOut) {
		// Direct/nested skill callers may still expose the compatibility
		// sentinel without a TravelResult. Its semantic meaning is the same
		// required combat defeat as portable BattleEvidence.
		return failureCauseCombatDefeat, nil
	}
	if errors.Is(err, skill.ErrCatchBlackout) {
		return "catch_blackout", nil
	}
	if errors.Is(err, skill.ErrBlackedOut) {
		return "blacked_out", nil
	}
	if errors.Is(err, skill.ErrCatchHuntExhausted) {
		return "catch_hunt_exhausted", nil
	}
	if errors.Is(err, skill.ErrFishingHuntExhausted) {
		return "fishing_hunt_exhausted", nil
	}
	if errors.Is(err, skill.ErrFishingNoShoreline) {
		return "fishing_no_shoreline", nil
	}
	if errors.Is(err, skill.ErrFishingNoFishHere) {
		return "fishing_no_fish_here", nil
	}
	if errors.Is(err, ErrTrainingInefficient) {
		return "training_inefficient_area", nil
	}
	if errors.Is(err, skill.ErrTrainRetreat) {
		return "train_retreat", nil
	}
	if errors.Is(err, skill.ErrTrainProgress) {
		return "train_progress_shortfall", nil
	}
	if errors.Is(err, skill.ErrCantAfford) {
		return "cant_afford", nil
	}
	if errors.Is(err, skill.ErrNotInStock) {
		return "not_in_stock", nil
	}
	if errors.Is(err, skill.ErrBagNotRisen) {
		return "bag_not_risen", nil
	}
	if errors.Is(err, skill.ErrFieldRosterNoBalls) {
		return "no_pokeball", nil
	}
	if errors.Is(err, skill.ErrFieldRosterCatch) {
		return "field_roster_catch_failed", nil
	}
	if errors.Is(err, skill.ErrPCBoxFull) {
		return "pc_box_full", nil
	}
	if errors.Is(err, skill.ErrPCNoKnownCenter) {
		return "pc_no_known_center", nil
	}
	if errors.Is(err, skill.ErrFieldRosterNoRecovery) {
		return "field_roster_no_recovery", nil
	}
	if errors.Is(err, world.ErrNoPath) {
		return "no_path", nil
	}
	if errors.Is(err, world.ErrNoRoute) {
		return "no_route", nil
	}
	if errors.Is(err, skill.ErrLegUnwalkable) {
		return "leg_unwalkable", nil
	}
	if errors.Is(err, skill.ErrNoDialogue) {
		return "no_dialogue", nil
	}
	if errors.Is(err, skill.ErrDialogueInterrupted) {
		return "dialogue_interrupted", nil
	}
	if errors.Is(err, skill.ErrFieldMovePrerequisite) || errors.Is(err, skill.ErrFieldRosterPrerequisite) {
		return "field_move_prerequisite_missing", nil
	}
	var gate *skill.ErrRouteGateClosed
	if errors.As(err, &gate) {
		return "route_gate_closed", nil
	}
	var blocked *skill.ErrBlocked
	if errors.As(err, &blocked) {
		return "blocked_step", nil
	}
	if t := firstSpecificErrorType(err); t != "" {
		return FailureCauseID("type:" + t), nil
	}
	return "unknown_error", nil
}

// firstSpecificErrorType is the prose-free fallback for a failure not yet in
// the typed cause vocabulary. Standard wrapping/join/errorString
// implementations carry no stable semantic identity and are skipped.
func firstSpecificErrorType(err error) string {
	if err == nil {
		return ""
	}
	t := reflect.TypeOf(err)
	if t != nil {
		name := t.String()
		if name != "*fmt.wrapError" && name != "*errors.joinError" && name != "*errors.errorString" && name != "errors.errorString" {
			return name
		}
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range multi.Unwrap() {
			if name := firstSpecificErrorType(child); name != "" {
				return name
			}
		}
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		return firstSpecificErrorType(one.Unwrap())
	}
	return ""
}
