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

// preferredRecoveryCenters ranks the Centers a hurt party may heal at, most
// preferred first. It returns a list rather than one name because the live
// router can reject the best candidate: the cartridge's active blackout
// checkpoint is the strongest preference, but it is not usable when the player
// is standing behind a gate that the checkpoint sits on the far side of. The
// caller takes the first candidate the router accepts.
func preferredRecoveryCenters(obs Observation, known *Knowledge, knownLocations map[LocationID]bool, catalog ObjectiveCatalog) []PlaceID {
	ranked := make([]PlaceID, 0, 2)
	if obs.RecoveryCheckpoint != "" {
		if destination, ok := catalog.destination(obs.RecoveryCheckpoint); ok && destination.Center {
			// The cartridge's active blackout checkpoint is stronger evidence than
			// planner visitation: the player necessarily activated this nurse.
			ranked = append(ranked, obs.RecoveryCheckpoint)
		}
	}
	if name, ok := nearestKnownCenter(obs, known, knownLocations, catalog); ok && name != obs.RecoveryCheckpoint {
		ranked = append(ranked, name)
	}
	return ranked
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
