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

func preferredRecoveryCenter(obs Observation, known *Knowledge, knownLocations map[LocationID]bool, catalog ObjectiveCatalog) (PlaceID, bool) {
	if obs.RecoveryCheckpoint != "" {
		if destination, ok := catalog.destination(obs.RecoveryCheckpoint); ok && destination.Center {
			// The cartridge's active blackout checkpoint is stronger evidence than
			// planner visitation: the player necessarily activated this nurse.
			return obs.RecoveryCheckpoint, true
		}
	}
	return nearestKnownCenter(obs, known, knownLocations, catalog)
}

func nearestKnownCenter(obs Observation, known *Knowledge, knownLocations map[LocationID]bool, catalog ObjectiveCatalog) (PlaceID, bool) {
	current := observationLocation(obs, known)
	dist := mapHops(known.Adjacency, current)
	best, bestDist := PlaceID(""), 0
	for _, destination := range catalog.Destinations {
		if !destination.Center || destination.Location == "" || destination.Location == current || !knownLocations[destination.Location] {
			continue
		}
		hops, reachable := dist[destination.Location]
		if !reachable {
			continue
		}
		if best == "" || hops < bestDist || (hops == bestDist && destination.Place < best) {
			best, bestDist = destination.Place, hops
		}
	}
	return best, best != ""
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
