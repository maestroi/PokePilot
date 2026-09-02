package skill

// Post-Cerulean place vocabulary lives separately from goto.go so this
// progression slice is easy to review. Each destination is either a known
// open route tile or the floor tile immediately inside/beside a decomp warp.
func init() {
	places["route 24"] = Destination{Map: 0x23, X: 10, Y: 14}
	places["route 25"] = Destination{Map: 0x24, X: 44, Y: 3}
	// BillsHouse_Object exits at (2,7)/(3,7); one row above is ordinary floor.
	places["bill's house"] = Destination{Map: 0x58, X: 2, Y: 6}

	// Vermilion City's fly warp is (11,4), a canonical open city tile.
	places["vermilion city"] = Destination{Map: 0x05, X: 11, Y: 4}
	places["vermilion pokemon center"] = Destination{Map: 0x59, X: 3, Y: 3}
	// Lt. Surge stands at (5,1). This is the stand-beside tile behind the
	// trash-can door; Gym opens that door before Travel approaches it.
	places["vermilion gym"] = Destination{Map: 0x5C, X: 5, Y: 2}

	// The dock is a one-tile corridor between the city warp at (14,0) and
	// the S.S. Anne warp at (14,2).
	places["vermilion dock"] = Destination{Map: 0x5E, X: 14, Y: 1}
	// S.S. Anne 1F lands at (26,0)/(27,0); one row inside is safe floor.
	places["ss anne 1f"] = Destination{Map: 0x5F, X: 26, Y: 1}
	// The 1F stair enters 2F at (2,4). Stand one tile to its right so a
	// destination does not immediately re-trigger the stair warp.
	places["ss anne 2f"] = Destination{Map: 0x60, X: 3, Y: 4}
	// The captain is object 1 at (4,2); (4,3) is the interaction tile below.
	places["ss anne captain's room"] = Destination{Map: 0x65, X: 4, Y: 3}
}
