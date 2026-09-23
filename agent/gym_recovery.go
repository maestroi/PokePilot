package agent

import (
	"errors"
	"sort"
	"strings"
)

// legacyGymLossFailurePrefix is retained only to read v4/string-keyed checkpoint
// memory. New durable state uses failureModeGymLoss plus ObjectiveKey.
const legacyGymLossFailurePrefix = "beat the gym leader at "

func gymObjectiveKey(place string) ObjectiveKey {
	return Objective{Kind: KindGym, Place: PlaceID(strings.ToLower(place))}.Key()
}

func gymRetryReadyKey(place string) string {
	return failureStorageKey(gymObjectiveKey(place), failureModeGymRetry)
}

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

// gymLossFailureName is a compatibility adapter for legacy direct callers of
// Knowledge.Failed. Live Run records gym losses from ObjectiveResult.Battle via
// FailedResult; this path recognizes only the typed sentinel emitted by
// gymOutcomeErr and never infers semantics from error prose.
func gymLossFailureName(o Objective, err error) (string, bool) {
	if o.Kind != KindGym || o.Place == "" || !errors.Is(err, errGymLeaderLost) {
		return "", false
	}
	return gymLossFailureKey(o.Place), true
}

func gymLossRecorded(k *Knowledge, place string) bool {
	if k == nil || place == "" {
		return false
	}
	if combatLossRecorded(k, Objective{Kind: KindGym, Place: PlaceID(strings.ToLower(place))}) {
		return true
	}
	if _, ok := k.Failures[gymLossFailureKey(place)]; ok {
		return true
	}
	_, ok := k.Failures[legacyGymLossFailureKey(place)]
	return ok
}

func mergeGymRetryFailure(a, b Failure) Failure {
	if b.Times > a.Times || a.Objective == "" {
		return b
	}
	return a
}

// clearGymLossFailures marks every lost gym as READY TO RETRY after a material
// combat-readiness change. Recovery remains scoped by ObjectiveKey all the way
// through: multiple prior gym losses produce multiple retry markers instead of
// nondeterministically collapsing to whichever Go map entry was visited last.
func (k *Knowledge) clearGymLossFailures() {
	if k == nil {
		return
	}
	retries := map[string]Failure{}
	for storage, f := range k.Failures {
		if key, mode, ok := parseFailureStorageKey(storage); ok &&
			(mode == failureModeGymLoss || (mode == failureModeCombatLoss && key.Kind == KindGym)) {
			place := strings.ToLower(string(key.Place))
			if place != "" {
				retries[place] = mergeGymRetryFailure(retries[place], f)
			}
			delete(k.Failures, storage)
			continue
		}
		if strings.HasPrefix(storage, legacyGymLossFailurePrefix) {
			place := strings.ToLower(strings.TrimPrefix(storage, legacyGymLossFailurePrefix))
			if place != "" {
				retries[place] = mergeGymRetryFailure(retries[place], f)
			}
			delete(k.Failures, storage)
		}
	}
	for place, retry := range retries {
		retry.Objective = Objective{Kind: KindGym, Place: PlaceID(place)}.String()
		retry.Last = "party trained after losing at " + strings.ToUpper(place) + "; retry is due"
		storage := gymRetryReadyKey(place)
		retry = mergeGymRetryFailure(k.Failures[storage], retry)
		k.Failures[storage] = retry
	}
}

// gymRetryPlaces returns each gym whose prior loss has been followed by a
// material readiness change and not superseded by a fresh loss at that same
// gym. A loss at one gym never masks an independently ready retry elsewhere.
func gymRetryPlaces(k *Knowledge) map[string]bool {
	ready := map[string]bool{}
	lost := map[string]bool{}
	if k == nil {
		return ready
	}
	for storage, f := range k.Failures {
		if key, mode, parsed := parseFailureStorageKey(storage); parsed {
			place := strings.ToLower(string(key.Place))
			switch mode {
			case failureModeGymLoss:
				if place != "" {
					lost[place] = true
				}
			case failureModeCombatLoss:
				if key.Kind == KindGym && place != "" {
					lost[place] = true
				}
			case failureModeGymRetry:
				if place != "" {
					ready[place] = true
				}
			}
			continue
		}
		if strings.HasPrefix(storage, legacyGymLossFailurePrefix) {
			place := strings.ToLower(strings.TrimPrefix(storage, legacyGymLossFailurePrefix))
			if place != "" {
				lost[place] = true
			}
			continue
		}
		// v4 checkpoints used the generic gym display sentence as the retry key
		// and kept the place only in Last. Preserve that migration path.
		if storage == (Objective{Kind: KindGym}).String() {
			const prefix = "party trained after losing at "
			const suffix = "; retry is due"
			if strings.HasPrefix(f.Last, prefix) && strings.HasSuffix(f.Last, suffix) {
				place := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(f.Last, prefix), suffix))
				if place != "" {
					ready[place] = true
				}
			}
		}
	}
	for place := range lost {
		delete(ready, place)
	}
	return ready
}

// gymRetryPending retains the legacy single-place helper for callers/tests that
// only need to know whether any retry is due. Selection is deterministic.
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
