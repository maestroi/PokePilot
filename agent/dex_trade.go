package agent

import (
	"fmt"
	"strings"

	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/rom"
)

const (
	dexTradeIntent         = "dex-npc-trade"
	dexTradeObjectiveLimit = 4
)

// appendDexTradeObjectives exposes only NPC trades whose give-species is
// currently in the party and safe to surrender. Keeping the precondition
// explicit makes the offer deterministic and prevents a stale Pokédex-owned bit
// from being treated as proof that the requested species still physically
// exists somewhere in storage.
func appendDexTradeObjectives(romData []byte, obs Observation, known *Knowledge, out []Objective) []Objective {
	if len(obs.Dex.Targets) == 0 {
		return out
	}
	owned := pokedexOwnedSet(obs)
	already := map[SpeciesID]bool{}
	for _, o := range out {
		if o.Kind == KindCatch && o.Species != "" {
			already[o.Species] = true
		}
	}

	blocked := dexCatchBlockedPlaces(obs)
	hops := map[uint8]int{}
	var adjacency map[uint8][]uint8
	if known != nil {
		adjacency = known.Adjacency
		hops = mapHops(adjacency, obs.Map)
	}

	added := 0
	for _, entry := range obs.Dex.Targets {
		if added >= dexTradeObjectiveLimit || owned[entry.Species] || already[entry.Species] {
			continue
		}
		for _, src := range entry.Sources {
			if src.Kind != AcquireInGameTrade || src.Give == "" {
				continue
			}
			slot, _, ok := partySpeciesSlot(obs.Party, src.Give)
			if !ok || partySlotCarriesFieldMove(obs, slot) {
				continue
			}
			site, ok := dexTradeSite(romData, entry.Species, src.Give)
			if !ok {
				continue
			}
			place := PlaceID(site.Place)
			if _, ok := dexCatchPlaceDistance(obs, place, blocked, hops, adjacency); !ok {
				continue
			}
			out = append(out, Objective{
				Kind:    KindCatch,
				Species: entry.Species,
				Place:   place,
				Intent:  dexTradeIntent,
				Flee:    true,
				Note: fmt.Sprintf("(dex NPC trade: give %s for %s at %s; one-for-one party swap with owned-bit verification)",
					strings.ToUpper(string(src.Give)), strings.ToUpper(string(entry.Species)), strings.ToUpper(site.Place)),
			})
			already[entry.Species] = true
			added++
			break
		}
	}
	return out
}

func dexTradeSite(romData []byte, want, give SpeciesID) (reddata.NPCTradeSite, bool) {
	wantInternal, ok := redSpeciesID(want)
	if !ok {
		return reddata.NPCTradeSite{}, false
	}
	giveInternal, ok := redSpeciesID(give)
	if !ok {
		return reddata.NPCTradeSite{}, false
	}
	trades, err := rom.NPCTrades(romData)
	if err != nil {
		return reddata.NPCTradeSite{}, false
	}
	for _, trade := range trades {
		if trade.Get != wantInternal || trade.Give != giveInternal {
			continue
		}
		return reddata.NPCTradeSiteByIndex(trade.Index)
	}
	return reddata.NPCTradeSite{}, false
}

func partySlotCarriesFieldMove(obs Observation, slot int) bool {
	for _, capability := range obs.FieldCapabilities {
		if capability.PartySlot == slot && capability.Learned {
			return true
		}
	}
	return false
}
