package rom

import "github.com/maestroi/pokepilot/world"

const (
	forcedRocketHideoutB2F uint8 = 0xC8
	forcedRocketHideoutB3F uint8 = 0xC9
	forcedViridianGym      uint8 = 0x2D
)

// forcedMovementTables are Red adapter facts decoded from the map scripts.
// The portable navigation layer only consumes trigger -> landing transitions;
// it does not know what a spinner/arrow tile is or which game supplied them.
var forcedMovementTables = map[uint8]map[[2]int]world.Point{
	forcedRocketHideoutB2F: {
		{4, 9}: {X: 2, Y: 9}, {4, 11}: {X: 8, Y: 11}, {4, 15}: {X: 8, Y: 11}, {4, 16}: {X: 8, Y: 11},
		{4, 19}: {X: 2, Y: 19}, {4, 22}: {X: 2, Y: 19}, {5, 14}: {X: 9, Y: 16}, {6, 22}: {X: 6, Y: 20},
		{6, 24}: {X: 6, Y: 20}, {8, 9}: {X: 2, Y: 9}, {8, 12}: {X: 8, Y: 11}, {8, 15}: {X: 8, Y: 11},
		{8, 19}: {X: 2, Y: 19}, {8, 23}: {X: 2, Y: 19}, {9, 14}: {X: 9, Y: 16}, {9, 22}: {X: 9, Y: 24},
		{10, 9}: {X: 2, Y: 9}, {10, 10}: {X: 2, Y: 9}, {10, 15}: {X: 2, Y: 9}, {10, 17}: {X: 14, Y: 15},
		{10, 19}: {X: 14, Y: 15}, {10, 25}: {X: 14, Y: 25}, {11, 14}: {X: 15, Y: 18}, {11, 16}: {X: 15, Y: 18},
		{11, 18}: {X: 11, Y: 20}, {12, 9}: {X: 2, Y: 9}, {12, 11}: {X: 2, Y: 9}, {12, 13}: {X: 2, Y: 9},
		{12, 17}: {X: 14, Y: 15}, {13, 10}: {X: 14, Y: 12}, {13, 12}: {X: 14, Y: 12}, {13, 16}: {X: 15, Y: 18},
		{13, 18}: {X: 11, Y: 20}, {13, 19}: {X: 14, Y: 15}, {13, 22}: {X: 9, Y: 24}, {13, 23}: {X: 2, Y: 19},
		{14, 17}: {X: 14, Y: 15}, {15, 16}: {X: 15, Y: 18}, {16, 14}: {X: 16, Y: 13}, {16, 16}: {X: 16, Y: 13},
		{16, 18}: {X: 16, Y: 13}, {17, 10}: {X: 14, Y: 12}, {17, 11}: {X: 2, Y: 9},
	},
	forcedRocketHideoutB3F: {
		{10, 13}: {X: 14, Y: 13}, {10, 19}: {X: 18, Y: 15}, {11, 18}: {X: 15, Y: 22}, {12, 11}: {X: 10, Y: 11},
		{12, 17}: {X: 18, Y: 15}, {12, 20}: {X: 18, Y: 15}, {13, 16}: {X: 17, Y: 16}, {14, 11}: {X: 16, Y: 11},
		{14, 15}: {X: 18, Y: 15}, {14, 17}: {X: 18, Y: 15}, {14, 19}: {X: 18, Y: 15}, {15, 16}: {X: 17, Y: 16},
		{15, 18}: {X: 15, Y: 22}, {16, 13}: {X: 16, Y: 11}, {17, 12}: {X: 17, Y: 16}, {18, 16}: {X: 18, Y: 15},
	},
	forcedViridianGym: {
		{19, 11}: {X: 19, Y: 2},
		{19, 1}:  {X: 11, Y: 1},
		{18, 2}:  {X: 18, Y: 11},
		{11, 2}:  {X: 17, Y: 2},
		{16, 10}: {X: 16, Y: 12},
		{4, 6}:   {X: 4, Y: 13},
		{5, 13}:  {X: 13, Y: 13},
		{4, 14}:  {X: 13, Y: 14},
		{0, 15}:  {X: 0, Y: 7},
		{1, 15}:  {X: 1, Y: 9},
		{13, 16}: {X: 7, Y: 16},
		{13, 17}: {X: 1, Y: 17},
	},
}

// ForcedMovementLanding returns the deterministic landing selected by Red's
// script after the player enters trigger (x,y). false means the tile is an
// ordinary movement cell for this map.
func ForcedMovementLanding(mapID uint8, x, y int) (world.Point, bool) {
	table := forcedMovementTables[mapID]
	if table == nil {
		return world.Point{}, false
	}
	landing, ok := table[[2]int{x, y}]
	return landing, ok
}

// ForcedMovementTransitions returns a copy for adapter-level verification.
func ForcedMovementTransitions(mapID uint8) map[[2]int]world.Point {
	src := forcedMovementTables[mapID]
	if len(src) == 0 {
		return nil
	}
	out := make(map[[2]int]world.Point, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
