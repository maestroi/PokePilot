// Package rom owns Gold/Silver static world facts that are not part of the
// portable semantic catalog.
package rom

import (
	"fmt"
	"sort"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/worldmodel"
)

const (
	dirNorth uint8 = iota
	dirSouth
	dirWest
	dirEast
)

type warpSpec struct {
	x, y     uint8
	dest     string
	destWarp uint8 // decomp warp ids are 1-based
}

type connectionSpec struct {
	dir    uint8
	dest   string
	offset int8
}

type mapSpec struct {
	name        string
	warps       []warpSpec
	connections []connectionSpec
}

// firstBadgeTopology is the verified early-Johto topology currently covered by
// the native provider: opening maps through Violet/Azalea, Ilex Forest and
// Goldenrod, then Routes 35/36/37 through Ecruteak.
//
// Facts are projected from pret/pokegold at gs/data.SourceRevision. Keeping the
// names here (rather than raw group/number ids) makes the generated gs/data
// catalog the single source of native map identity.
//
// Live routing still uses this verified early-Johto slice. The full #973
// ROM parser is NewWorldProvider: it enumerates every catalog map. Unsupported
// destinations may appear in a source map's warp/connection table, but
// BuildNativeGraph drops an edge unless the destination is also enumerated
// by MapIDs.
var firstBadgeTopology = []mapSpec{
	{
		name: "PLAYERS_HOUSE_2F",
		warps: []warpSpec{
			{x: 7, y: 0, dest: "PLAYERS_HOUSE_1F", destWarp: 3},
		},
	},
	{
		name: "PLAYERS_HOUSE_1F",
		warps: []warpSpec{
			{x: 6, y: 7, dest: "NEW_BARK_TOWN", destWarp: 2},
			{x: 7, y: 7, dest: "NEW_BARK_TOWN", destWarp: 2},
			{x: 9, y: 0, dest: "PLAYERS_HOUSE_2F", destWarp: 1},
		},
	},
	{
		name: "NEW_BARK_TOWN",
		warps: []warpSpec{
			{x: 6, y: 3, dest: "ELMS_LAB", destWarp: 1},
			{x: 13, y: 5, dest: "PLAYERS_HOUSE_1F", destWarp: 1},
			{x: 3, y: 11, dest: "PLAYERS_NEIGHBORS_HOUSE", destWarp: 1},
			{x: 11, y: 13, dest: "ELMS_HOUSE", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirWest, dest: "ROUTE_29", offset: 0},
			{dir: dirEast, dest: "ROUTE_27", offset: 0},
		},
	},
	{
		name: "ELMS_LAB",
		warps: []warpSpec{
			{x: 4, y: 11, dest: "NEW_BARK_TOWN", destWarp: 1},
			{x: 5, y: 11, dest: "NEW_BARK_TOWN", destWarp: 1},
		},
	},
	{
		name: "ROUTE_29",
		warps: []warpSpec{
			{x: 27, y: 1, dest: "ROUTE_29_ROUTE_46_GATE", destWarp: 3},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_46", offset: 10},
			{dir: dirWest, dest: "CHERRYGROVE_CITY", offset: 0},
			{dir: dirEast, dest: "NEW_BARK_TOWN", offset: 0},
		},
	},
	{
		name: "CHERRYGROVE_CITY",
		warps: []warpSpec{
			{x: 23, y: 3, dest: "CHERRYGROVE_MART", destWarp: 2},
			{x: 29, y: 3, dest: "CHERRYGROVE_POKECENTER_1F", destWarp: 1},
			{x: 17, y: 7, dest: "CHERRYGROVE_GYM_SPEECH_HOUSE", destWarp: 1},
			{x: 25, y: 9, dest: "GUIDE_GENTS_HOUSE", destWarp: 1},
			{x: 31, y: 11, dest: "CHERRYGROVE_EVOLUTION_SPEECH_HOUSE", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_30", offset: 5},
			{dir: dirEast, dest: "ROUTE_29", offset: 0},
		},
	},
	{
		name: "CHERRYGROVE_MART",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "CHERRYGROVE_CITY", destWarp: 1},
			{x: 3, y: 7, dest: "CHERRYGROVE_CITY", destWarp: 1},
		},
	},
	{
		name: "CHERRYGROVE_POKECENTER_1F",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "CHERRYGROVE_CITY", destWarp: 2},
			{x: 4, y: 7, dest: "CHERRYGROVE_CITY", destWarp: 2},
			{x: 0, y: 7, dest: "POKECENTER_2F", destWarp: 1},
		},
	},
	{
		name: "ROUTE_30",
		warps: []warpSpec{
			{x: 7, y: 39, dest: "ROUTE_30_BERRY_HOUSE", destWarp: 1},
			{x: 17, y: 5, dest: "MR_POKEMONS_HOUSE", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_31", offset: -10},
			{dir: dirSouth, dest: "CHERRYGROVE_CITY", offset: -5},
		},
	},
	{
		name: "ROUTE_30_BERRY_HOUSE",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "ROUTE_30", destWarp: 1},
			{x: 3, y: 7, dest: "ROUTE_30", destWarp: 1},
		},
	},
	{
		name: "MR_POKEMONS_HOUSE",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "ROUTE_30", destWarp: 2},
			{x: 3, y: 7, dest: "ROUTE_30", destWarp: 2},
		},
	},
	{
		name: "ROUTE_31",
		warps: []warpSpec{
			{x: 4, y: 6, dest: "ROUTE_31_VIOLET_GATE", destWarp: 3},
			{x: 4, y: 7, dest: "ROUTE_31_VIOLET_GATE", destWarp: 4},
			{x: 34, y: 5, dest: "DARK_CAVE_VIOLET_ENTRANCE", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirSouth, dest: "ROUTE_30", offset: 10},
			{dir: dirWest, dest: "VIOLET_CITY", offset: -9},
		},
	},
	{
		name: "ROUTE_31_VIOLET_GATE",
		warps: []warpSpec{
			{x: 0, y: 4, dest: "VIOLET_CITY", destWarp: 8},
			{x: 0, y: 5, dest: "VIOLET_CITY", destWarp: 9},
			{x: 9, y: 4, dest: "ROUTE_31", destWarp: 1},
			{x: 9, y: 5, dest: "ROUTE_31", destWarp: 2},
		},
	},
	{
		name: "VIOLET_CITY",
		warps: []warpSpec{
			{x: 9, y: 17, dest: "VIOLET_MART", destWarp: 2},
			{x: 18, y: 17, dest: "VIOLET_GYM", destWarp: 1},
			{x: 30, y: 17, dest: "EARLS_POKEMON_ACADEMY", destWarp: 1},
			{x: 3, y: 15, dest: "VIOLET_NICKNAME_SPEECH_HOUSE", destWarp: 1},
			{x: 31, y: 25, dest: "VIOLET_POKECENTER_1F", destWarp: 1},
			{x: 21, y: 29, dest: "VIOLET_KYLES_HOUSE", destWarp: 1},
			{x: 23, y: 5, dest: "SPROUT_TOWER_1F", destWarp: 1},
			{x: 39, y: 24, dest: "ROUTE_31_VIOLET_GATE", destWarp: 1},
			{x: 39, y: 25, dest: "ROUTE_31_VIOLET_GATE", destWarp: 2},
		},
		connections: []connectionSpec{
			{dir: dirSouth, dest: "ROUTE_32", offset: 0},
			{dir: dirWest, dest: "ROUTE_36", offset: 0},
			{dir: dirEast, dest: "ROUTE_31", offset: 9},
		},
	},
	{
		name: "VIOLET_MART",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "VIOLET_CITY", destWarp: 1},
			{x: 3, y: 7, dest: "VIOLET_CITY", destWarp: 1},
		},
	},
	{
		name: "VIOLET_POKECENTER_1F",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "VIOLET_CITY", destWarp: 5},
			{x: 4, y: 7, dest: "VIOLET_CITY", destWarp: 5},
			{x: 0, y: 7, dest: "POKECENTER_2F", destWarp: 1},
		},
	},
	{
		name: "VIOLET_GYM",
		warps: []warpSpec{
			{x: 4, y: 15, dest: "VIOLET_CITY", destWarp: 2},
			{x: 5, y: 15, dest: "VIOLET_CITY", destWarp: 2},
		},
	},
	{
		name: "SPROUT_TOWER_1F",
		warps: []warpSpec{
			{x: 9, y: 15, dest: "VIOLET_CITY", destWarp: 7},
			{x: 10, y: 15, dest: "VIOLET_CITY", destWarp: 7},
			{x: 6, y: 4, dest: "SPROUT_TOWER_2F", destWarp: 1},
			{x: 2, y: 6, dest: "SPROUT_TOWER_2F", destWarp: 2},
			{x: 17, y: 3, dest: "SPROUT_TOWER_2F", destWarp: 3},
		},
	},
	{
		name: "SPROUT_TOWER_2F",
		warps: []warpSpec{
			{x: 6, y: 4, dest: "SPROUT_TOWER_1F", destWarp: 3},
			{x: 2, y: 6, dest: "SPROUT_TOWER_1F", destWarp: 4},
			{x: 17, y: 3, dest: "SPROUT_TOWER_1F", destWarp: 5},
			{x: 10, y: 14, dest: "SPROUT_TOWER_3F", destWarp: 1},
		},
	},
	{
		name: "SPROUT_TOWER_3F",
		warps: []warpSpec{
			{x: 10, y: 14, dest: "SPROUT_TOWER_2F", destWarp: 4},
		},
	},
	{
		name: "ROUTE_32",
		warps: []warpSpec{
			{x: 6, y: 79, dest: "UNION_CAVE_1F", destWarp: 4},
		},
		// The decomp also declares a south connection directly to Route 33,
		// but the executable badge corridor is the cave warp. Keep the partial
		// provider honest instead of offering an abstract edge whose landing is
		// not reachable through the live collision grid.
		connections: []connectionSpec{
			{dir: dirNorth, dest: "VIOLET_CITY", offset: 0},
		},
	},
	{
		name: "UNION_CAVE_1F",
		warps: []warpSpec{
			{x: 17, y: 31, dest: "ROUTE_33", destWarp: 1},
			{x: 17, y: 3, dest: "ROUTE_32", destWarp: 4},
		},
	},
	{
		name: "ROUTE_33",
		warps: []warpSpec{
			{x: 11, y: 9, dest: "UNION_CAVE_1F", destWarp: 3},
		},
		connections: []connectionSpec{
			{dir: dirWest, dest: "AZALEA_TOWN", offset: 0},
		},
	},
	{
		name: "AZALEA_TOWN",
		warps: []warpSpec{
			{x: 15, y: 9, dest: "AZALEA_POKECENTER_1F", destWarp: 1},
			{x: 9, y: 5, dest: "KURTS_HOUSE", destWarp: 1},
			{x: 10, y: 15, dest: "AZALEA_GYM", destWarp: 1},
			{x: 31, y: 7, dest: "SLOWPOKE_WELL_B1F", destWarp: 1},
			{x: 2, y: 10, dest: "ILEX_FOREST_AZALEA_GATE", destWarp: 3},
			{x: 2, y: 11, dest: "ILEX_FOREST_AZALEA_GATE", destWarp: 4},
		},
		connections: []connectionSpec{
			{dir: dirEast, dest: "ROUTE_33", offset: 0},
		},
	},
	{
		name: "AZALEA_POKECENTER_1F",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "AZALEA_TOWN", destWarp: 1},
			{x: 4, y: 7, dest: "AZALEA_TOWN", destWarp: 1},
		},
	},
	{
		name: "AZALEA_GYM",
		warps: []warpSpec{
			{x: 4, y: 15, dest: "AZALEA_TOWN", destWarp: 5},
			{x: 5, y: 15, dest: "AZALEA_TOWN", destWarp: 5},
		},
	},
	{
		name: "KURTS_HOUSE",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "AZALEA_TOWN", destWarp: 4},
			{x: 4, y: 7, dest: "AZALEA_TOWN", destWarp: 4},
		},
	},
	{
		name: "SLOWPOKE_WELL_B1F",
		warps: []warpSpec{
			{x: 17, y: 15, dest: "AZALEA_TOWN", destWarp: 6},
		},
	},
	{
		name: "ILEX_FOREST_AZALEA_GATE",
		warps: []warpSpec{
			{x: 0, y: 4, dest: "ILEX_FOREST", destWarp: 2},
			{x: 0, y: 5, dest: "ILEX_FOREST", destWarp: 3},
			{x: 9, y: 4, dest: "AZALEA_TOWN", destWarp: 7},
			{x: 9, y: 5, dest: "AZALEA_TOWN", destWarp: 8},
		},
	},
	{
		name: "ILEX_FOREST",
		warps: []warpSpec{
			{x: 1, y: 5, dest: "ROUTE_34_ILEX_FOREST_GATE", destWarp: 3},
			{x: 3, y: 42, dest: "ILEX_FOREST_AZALEA_GATE", destWarp: 1},
			{x: 3, y: 43, dest: "ILEX_FOREST_AZALEA_GATE", destWarp: 2},
		},
	},
	{
		name: "ROUTE_34_ILEX_FOREST_GATE",
		warps: []warpSpec{
			{x: 4, y: 0, dest: "ROUTE_34", destWarp: 1},
			{x: 5, y: 0, dest: "ROUTE_34", destWarp: 2},
			{x: 4, y: 7, dest: "ILEX_FOREST", destWarp: 1},
			{x: 5, y: 7, dest: "ILEX_FOREST", destWarp: 1},
		},
	},
	{
		name: "ROUTE_34",
		warps: []warpSpec{
			{x: 13, y: 37, dest: "ROUTE_34_ILEX_FOREST_GATE", destWarp: 1},
			{x: 14, y: 37, dest: "ROUTE_34_ILEX_FOREST_GATE", destWarp: 2},
		},
		// The decomp also connects Route 34 east to Azalea. The executable
		// story corridor reaches that side through Ilex Forest, so expose only
		// the north connection here rather than a static shortcut around Cut.
		connections: []connectionSpec{
			{dir: dirNorth, dest: "GOLDENROD_CITY", offset: -5},
		},
	},
	{
		name: "GOLDENROD_CITY",
		warps: []warpSpec{
			{x: 24, y: 7, dest: "GOLDENROD_GYM", destWarp: 1},
			{x: 29, y: 29, dest: "GOLDENROD_BIKE_SHOP", destWarp: 1},
			{x: 31, y: 21, dest: "GOLDENROD_HAPPINESS_RATER", destWarp: 1},
			{x: 5, y: 25, dest: "BILLS_FAMILYS_HOUSE", destWarp: 1},
			{x: 9, y: 13, dest: "GOLDENROD_MAGNET_TRAIN_STATION", destWarp: 2},
			{x: 33, y: 5, dest: "GOLDENROD_FLOWER_SHOP", destWarp: 1},
			{x: 15, y: 27, dest: "GOLDENROD_POKECENTER_1F", destWarp: 1},
			{x: 33, y: 9, dest: "GOLDENROD_PP_SPEECH_HOUSE", destWarp: 1},
			{x: 15, y: 7, dest: "GOLDENROD_NAME_RATER", destWarp: 1},
			{x: 24, y: 27, dest: "GOLDENROD_DEPT_STORE_1F", destWarp: 1},
			{x: 14, y: 21, dest: "GOLDENROD_GAME_CORNER", destWarp: 1},
			{x: 5, y: 15, dest: "RADIO_TOWER_1F", destWarp: 1},
			{x: 19, y: 1, dest: "ROUTE_35_GOLDENROD_GATE", destWarp: 3},
			{x: 9, y: 5, dest: "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", destWarp: 8},
			{x: 11, y: 29, dest: "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", destWarp: 5},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_35", offset: 5},
			{dir: dirSouth, dest: "ROUTE_34", offset: 5},
		},
	},
	{
		name: "GOLDENROD_GYM",
		warps: []warpSpec{
			{x: 2, y: 17, dest: "GOLDENROD_CITY", destWarp: 1},
			{x: 3, y: 17, dest: "GOLDENROD_CITY", destWarp: 1},
		},
	},
	{
		name: "GOLDENROD_FLOWER_SHOP",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "GOLDENROD_CITY", destWarp: 6},
			{x: 3, y: 7, dest: "GOLDENROD_CITY", destWarp: 6},
		},
	},
	{
		name: "GOLDENROD_POKECENTER_1F",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "GOLDENROD_CITY", destWarp: 7},
			{x: 4, y: 7, dest: "GOLDENROD_CITY", destWarp: 7},
			{x: 0, y: 7, dest: "POKECENTER_2F", destWarp: 1},
		},
	},
	{
		name: "GOLDENROD_DEPT_STORE_1F",
		warps: []warpSpec{
			{x: 7, y: 7, dest: "GOLDENROD_CITY", destWarp: 10},
			{x: 8, y: 7, dest: "GOLDENROD_CITY", destWarp: 10},
			{x: 15, y: 0, dest: "GOLDENROD_DEPT_STORE_2F", destWarp: 2},
			{x: 2, y: 0, dest: "GOLDENROD_DEPT_STORE_ELEVATOR", destWarp: 1},
		},
	},
	{
		name: "ROUTE_35_GOLDENROD_GATE",
		warps: []warpSpec{
			{x: 4, y: 0, dest: "ROUTE_35", destWarp: 1},
			{x: 5, y: 0, dest: "ROUTE_35", destWarp: 2},
			{x: 4, y: 7, dest: "GOLDENROD_CITY", destWarp: 13},
			{x: 5, y: 7, dest: "GOLDENROD_CITY", destWarp: 13},
		},
	},
	{
		name: "ROUTE_35",
		warps: []warpSpec{
			{x: 9, y: 33, dest: "ROUTE_35_GOLDENROD_GATE", destWarp: 1},
			{x: 10, y: 33, dest: "ROUTE_35_GOLDENROD_GATE", destWarp: 2},
			{x: 3, y: 5, dest: "ROUTE_35_NATIONAL_PARK_GATE", destWarp: 3},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_36", offset: 0},
			{dir: dirSouth, dest: "GOLDENROD_CITY", offset: -5},
		},
	},
	{
		name: "ROUTE_36",
		warps: []warpSpec{
			{x: 18, y: 8, dest: "ROUTE_36_NATIONAL_PARK_GATE", destWarp: 3},
			{x: 18, y: 9, dest: "ROUTE_36_NATIONAL_PARK_GATE", destWarp: 4},
			{x: 47, y: 13, dest: "ROUTE_36_RUINS_OF_ALPH_GATE", destWarp: 1},
			{x: 48, y: 13, dest: "ROUTE_36_RUINS_OF_ALPH_GATE", destWarp: 2},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_37", offset: 10},
			{dir: dirSouth, dest: "ROUTE_35", offset: 0},
			{dir: dirEast, dest: "VIOLET_CITY", offset: 0},
		},
	},
	{
		name: "ROUTE_37",
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ECRUTEAK_CITY", offset: -5},
			{dir: dirSouth, dest: "ROUTE_36", offset: -10},
		},
	},
	{
		name: "ECRUTEAK_CITY",
		warps: []warpSpec{
			{x: 35, y: 26, dest: "ROUTE_42_ECRUTEAK_GATE", destWarp: 1},
			{x: 35, y: 27, dest: "ROUTE_42_ECRUTEAK_GATE", destWarp: 2},
			{x: 18, y: 11, dest: "ECRUTEAK_TIN_TOWER_ENTRANCE", destWarp: 1},
			{x: 20, y: 2, dest: "ECRUTEAK_TIN_TOWER_BACK_ENTRANCE", destWarp: 1},
			{x: 20, y: 3, dest: "ECRUTEAK_TIN_TOWER_BACK_ENTRANCE", destWarp: 2},
			{x: 23, y: 27, dest: "ECRUTEAK_POKECENTER_1F", destWarp: 1},
			{x: 5, y: 21, dest: "ECRUTEAK_LUGIA_SPEECH_HOUSE", destWarp: 1},
			{x: 23, y: 21, dest: "DANCE_THEATER", destWarp: 1},
			{x: 29, y: 21, dest: "ECRUTEAK_MART", destWarp: 2},
			{x: 6, y: 27, dest: "ECRUTEAK_GYM", destWarp: 1},
			{x: 13, y: 27, dest: "ECRUTEAK_ITEMFINDER_HOUSE", destWarp: 1},
			{x: 37, y: 7, dest: "TIN_TOWER_1F", destWarp: 1},
			{x: 5, y: 5, dest: "BURNED_TOWER_1F", destWarp: 1},
			{x: 0, y: 18, dest: "ROUTE_38_ECRUTEAK_GATE", destWarp: 3},
			{x: 0, y: 19, dest: "ROUTE_38_ECRUTEAK_GATE", destWarp: 4},
		},
		connections: []connectionSpec{
			{dir: dirSouth, dest: "ROUTE_37", offset: 5},
			{dir: dirWest, dest: "ROUTE_38", offset: 5},
			{dir: dirEast, dest: "ROUTE_42", offset: 9},
		},
	},
	{
		name: "BURNED_TOWER_1F",
		warps: []warpSpec{
			{x: 9, y: 15, dest: "ECRUTEAK_CITY", destWarp: 13},
			{x: 10, y: 15, dest: "ECRUTEAK_CITY", destWarp: 13},
			{x: 5, y: 4, dest: "BURNED_TOWER_B1F", destWarp: 1},
			{x: 5, y: 5, dest: "BURNED_TOWER_B1F", destWarp: 1},
			{x: 5, y: 6, dest: "BURNED_TOWER_B1F", destWarp: 1},
			{x: 4, y: 6, dest: "BURNED_TOWER_B1F", destWarp: 1},
			{x: 15, y: 4, dest: "BURNED_TOWER_B1F", destWarp: 2},
			{x: 15, y: 5, dest: "BURNED_TOWER_B1F", destWarp: 2},
			{x: 10, y: 7, dest: "BURNED_TOWER_B1F", destWarp: 3},
			{x: 5, y: 14, dest: "BURNED_TOWER_B1F", destWarp: 4},
			{x: 4, y: 14, dest: "BURNED_TOWER_B1F", destWarp: 4},
			{x: 14, y: 14, dest: "BURNED_TOWER_B1F", destWarp: 5},
			{x: 15, y: 14, dest: "BURNED_TOWER_B1F", destWarp: 5},
			{x: 7, y: 15, dest: "BURNED_TOWER_B1F", destWarp: 6},
		},
	},
	{
		name: "BURNED_TOWER_B1F",
		warps: []warpSpec{
			{x: 3, y: 3, dest: "BURNED_TOWER_1F", destWarp: 3},
			{x: 17, y: 7, dest: "BURNED_TOWER_1F", destWarp: 7},
			{x: 10, y: 8, dest: "BURNED_TOWER_1F", destWarp: 9},
			{x: 3, y: 13, dest: "BURNED_TOWER_1F", destWarp: 10},
			{x: 17, y: 14, dest: "BURNED_TOWER_1F", destWarp: 12},
			{x: 7, y: 15, dest: "BURNED_TOWER_1F", destWarp: 14},
		},
	},
	{
		name: "ECRUTEAK_POKECENTER_1F",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "ECRUTEAK_CITY", destWarp: 6},
			{x: 4, y: 7, dest: "ECRUTEAK_CITY", destWarp: 6},
			{x: 0, y: 7, dest: "POKECENTER_2F", destWarp: 1},
		},
	},
	{
		name: "ECRUTEAK_MART",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "ECRUTEAK_CITY", destWarp: 9},
			{x: 3, y: 7, dest: "ECRUTEAK_CITY", destWarp: 9},
		},
	},
	{
		name: "ECRUTEAK_GYM",
		warps: []warpSpec{
			{x: 4, y: 17, dest: "ECRUTEAK_CITY", destWarp: 10},
			{x: 5, y: 17, dest: "ECRUTEAK_CITY", destWarp: 10},
			{x: 4, y: 14, dest: "ECRUTEAK_GYM", destWarp: 4},
			{x: 2, y: 4, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 3, y: 4, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 4, y: 4, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 4, y: 5, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 6, y: 7, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 7, y: 4, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 2, y: 6, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 3, y: 6, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 4, y: 6, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 5, y: 6, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 7, y: 6, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 7, y: 7, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 4, y: 8, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 5, y: 8, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 6, y: 8, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 7, y: 8, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 2, y: 8, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 2, y: 9, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 2, y: 10, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 2, y: 11, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 4, y: 10, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 5, y: 10, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 2, y: 12, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 3, y: 12, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 4, y: 12, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 5, y: 12, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 7, y: 10, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 7, y: 11, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 7, y: 12, dest: "ECRUTEAK_GYM", destWarp: 3},
			{x: 7, y: 13, dest: "ECRUTEAK_GYM", destWarp: 3},
		},
	},
}

