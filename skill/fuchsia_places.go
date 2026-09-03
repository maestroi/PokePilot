package skill

// Post-Tower place vocabulary follows the eastern Snorlax route to Fuchsia:
// Lavender -> Route 12 -> Routes 13/14/15 -> Fuchsia. It intentionally avoids
// Cycling Road so #33 depends only on the Poké Flute handoff from #32.
func init() {
	// Route 12 Snorlax is at (10,62). The tile immediately north is a stable
	// place to use the Poké Flute; the south-side target becomes reachable
	// only after the story encounter has been resolved.
	places["route 12 snorlax"] = Destination{Map: route12Map, X: 10, Y: 61}
	places["route 12 south of snorlax"] = Destination{Map: route12Map, X: 10, Y: 64}

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

	// The Safari entrance script triggers one tile north of (3,3). The exit
	// approach is on Center at (14,24), directly above its south gate warp.
	places["safari zone gate"] = Destination{Map: safariZoneGateMap, X: 3, Y: 3}
	places["safari exit approach"] = Destination{Map: safariZoneCenterMap, X: 14, Y: 24}

	// Gold Teeth are the item object at (19,7) in West; stand below it. The
	// Secret House guru is at (3,3), so (3,4) is the interaction stand.
	places["safari gold teeth"] = Destination{Map: safariZoneWestMap, X: 19, Y: 8}
	places["safari secret house"] = Destination{Map: safariZoneSecretHouse, X: 3, Y: 4}

	// The Warden is at (2,3); (2,4) is directly below him.
	places["warden"] = Destination{Map: wardensHouseMap, X: 2, Y: 4}
	places["wardens house"] = Destination{Map: wardensHouseMap, X: 3, Y: 3}
}
