package agent

import "strings"

var fieldMedStatus = map[string]string{
	"potion":       "",
	"super potion": "",
	"hyper potion": "",
	"max potion":   "",
	"full restore": "",
	"antidote":     "poisoned",
	"burn heal":    "burned",
	"ice heal":     "frozen",
	"awakening":    "asleep",
	"parlyz heal":  "paralyzed",
}

func medReaches(mon PartyMon, wantStatus string) bool {
	if wantStatus == "" {
		return mon.HP > 0 && monHurt(mon)
	}
	return mon.Status == wantStatus
}

// preferredRecoveryCenters is the candidate-only compatibility projection of
// the structured recovery checkpoint scorer.
func preferredRecoveryCenters(obs Observation, known *Knowledge, knownLocations map[LocationID]bool, catalog ObjectiveCatalog) []PlaceID {
	return recoveryCheckpointPlaces(rankRecoveryCheckpoints(obs, known, knownLocations, catalog, nil))
}

// firstRoutableRecoveryCenter returns the most preferred Center whose live
// route is not positively rejected. Routability is a decision the generic Offer
// already makes for journeys (route_blockage.go, travelObjectiveProvider); a
// heal that names a destination is the same question and must not skip it.
// Withholding is fail-open: an empty unroutable set means the router was never
// consulted, so the first preference stands.
func firstRoutableRecoveryCenter(candidates []PlaceID, unroutable map[string]bool) (PlaceID, bool) {
	for _, candidate := range candidates {
		if !unroutable[string(candidate)] {
			return candidate, true
		}
	}
	return "", false
}

func observedEvent(obs Observation, name string) bool {
	for _, event := range obs.Events {
		if event == name {
			return true
		}
	}
	return false
}

func bagHas(obs Observation, name string) bool {
	for _, it := range obs.Bag {
		if it.Name == name && it.Quantity > 0 {
			return true
		}
	}
	return false
}

func hasBalls(obs Observation) bool {
	for _, it := range obs.Bag {
		if it.Name == "pokeball" || it.Name == "great ball" {
			return true
		}
	}
	return false
}

func isCenter(mapName string) bool {
	return strings.Contains(strings.ToUpper(mapName), "POKECENTER")
}

func isMart(mapName string) bool {
	return strings.Contains(strings.ToUpper(mapName), "MART")
}
