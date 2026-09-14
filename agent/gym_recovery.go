package agent

import "strings"

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

func gymLossFailureName(o Objective, err error) (string, bool) {
	if o.Kind != KindGym || o.Place == "" || err == nil {
		return "", false
	}
	if !strings.Contains(err.Error(), "lost to the gym leader") {
		return "", false
	}
	return gymLossFailureKey(o.Place), true
}

func gymLossRecorded(k *Knowledge, place string) bool {
	if k == nil || place == "" {
		return false
	}
	if _, ok := k.Failures[gymLossFailureKey(place)]; ok {
		return true
	}
	_, ok := k.Failures[legacyGymLossFailureKey(place)]
	return ok
}

// clearGymLossFailures marks a lost gym as READY TO RETRY after a material
// combat-readiness change. The recovery marker is now keyed by the same
// ObjectiveKey as the challenge plus a typed recovery mode, so copy changes to
// "beat the gym leader here" cannot change the state machine.
func (k *Knowledge) clearGymLossFailures() {
	if k == nil {
		return
	}
	var retryPlace string
	var retry Failure
	for storage, f := range k.Failures {
		if key, mode, ok := parseFailureStorageKey(storage); ok && mode == failureModeGymLoss {
			retryPlace = string(key.Place)
			retry = f
			delete(k.Failures, storage)
			continue
		}
		if strings.HasPrefix(storage, legacyGymLossFailurePrefix) {
			retryPlace = strings.ToLower(strings.TrimPrefix(storage, legacyGymLossFailurePrefix))
			retry = f
			delete(k.Failures, storage)
		}
	}
	if retryPlace == "" {
		return
	}
	retry.Objective = Objective{Kind: KindGym, Place: PlaceID(retryPlace)}.String()
	retry.Last = "party trained after losing at " + strings.ToUpper(retryPlace) + "; retry is due"
	k.Failures[gymRetryReadyKey(retryPlace)] = retry
}

// gymRetryPending reports the recovery state created by clearGymLossFailures.
// A fresh scoped leader loss takes precedence over an older ready marker.
func gymRetryPending(k *Knowledge) (place string, ok bool) {
	if k == nil {
		return "", false
	}
	for storage := range k.Failures {
		if _, mode, parsed := parseFailureStorageKey(storage); parsed && mode == failureModeGymLoss {
			return "", false
		}
		if strings.HasPrefix(storage, legacyGymLossFailurePrefix) {
			return "", false
		}
	}
	for storage, f := range k.Failures {
		if key, mode, parsed := parseFailureStorageKey(storage); parsed && mode == failureModeGymRetry {
			return string(key.Place), true
		}
		// v4 checkpoints used the display sentence as the retry key and kept
		// the place only in Last. Preserve that one-release migration path.
		if storage == (Objective{Kind: KindGym}).String() {
			const prefix = "party trained after losing at "
			const suffix = "; retry is due"
			if strings.HasPrefix(f.Last, prefix) && strings.HasSuffix(f.Last, suffix) {
				place = strings.TrimSuffix(strings.TrimPrefix(f.Last, prefix), suffix)
				return strings.ToLower(place), true
			}
		}
	}
	return "", false
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
