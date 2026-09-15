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

func nearestKnownCenter(obs Observation, known *Knowledge, knownMaps map[uint8]bool, catalog ObjectiveCatalog) (PlaceID, bool) {
	dist := map[uint8]int{obs.Map: 0}
	for queue := []uint8{obs.Map}; len(queue) > 0; queue = queue[1:] {
		for _, next := range known.Adjacency[queue[0]] {
			if _, seen := dist[next]; !seen {
				dist[next] = dist[queue[0]] + 1
				queue = append(queue, next)
			}
		}
	}
	best, bestDist := PlaceID(""), 0
	for _, destination := range catalog.Destinations {
		if !destination.Center || destination.NativeMap == obs.Map || !knownMaps[destination.NativeMap] {
			continue
		}
		hops, reachable := dist[destination.NativeMap]
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
