package agent

import (
	"sort"
	"strings"
)

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
	current := observationLocation(obs, known)
	hops := mapHops(known.Adjacency, current)
	type candidate struct {
		place      PlaceID
		hops       int
		successful bool
		active     bool
	}
	seen := map[PlaceID]bool{}
	candidates := make([]candidate, 0, len(known.RecoveryCheckpoints)+2)
	add := func(place PlaceID, location LocationID, successful, active bool) {
		if place == "" || location == "" || seen[place] {
			return
		}
		destination, ok := catalog.destination(place)
		if !ok || !destination.Center {
			return
		}
		distance, reachable := hops[location]
		if location != current && !reachable && len(known.Adjacency) > 0 {
			return
		}
		seen[place] = true
		candidates = append(candidates, candidate{place: place, hops: distance, successful: successful, active: active})
	}
	if obs.RecoveryCheckpoint != "" {
		if destination, ok := catalog.destination(obs.RecoveryCheckpoint); ok {
			add(obs.RecoveryCheckpoint, destination.Location, true, true)
		}
	}
	for place, checkpoint := range known.RecoveryCheckpoints {
		add(place, checkpoint.Location, checkpoint.Successful, place == obs.RecoveryCheckpoint)
	}
	for _, destination := range catalog.Destinations {
		if destination.Center && destination.Location != "" && knownLocations[destination.Location] {
			add(destination.Place, destination.Location, false, destination.Place == obs.RecoveryCheckpoint)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].active != candidates[j].active {
			return candidates[i].active
		}
		if candidates[i].successful != candidates[j].successful {
			return candidates[i].successful
		}
		if candidates[i].hops != candidates[j].hops {
			return candidates[i].hops < candidates[j].hops
		}
		return candidates[i].place < candidates[j].place
	})
	ranked := make([]PlaceID, 0, len(candidates))
	for _, candidate := range candidates {
		ranked = append(ranked, candidate.place)
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
