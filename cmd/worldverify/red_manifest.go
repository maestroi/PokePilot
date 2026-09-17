package main

import (
	"strconv"
	"strings"

	redstate "github.com/maestroi/pokepilot/red/state"
	verifier "github.com/maestroi/pokepilot/worldverify"
)

// gen1RequiredMaps is deliberately a completion-path invariant, not a list of
// every interesting room in English Red/Blue. The two versions share these map
// ids and story locations; version-exclusive encounter data is irrelevant to
// world reachability. Each entry owns a story item, mandatory boss, or final
// game handoff that a normal completed run must be able to reach.
var gen1RequiredMaps = map[verifier.MapID]string{
	"00": "normal game starts in Pallet Town",
	"28": "Oak's Lab owns the opening/Pokedex progression",
	"02": "Pewter City is the first badge hub",
	"36": "Pewter Gym owns the Boulder Badge",
	"03": "Cerulean City is the second badge hub",
	"41": "Cerulean Gym owns the Cascade Badge",
	"58": "Bill's House owns the S.S. Ticket progression",
	"05": "Vermilion City is the S.S. Anne and Thunder Badge hub",
	"5e": "Vermilion Dock is the mandatory S.S. Anne handoff",
	"5f": "S.S. Anne 1F is the ship's traversal root",
	"65": "S.S. Anne Captain's Room owns HM01 Cut",
	"5c": "Vermilion Gym owns the Thunder Badge",
	"06": "Celadon City is the Rocket and Rainbow Badge hub",
	"87": "Game Corner owns the Rocket Hideout entrance",
	"c7": "Rocket Hideout B1F is mandatory Rocket progression",
	"ca": "Rocket Hideout B4F owns the Silph Scope boss progression",
	"86": "Celadon Gym owns the Rainbow Badge",
	"04": "Lavender Town is the Pokemon Tower progression hub",
	"8e": "Pokemon Tower 1F is the tower traversal root",
	"94": "Pokemon Tower 7F owns the Mr. Fuji rescue",
	"95": "Mr. Fuji's House owns the Poke Flute handoff",
	"07": "Fuchsia City is the Safari and Soul Badge hub",
	"9c": "Safari Zone Gate is the mandatory HM03 route",
	"de": "Safari Zone Secret House owns HM03 Surf",
	"9d": "Fuchsia Gym owns the Soul Badge",
	"0a": "Saffron City is the Silph and Marsh Badge hub",
	"b5": "Silph Co 1F is the Silph traversal root",
	"eb": "Silph Co 11F owns the Giovanni rescue progression",
	"b2": "Saffron Gym owns the Marsh Badge",
	"08": "Cinnabar Island is the Volcano Badge hub",
	"a5": "Pokemon Mansion 1F is the Secret Key dungeon root",
	"d8": "Pokemon Mansion B1F contains the Secret Key progression",
	"a6": "Cinnabar Gym owns the Volcano Badge",
	"01": "Viridian City is the final badge hub",
	"2d": "Viridian Gym owns the Earth Badge",
	"c1": "Route 22 Gate is the mandatory badge-check handoff to Victory Road",
	"6c": "Victory Road 1F is mandatory Elite Four progression",
	"c2": "Victory Road 2F is mandatory Elite Four progression",
	"c6": "Victory Road 3F is mandatory Elite Four progression",
	"09": "Indigo Plateau is the Elite Four hub",
	"ae": "Indigo Plateau Lobby is the Elite Four entry point",
	"f5": "Lorelei's Room is a mandatory Elite Four battle",
	"f6": "Bruno's Room is a mandatory Elite Four battle",
	"f7": "Agatha's Room is a mandatory Elite Four battle",
	"71": "Lance's Room is a mandatory Elite Four battle",
	"78": "Champion's Room is the mandatory final rival battle",
}

func applyGen1ReachabilityManifest(snapshot *verifier.Snapshot) {
	if snapshot == nil {
		return
	}

	// English Red and Blue share the Gen-I native map-id/name table. The
	// generic verifier still treats both ids and labels as opaque strings.
	for i := range snapshot.Maps {
		raw, err := strconv.ParseUint(string(snapshot.Maps[i].ID), 16, 8)
		if err != nil {
			continue
		}
		snapshot.Maps[i].Label = redstate.MapName(uint8(raw))
	}

	seen := make(map[verifier.MapID]bool)
	add := func(id verifier.MapID, class verifier.ReachabilityClass, reason string) {
		if seen[id] {
			return
		}
		seen[id] = true
		snapshot.MapExpectations = append(snapshot.MapExpectations, verifier.MapExpectation{
			Map: id, Class: class, Reason: reason,
		})
	}

	for id, reason := range gen1RequiredMaps {
		add(id, verifier.ReachabilityRequired, reason)
	}

	for _, m := range snapshot.Maps {
		label := m.Label
		switch {
		case strings.HasPrefix(label, "UNUSED_MAP_"):
			add(m.ID, verifier.ReachabilityExpectedUnreachable, "Gen-I decomp marks this map unused")
		case strings.HasSuffix(label, "_COPY"):
			add(m.ID, verifier.ReachabilityExpectedUnreachable, "dead duplicate map; live warps target the non-COPY map")
		case label == "TRADE_CENTER" || label == "COLOSSEUM":
			add(m.ID, verifier.ReachabilityOptional, "link-cable room outside the normal single-player world graph")
		case label == "HALL_OF_FAME":
			add(m.ID, verifier.ReachabilityStoryStateDependent, "entered by the scripted post-Champion sequence rather than ordinary traversal")
		}
	}
}
