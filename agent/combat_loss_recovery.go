package agent

import gameruntime "github.com/maestroi/pokepilot/game"

const (
	failureCauseCombatDefeat = "combat_defeat"
	failureCauseCombatNotWon = "combat_not_won"
)

// promoteDefeatRespawnFailure converts an otherwise unknown native execution
// error into the portable combat-loss outcome when the settled observations
// prove that gameplay ended at the registered blackout respawn point.
//
// This belongs at the shared transaction boundary rather than in individual
// story skills. Battle intentionally reports ResultLost as data, and compound
// progression skills may forget to translate that result into a typed error.
// The runtime can still prove the important consequence without knowing the
// trainer, gym, map script, or concrete game error type.
func promoteDefeatRespawnFailure(result *ObjectiveResult, initial, final Observation) {
	if result == nil || result.Failure == nil {
		return
	}
	if result.Failure.Class != gameruntime.FailureClassUnknown {
		return
	}
	if !defeatRespawned(initial, final) {
		return
	}

	failure := *result.Failure
	failure.Class = gameruntime.FailureClassBlocked
	failure.Cause = failureCauseCombatDefeat
	failure.Recoverable = true
	failure.Context = nil
	result.Failure = &failure
	result.Outcome = OutcomeBlocked
	result.Cause = FailureCauseID(failureCauseCombatDefeat)
	result.CauseContext = nil
}

// defeatRespawned is deliberately semantic. It never reads a Red RAM bit.
// A defeat is inferred only at a stable registered respawn location with a
// fully restored party plus evidence that the objective actually transitioned
// there: either the blackout money penalty is visible, or the adapter's
// blackout signal accompanies a location/tile transition.
//
// The latter is corroborating evidence rather than a standalone signal. Some
// games keep a broad "battle ended" bit around briefly, so merely seeing
// BlackedOut=true must never be enough to manufacture a defeat.
func defeatRespawned(initial, final Observation) bool {
	if final.RespawnPlace == "" || final.Location == "" ||
		final.Location != final.RespawnPlace || !stableObjectiveBoundary(final) ||
		!partyFullyRecovered(final) {
		return false
	}
	if final.Money < initial.Money {
		return true
	}
	if !final.BlackedOut {
		return false
	}
	return initial.Location != final.Location || initial.X != final.X || initial.Y != final.Y
}

func partyFullyRecovered(obs Observation) bool {
	if len(obs.Party) == 0 {
		return false
	}
	for _, mon := range obs.Party {
		if mon.MaxHP == 0 || mon.HP != mon.MaxHP || mon.Status != "" {
			return false
		}
	}
	return true
}
