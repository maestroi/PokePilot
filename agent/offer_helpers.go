package agent

import (
	"strings"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
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

func nearestKnownCenter(obs Observation, known *Knowledge, knownMaps map[uint8]bool) (string, bool) {
	dist := map[uint8]int{obs.Map: 0}
	for queue := []uint8{obs.Map}; len(queue) > 0; queue = queue[1:] {
		for _, next := range known.Adjacency[queue[0]] {
			if _, seen := dist[next]; !seen {
				dist[next] = dist[queue[0]] + 1
				queue = append(queue, next)
			}
		}
	}
	best, bestDist := "", 0
	for _, name := range skill.PlaceNames() {
		d, _ := skill.Place(name)
		if d.Map == obs.Map || !knownMaps[d.Map] || !isCenter(state.MapName(d.Map)) {
			continue
		}
		hops, reachable := dist[d.Map]
		if !reachable {
			continue
		}
		if best == "" || hops < bestDist {
			best, bestDist = name, hops
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
