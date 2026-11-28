package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// gsVerifyLeagueEndingEvent is the narrow exception to the generic
// controllable-overworld postcondition: Lance's victory event is set during
// the post-battle Oak/Mary scene, and the Hall of Fame event is set during
// the ceremony. Waiting for control would drive into the credits/reset.
//
// Only the real ROM-observed event transition counts. The required prior
// victory must exist in the initial snapshot and battle must have ended;
// reaching a map, trainer dialogue or a synthesized progress fact alone
// must never count as a League clear.
func gsVerifyLeagueEndingEvent(goal game.ProgressID, initial, final Observation) error {
	if initial.GameID != final.GameID ||
		(initial.GameID != gsprofile.GoldGameID && initial.GameID != gsprofile.SilverGameID) {
		return fmt.Errorf("%w: ending transaction changed or lacks a Gold/Silver cartridge identity", ErrObjectivePostconditionUnavailable)
	}
	previous := game.ProgressID("")
	switch goal {
	case gsprofile.ProgressLeagueLanceDefeated:
		previous = gsprofile.ProgressLeagueKarenDefeated
	case gsprofile.ProgressLeagueHallOfFameEntered:
		previous = gsprofile.ProgressLeagueLanceDefeated
	default:
		return fmt.Errorf("%w: unsupported League ending event %s", ErrObjectivePostconditionUnavailable, goal)
	}
	if !initial.Story.Has(previous) {
		return fmt.Errorf("%w: League ending %s began without prerequisite %s", ErrObjectivePostconditionFailed, goal, previous)
	}
	if initial.Story.Has(goal) {
		return fmt.Errorf("%w: League ending %s was already completed before the transaction", ErrObjectivePostconditionFailed, goal)
	}
	if final.InBattle || !final.Story.Has(goal) {
		return fmt.Errorf("%w: League ending %s has no settled, real event transition", ErrObjectivePostconditionFailed, goal)
	}
	// For the transient Lance event the preceding Karen victory must not be
	// reset by re-entering the Indigo lobby in the middle of the challenge.
	// Hall of Fame is durable and can legitimately reset those fight flags.
	if goal == gsprofile.ProgressLeagueLanceDefeated && !final.Story.Has(previous) {
		return fmt.Errorf("%w: Lance event observed after Karen's victory flag reset", ErrObjectivePostconditionFailed)
	}
	return nil
}

// gsVerifyRedEndingEvent is the Red equivalent of the cartridge-observed
// Hall-of-Fame exception. The scripted disappearance flag is set after Red's
// real trainer fight and before the credits reset overworld state; requiring
// controllable movement at that point would incorrectly reject victory.
func gsVerifyRedEndingEvent(initial, final Observation) error {
	if initial.GameID != final.GameID ||
		(initial.GameID != gsprofile.GoldGameID && initial.GameID != gsprofile.SilverGameID) {
		return fmt.Errorf("%w: Red ending changed cartridge identity", ErrObjectivePostconditionUnavailable)
	}
	if !initial.Story.Has(gsprofile.ProgressMtSilverOpened) {
		return fmt.Errorf("%w: Red encounter began before Oak unlocked Mt. Silver", ErrObjectivePostconditionFailed)
	}
	if initial.Story.Has(gsprofile.ProgressMtSilverRedDefeated) {
		return fmt.Errorf("%w: Red disappearance event was already set before battle", ErrObjectivePostconditionFailed)
	}
	if final.InBattle || !final.Story.Has(gsprofile.ProgressMtSilverRedDefeated) {
		return fmt.Errorf("%w: Red ending lacked settled retail disappearance event", ErrObjectivePostconditionFailed)
	}
	return nil
}