type firstBadgeWorldProvider struct {
	ids   []uint16
	specs map[uint16]mapSpec
}

// NewFirstBadgeWorldProvider returns the verified Gold/Silver topology needed
// for the verified early-Johto slice from a fresh save through Ecruteak.
//
// romData is accepted so the profile-facing constructor stays stable while
// NewWorldProvider owns the full ROM parse. The current slice uses generated
// catalog identities plus decomp-verified topology facts.
func NewFirstBadgeWorldProvider(romData []byte) worldmodel.NativeGridProvider {
	_ = romData
	specs := make(map[uint16]mapSpec, len(firstBadgeTopology))
	ids := make([]uint16, 0, len(firstBadgeTopology))
	for _, spec := range firstBadgeTopology {
		info, ok := gsdata.MapByName(spec.name)
		if !ok {
			continue
		}
		id := gsdata.NativeMapID(info.Group, info.Number)
		specs[id] = spec
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return &firstBadgeWorldProvider{ids: ids, specs: specs}
}

func (p *firstBadgeWorldProvider) MapIDs() []uint16 {
	if p == nil {
		return nil
	}
	return append([]uint16(nil), p.ids...)
}

func (p *firstBadgeWorldProvider) ParseMap(mapID uint16) (worldmodel.NativeMapHeader, error) {
	if p == nil {
		return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: nil first-badge world provider")
	}
	spec, ok := p.specs[mapID]
	if !ok {
		return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: map %#04x is outside first-badge topology slice", mapID)
	}
	info, ok := gsdata.Map(mapID)
	if !ok {
		return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: generated map %#04x is missing", mapID)
	}

	header := worldmodel.NativeMapHeader{
		ID:           mapID,
		WidthBlocks:  info.WidthBlocks,
		HeightBlocks: info.HeightBlocks,
		Warps:        make([]worldmodel.NativeWarp, 0, len(spec.warps)),
		Connections:  make([]worldmodel.NativeConnection, 0, len(spec.connections)),
	}
	for _, warp := range spec.warps {
		dest, ok := gsdata.MapByName(warp.dest)
		if !ok {
			return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: %s warp destination %q missing from generated catalog", spec.name, warp.dest)
		}
		if warp.destWarp == 0 {
			return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: %s warp at (%d,%d) has zero decomp destination warp", spec.name, warp.x, warp.y)
		}
		header.Warps = append(header.Warps, worldmodel.NativeWarp{
			X:          warp.x,
			Y:          warp.y,
			DestWarpID: warp.destWarp - 1,
			DestMap:    gsdata.NativeMapID(dest.Group, dest.Number),
		})
	}
	for _, connection := range spec.connections {
		dest, ok := gsdata.MapByName(connection.dest)
		if !ok {
			return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: %s connection destination %q missing from generated catalog", spec.name, connection.dest)
		}
		header.Connections = append(header.Connections, worldmodel.NativeConnection{
			Dir:    connection.dir,
			MapID:  gsdata.NativeMapID(dest.Group, dest.Number),
			Offset: connection.offset,
		})
	}
	return header, nil
}
