package skill

// Post-Tower place vocabulary follows the eastern Snorlax route to Fuchsia:
// Lavender -> Route 12 -> Routes 13/14/15 -> Fuchsia. It intentionally avoids
// Cycling Road so #33 depends only on the Poké Flute handoff from #32.
func init() {
	// Snorlax stand tiles are execution details of FuchsiaProgression, not
	// standalone journeys. Keeping them out of PlaceNames prevents the planner
	// from parking on either side of the sleeping sprite instead of selecting
	// the story verb that wakes and resolves it. Place still resolves them for
	// the compound skill through interactionPlaces.
	interactionPlaces["route 12 snorlax"] = Destination{Map: route12Map, X: 10, Y: 61}
	interactionPlaces["route 12 south of snorlax"] = Destination{Map: route12Map, X: 10, Y: 64}

	// These route targets sit beside signs or open corridor tiles rather than
	// trainer sprites, making them useful deterministic waypoints for Travel.
	places["route 13"] = Destination{Map: route13Map, X: 31, Y: 12}
	places["route 14"] = Destination{Map: route14Map, X: 17, Y: 14}
	places["route 15"] = Destination{Map: route15Map, X: 15, Y: 9}

	// Fuchsia's Center door is at (19,27); stand immediately below it for a
	// stable city checkpoint. Indoor center coordinates follow the same nurse
	// target convention as the other city place tables.
	places["fuchsia city"] = Destination{Map: fuchsiaCityMap, X: 19, Y: 28}
	places["fuchsia pokemon center"] = Destination{Map: fuchsiaPokemonCenterMap, X: 3, Y: 3}

	// Koga stands at (4,10), facing down.
	places["fuchsia gym"] = Destination{Map: fuchsiaGymMap, X: 4, Y: 11}

	// Safari entry, finite-step routing, reward pickup and Warden handoff are
	// one resumable story operation. Exposing these implementation coordinates
	// as ordinary journeys lets the strategist enter a paid/choice-gated zone
	// without the code that owns its session budget, so keep them interaction-
	// only while retaining Place lookups for FuchsiaProgression.
	interactionPlaces["safari zone gate"] = Destination{Map: safariZoneGateMap, X: 3, Y: 3}
	interactionPlaces["safari exit approach"] = Destination{Map: safariZoneCenterMap, X: 14, Y: 24}
	interactionPlaces["safari gold teeth"] = Destination{Map: safariZoneWestMap, X: 19, Y: 8}
	interactionPlaces["safari secret house"] = Destination{Map: safariZoneSecretHouse, X: 3, Y: 4}
	interactionPlaces["warden"] = Destination{Map: wardensHouseMap, X: 2, Y: 4}

	// The house itself is a safe ordinary destination; only the NPC interaction
	// above is progression-owned.
	places["wardens house"] = Destination{Map: wardensHouseMap, X: 3, Y: 3}
}
