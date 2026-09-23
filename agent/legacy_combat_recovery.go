package agent

import "strings"

// Historical combat-recovery persistence is decoded only at the checkpoint
// boundary. New runtime state must never write these modes or string keys.
const (
	legacyFailureModeGymLoss     = "gym_loss"
	legacyFailureModeGymRetry    = "gym_retry"
	legacyFailureModeTrainerLoss = "trainer_loss"

	legacyTrainerLossFailurePrefix = "trainer loss while attempting "
	legacyGymLossFailurePrefix     = "beat the gym leader at "
)

func legacyCombatFailureModes() []string {
	return []string{
		legacyFailureModeGymLoss,
		legacyFailureModeGymRetry,
		legacyFailureModeTrainerLoss,
	}
}

func legacyTrainerLossStorageKey(o Objective) string {
	return failureStorageKey(combatRecoveryObjective(o).Key(), legacyFailureModeTrainerLoss)
}

func legacyTrainerLossStringKey(o Objective) string {
	base := combatRecoveryObjective(o)
	if base.Kind == KindGym && base.Place != "" {
		return legacyTrainerLossFailurePrefix + "beat the gym leader at " + strings.ToUpper(string(base.Place))
	}
	return legacyTrainerLossFailurePrefix + base.String()
}

func legacyGymObjectiveKey(place string) ObjectiveKey {
	return Objective{Kind: KindGym, Place: PlaceID(strings.ToLower(place))}.Key()
}

func legacyGymLossStorageKey(place string) string {
	if place == "" {
		return ""
	}
	return failureStorageKey(legacyGymObjectiveKey(place), legacyFailureModeGymLoss)
}

func legacyGymRetryStorageKey(place string) string {
	if place == "" {
		return ""
	}
	return failureStorageKey(legacyGymObjectiveKey(place), legacyFailureModeGymRetry)
}

func legacyGymLossStringKey(place string) string {
	if place == "" {
		return ""
	}
	return legacyGymLossFailurePrefix + strings.ToUpper(place)
}

func clearLegacyCombatRecovery(k *Knowledge, o Objective) {
	if k == nil {
		return
	}
	delete(k.Failures, legacyTrainerLossStorageKey(o))
	delete(k.Failures, legacyTrainerLossStringKey(o))
	if o.Kind == KindGym && o.Place != "" {
		place := string(o.Place)
		delete(k.Failures, legacyGymLossStorageKey(place))
		delete(k.Failures, legacyGymLossStringKey(place))
		delete(k.Failures, legacyGymRetryStorageKey(place))
		delete(k.Failures, (Objective{Kind: KindGym}).String())
	}
}
