package skill

// Badge-four place vocabulary follows the route the map graph exposes after
// Cerulean: Route 9 -> Route 10 / Rock Tunnel -> Lavender -> Route 8 -> the
// west-east Underground Path -> Route 7 -> Celadon. The place table supplies
// concrete navigation targets only; Offer still reveals a name only when its
// map has been visited, is adjacent to the current map, or the game said it.
func init() {
	// Route 9 is 60x18 game tiles. The sign is at (25,7); one row below is
	// the ordinary standing tile used to read it. Reaching this point from
	// Cerulean crosses the Cut-gated east route, exercising Cut-aware Travel.
	places["route 9"] = Destination{Map: 0x14, X: 25, Y: 8}

	// Route 10's canonical fly warp is (11,20), immediately beside the north
	// Rock Tunnel entrance at (8,17) and the Pokemon Center at (11,19).
	places["route 10"] = Destination{Map: 0x15, X: 11, Y: 20}
	places["rock tunnel pokemon center"] = Destination{Map: 0x51, X: 3, Y: 3}
	// North Route 10 enters 1F on the pair of outside warps at x=15. One row
	// below the north entrance is floor, not a warp, so it is a stable target.
	places["rock tunnel 1f"] = Destination{Map: 0x52, X: 15, Y: 4}

	// Lavender's canonical fly warp is (3,6), directly below its Pokemon
	// Center door at (3,5). This is also a stable respawn/navigation point.
	places["lavender town"] = Destination{Map: 0x04, X: 3, Y: 6}
	places["lavender pokemon center"] = Destination{Map: 0x8D, X: 3, Y: 3}

	// Route 8's underground entrance is (13,3). One row below is the approach
	// tile on the route; the building's main-path stair is (4,4), with (4,5)
	// as the non-warp floor below it.
	places["route 8"] = Destination{Map: 0x13, X: 13, Y: 4}
	places["underground path route 8"] = Destination{Map: 0x50, X: 4, Y: 5}
	// The east end of the long west-east tunnel is warp (47,2); one tile left
	// stays in the corridor and does not immediately re-trigger the warp.
	places["underground path west east"] = Destination{Map: 0x79, X: 46, Y: 2}
	places["underground path route 7"] = Destination{Map: 0x4D, X: 4, Y: 5}
	// Route 7's underground door is (5,13); one row above is the route side.
	places["route 7"] = Destination{Map: 0x12, X: 5, Y: 12}

	// Celadon's canonical fly warp is (41,10), next to the Pokemon Center.
	places["celadon city"] = Destination{Map: 0x06, X: 41, Y: 10}
	places["celadon pokemon center"] = Destination{Map: 0x85, X: 3, Y: 3}
	// Erika stands at (4,3); (4,4) is the interaction tile directly below.
	places["celadon gym"] = Destination{Map: 0x86, X: 4, Y: 4}
}
