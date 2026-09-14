package data

// NPCTradeSite binds TradeMons' stable table index to the map object that
// actually runs that trade in English Red. Index 2 (Butterfree -> Beedrill)
// is intentionally absent because the ROM table row is unused by any script.
type NPCTradeSite struct {
	Index int
	Map   uint8
	X, Y  uint8
	Place string
	// TravelX/Y is a stable walkable interior tile used only to enter the
	// correct map. The interaction itself refreshes the NPC's live sprite
	// position, so the two pacing gate youngsters are not treated as static.
	TravelX, TravelY uint8
}

var npcTradeSites = map[int]NPCTradeSite{
	0: {Index: 0, Map: 0x56, X: 4, Y: 2, Place: "route 11 gate trade", TravelX: 7, TravelY: 6},
	1: {Index: 1, Map: 0x30, X: 4, Y: 1, Place: "route 2 trade house trade", TravelX: 2, TravelY: 6},
	3: {Index: 3, Map: 0xAA, X: 7, Y: 6, Place: "cinnabar lab fossil room trade", TravelX: 2, TravelY: 6},
	4: {Index: 4, Map: 0xC4, X: 3, Y: 5, Place: "vermilion trade house trade", TravelX: 2, TravelY: 6},
	5: {Index: 5, Map: 0xBF, X: 4, Y: 2, Place: "route 18 gate trade", TravelX: 7, TravelY: 6},
	6: {Index: 6, Map: 0x3F, X: 1, Y: 2, Place: "cerulean trade house trade", TravelX: 2, TravelY: 6},
	7: {Index: 7, Map: 0xA8, X: 1, Y: 4, Place: "cinnabar lab trade room gramps", TravelX: 2, TravelY: 6},
	8: {Index: 8, Map: 0xA8, X: 5, Y: 5, Place: "cinnabar lab trade room beauty", TravelX: 2, TravelY: 6},
	9: {Index: 9, Map: 0x47, X: 2, Y: 3, Place: "route 5 underground trade", TravelX: 3, TravelY: 6},
}

func NPCTradeSiteByIndex(index int) (NPCTradeSite, bool) {
	site, ok := npcTradeSites[index]
	return site, ok
}

func NPCTradeSites() []NPCTradeSite {
	out := make([]NPCTradeSite, 0, len(npcTradeSites))
	for index := 0; index < 10; index++ {
		if site, ok := npcTradeSites[index]; ok {
			out = append(out, site)
		}
	}
	return out
}
