package agent

import (
	"sort"
	"strings"
)

// legacyGymLossFailurePrefix is retained only to read v4/string-keyed
// checkpoint memory. New durable state uses generic combat_loss/combat_retry.
const legacyGymLossFailurePrefix = "beat the gym leader at "

func gymObjectiveKey(place string) ObjectiveKey {
	return Objective{Kind: KindGym, Place: PlaceID(strings.ToLower(place))}.Key()
}

// gymRetryReadyKey is read-only compatibility for pre-generic checkpoints.
// New retry-ready state uses combatRetryReadyKey.
func gymRetryReadyKey(place string) string {
	return failureStorageKey(gymObjectiveKey(place), failureModeGymRetry)
}

// gymLossFailureKey is read-only compatibility for pre-generic checkpoints.
// New losses use combatLossFailureKey.
func gymLossFailureKey(place string) string {
	if place == "" {
		return ""
	}
	return failureStorageKey(gymObjectiveKey(place), failureModeGymLoss)
}

func legacyGymLossFailureKey(place string) string {
	if place == "" {
		return ""
	}
	return legacyGymLossFailurePrefix + strings.ToUpper(place)
}

func gymLossRecorded(k *Knowledge, place string) bool {
	if k == nil || place == "" {
		return false
	}
	return combatLossRecorded(k, Objective{Kind: KindGym, Place: PlaceID(strings.ToLower(place))})
}

// clearGymLossFailures is a compatibility name for old focused tests/callers.
// The live transition is generic: every combat loss becomes combat_retry after
// a material readiness change.
func (k *Knowledge) clearGymLossFailures() {
	k.promoteCombatLossesToRetry()
}

// gymRetryPlaces is a gym-specific view over generic retry-ready combat state.
// It is retained because gym offering needs a place-indexed lookup; it does not
// own persistence or the loss->retry transition.
func gymRetryPlaces(k *Knowledge) map[string]bool {
	ready := map[string]bool{}
	for key := range combatRetryKeys(k) {
		if key.Kind != KindGym {
			continue
		}
		place := strings.ToLower(string(key.Place))
		if place != "" {
			ready[place] = true
		}
	}
	return ready
}

// gymRetryPending retains the legacy single-place helper for callers/tests that
// only need to know whether any gym retry is due. Selection is deterministic.
func gymRetryPending(k *Knowledge) (place string, ok bool) {
	ready := gymRetryPlaces(k)
	if len(ready) == 0 {
		return "", false
	}
	places := make([]string, 0, len(ready))
	for place := range ready {
		places = append(places, place)
	}
	sort.Strings(places)
	return places[0], true
}

func appendObjectiveNote(o Objective, note string) Objective {
	if note == "" {
		return o
	}
	if o.Note == "" {
		o.Note = note
	} else {
		o.Note += " " + note
	}
	return o
}
