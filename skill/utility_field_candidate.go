package skill

import (
	"sort"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// findReachableWildFieldCandidate is the utility-HM recovery search used by
// Cut/Flash. Unlike the older generic search it proves each hunt destination
// with the same live, component-aware semantic route planner GoTo uses. A map
// that is connected in the immutable ROM graph but sits behind an unmet story,
// Cut, Surf, Snorlax, or other semantic gate is therefore never advertised as
// a recovery source the player cannot actually reach yet.
func findReachableWildFieldCandidate(m *emu.Emu, romData []byte, target FieldMove, required []FieldMove) (wildFieldCandidate, bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)

	planner, err := NewRoutePlanner(m, romData)
	if err != nil {
		return wildFieldCandidate{}, false, err
	}

	// Prefer the current map's live component when it has a compatible encounter
	// before considering travel. Do not let the canonical Place coordinate for
	// this same map hide a reachable local grass component.
	dests := []Destination{{Map: planner.cur, X: planner.x, Y: planner.y}}
	for _, dest := range knownGrassDestinations() {
		if dest.Map != planner.cur {
			dests = append(dests, dest)
		}
	}
	sort.SliceStable(dests[1:], func(i, j int) bool {
		a, b := dests[i+1], dests[j+1]
		if a.Map != b.Map {
			return a.Map < b.Map
		}
		if a.X != b.X {
			return a.X < b.X
		}
		return a.Y < b.Y
	})

	best := wildFieldCandidate{RouteLen: int(^uint(0) >> 1)}
	found := false
	seenMap := map[uint8]bool{}
	for _, dest := range dests {
		if seenMap[dest.Map] {
			continue
		}
		seenMap[dest.Map] = true

		route, err := world.FindRouteAtDestinationWithCapabilities(
			planner.graph,
			planner.cur,
			dest.Map,
			int(planner.x), int(planner.y),
			int(dest.X), int(dest.Y),
			nil,
			planner.prereqs,
		)
		if err != nil {
			continue
		}

		wild, err := WildGrass(romData, dest.Map)
		if err != nil {
			return wildFieldCandidate{}, false, err
		}
		for _, species := range wild {
			incoming := state.Mon{Species: species.ID}
			canTarget, err := monCanPlaceFieldMove(romData, incoming, target)
			if err != nil {
				return wildFieldCandidate{}, false, err
			}
			if !canTarget {
				continue
			}
			_, legal, err := chooseDepositSlotForIncoming(romData, party, incoming, required)
			if err != nil {
				return wildFieldCandidate{}, false, err
			}
			if !legal {
				continue
			}

			candidate := wildFieldCandidate{
				Destination: dest,
				Map:         dest.Map,
				Species:     species.ID,
				Slots:       species.Slots,
				RouteLen:    len(route),
			}
			if !found || candidate.RouteLen < best.RouteLen ||
				(candidate.RouteLen == best.RouteLen && candidate.Slots > best.Slots) ||
				(candidate.RouteLen == best.RouteLen && candidate.Slots == best.Slots && candidate.Map < best.Map) ||
				(candidate.RouteLen == best.RouteLen && candidate.Slots == best.Slots && candidate.Map == best.Map && candidate.Species < best.Species) {
				best, found = candidate, true
			}
		}
	}
	return best, found, nil
}
