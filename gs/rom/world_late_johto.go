package rom

// Late Johto native topology transcribed from pret/pokegold map-event warps
// and map-attribute connections. These edges are geometry facts, NOT proof
// of Ice Path puzzle, Radio Tower, or Clair story-script execution.
var lateJohtoTopology = []mapSpec{
	// Radio Tower takeover geometry. Reaching any floor or warehouse room is
	// NOT evidence of opening the Card Key shutter, solving basement switches
	// or clearing Rocket. Those must be owned by separate story executors.
	{
		name: "RADIO_TOWER_1F",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "GOLDENROD_CITY", destWarp: 12},
			{x: 3, y: 7, dest: "GOLDENROD_CITY", destWarp: 12},
			{x: 15, y: 0, dest: "RADIO_TOWER_2F", destWarp: 2},
		},
	},
	{
		name: "RADIO_TOWER_2F",
		warps: []warpSpec{
			{x: 0, y: 0, dest: "RADIO_TOWER_3F", destWarp: 1},
			{x: 15, y: 0, dest: "RADIO_TOWER_1F", destWarp: 3},
		},
	},
	{
		name: "RADIO_TOWER_3F",
		warps: []warpSpec{
			{x: 0, y: 0, dest: "RADIO_TOWER_2F", destWarp: 1},
			{x: 7, y: 0, dest: "RADIO_TOWER_4F", destWarp: 2},
			{x: 17, y: 0, dest: "RADIO_TOWER_4F", destWarp: 4},
		},
	},
	{
		name: "RADIO_TOWER_4F",
		warps: []warpSpec{
			{x: 0, y: 0, dest: "RADIO_TOWER_5F", destWarp: 1},
			{x: 9, y: 0, dest: "RADIO_TOWER_3F", destWarp: 2},
			{x: 12, y: 0, dest: "RADIO_TOWER_5F", destWarp: 2},
			{x: 17, y: 0, dest: "RADIO_TOWER_3F", destWarp: 3},
		},
	},
	{
		name: "RADIO_TOWER_5F",
		warps: []warpSpec{
			{x: 0, y: 0, dest: "RADIO_TOWER_4F", destWarp: 1},
			{x: 12, y: 0, dest: "RADIO_TOWER_4F", destWarp: 3},
		},
	},
	{
		name: "GOLDENROD_UNDERGROUND",
		warps: []warpSpec{
			{x: 3, y: 2, dest: "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", destWarp: 7},
			{x: 3, y: 34, dest: "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", destWarp: 4},
			{x: 18, y: 6, dest: "GOLDENROD_UNDERGROUND", destWarp: 4},
			{x: 21, y: 31, dest: "GOLDENROD_UNDERGROUND", destWarp: 3},
			{x: 22, y: 31, dest: "GOLDENROD_UNDERGROUND", destWarp: 3},
			{x: 22, y: 27, dest: "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", destWarp: 1},
		},
	},
	{
		name: "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES",
		warps: []warpSpec{
			{x: 23, y: 3, dest: "GOLDENROD_UNDERGROUND", destWarp: 6},
			{x: 22, y: 10, dest: "GOLDENROD_UNDERGROUND_WAREHOUSE", destWarp: 1},
			{x: 23, y: 10, dest: "GOLDENROD_UNDERGROUND_WAREHOUSE", destWarp: 2},
			{x: 5, y: 25, dest: "GOLDENROD_UNDERGROUND", destWarp: 2},
			{x: 4, y: 29, dest: "GOLDENROD_CITY", destWarp: 15},
			{x: 5, y: 29, dest: "GOLDENROD_CITY", destWarp: 15},
			{x: 21, y: 25, dest: "GOLDENROD_UNDERGROUND", destWarp: 1},
			{x: 20, y: 29, dest: "GOLDENROD_CITY", destWarp: 14},
			{x: 21, y: 29, dest: "GOLDENROD_CITY", destWarp: 14},
		},
	},
	{
		name: "GOLDENROD_UNDERGROUND_WAREHOUSE",
		warps: []warpSpec{
			{x: 2, y: 12, dest: "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", destWarp: 2},
			{x: 3, y: 12, dest: "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES", destWarp: 3},
			{x: 17, y: 2, dest: "GOLDENROD_DEPT_STORE_B1F", destWarp: 1},
		},
	},
	{
		name: "ROUTE_44",
		warps: []warpSpec{
			{x: 56, y: 7, dest: "ICE_PATH_1F", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirWest, dest: "MAHOGANY_TOWN", offset: 0},
			{dir: dirEast, dest: "BLACKTHORN_CITY", offset: -9},
		},
	},
	{
		name: "ICE_PATH_1F",
		warps: []warpSpec{
			{x: 4, y: 19, dest: "ROUTE_44", destWarp: 1},
			{x: 36, y: 27, dest: "BLACKTHORN_CITY", destWarp: 7},
			{x: 37, y: 5, dest: "ICE_PATH_B1F", destWarp: 1},
			{x: 37, y: 13, dest: "ICE_PATH_B1F", destWarp: 7},
		},
	},
	{
		name: "ICE_PATH_B1F",
		warps: []warpSpec{
			{x: 3, y: 15, dest: "ICE_PATH_1F", destWarp: 3},
			{x: 17, y: 3, dest: "ICE_PATH_B2F_MAHOGANY_SIDE", destWarp: 1},
			{x: 11, y: 2, dest: "ICE_PATH_B2F_MAHOGANY_SIDE", destWarp: 3},
			{x: 4, y: 7, dest: "ICE_PATH_B2F_MAHOGANY_SIDE", destWarp: 4},
			{x: 5, y: 12, dest: "ICE_PATH_B2F_MAHOGANY_SIDE", destWarp: 5},
			{x: 12, y: 13, dest: "ICE_PATH_B2F_MAHOGANY_SIDE", destWarp: 6},
			{x: 5, y: 25, dest: "ICE_PATH_1F", destWarp: 4},
			{x: 11, y: 27, dest: "ICE_PATH_B2F_BLACKTHORN_SIDE", destWarp: 1},
		},
	},
	{
		name: "ICE_PATH_B2F_MAHOGANY_SIDE",
		warps: []warpSpec{
			{x: 17, y: 1, dest: "ICE_PATH_B1F", destWarp: 2},
			{x: 9, y: 11, dest: "ICE_PATH_B3F", destWarp: 1},
			{x: 11, y: 4, dest: "ICE_PATH_B1F", destWarp: 3},
			{x: 4, y: 6, dest: "ICE_PATH_B1F", destWarp: 4},
			{x: 4, y: 12, dest: "ICE_PATH_B1F", destWarp: 5},
			{x: 12, y: 12, dest: "ICE_PATH_B1F", destWarp: 6},
		},
	},
	{
		name: "ICE_PATH_B2F_BLACKTHORN_SIDE",
		warps: []warpSpec{
			{x: 3, y: 15, dest: "ICE_PATH_B1F", destWarp: 8},
			{x: 3, y: 3, dest: "ICE_PATH_B3F", destWarp: 2},
		},
	},
	{
		name: "ICE_PATH_B3F",
		warps: []warpSpec{
			{x: 3, y: 5, dest: "ICE_PATH_B2F_MAHOGANY_SIDE", destWarp: 2},
			{x: 15, y: 5, dest: "ICE_PATH_B2F_BLACKTHORN_SIDE", destWarp: 2},
		},
	},
	{
		name: "BLACKTHORN_CITY",
		warps: []warpSpec{
			{x: 18, y: 11, dest: "BLACKTHORN_GYM_1F", destWarp: 1},
			{x: 13, y: 21, dest: "BLACKTHORN_DRAGON_SPEECH_HOUSE", destWarp: 1},
			{x: 29, y: 23, dest: "BLACKTHORN_EMYS_HOUSE", destWarp: 1},
			{x: 15, y: 29, dest: "BLACKTHORN_MART", destWarp: 2},
			{x: 21, y: 29, dest: "BLACKTHORN_POKECENTER_1F", destWarp: 1},
			{x: 9, y: 31, dest: "MOVE_DELETERS_HOUSE", destWarp: 1},
			{x: 36, y: 9, dest: "ICE_PATH_1F", destWarp: 2},
			{x: 20, y: 1, dest: "DRAGONS_DEN_1F", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirSouth, dest: "ROUTE_45", offset: 0},
			{dir: dirWest, dest: "ROUTE_44", offset: 9},
		},
	},
	{
		name: "BLACKTHORN_POKECENTER_1F",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "BLACKTHORN_CITY", destWarp: 5},
			{x: 4, y: 7, dest: "BLACKTHORN_CITY", destWarp: 5},
			{x: 0, y: 7, dest: "POKECENTER_2F", destWarp: 1},
		},
	},
	{
		name: "BLACKTHORN_MART",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "BLACKTHORN_CITY", destWarp: 4},
			{x: 3, y: 7, dest: "BLACKTHORN_CITY", destWarp: 4},
		},
	},
	{
		name: "BLACKTHORN_GYM_1F",
		warps: []warpSpec{
			{x: 4, y: 17, dest: "BLACKTHORN_CITY", destWarp: 1},
			{x: 5, y: 17, dest: "BLACKTHORN_CITY", destWarp: 1},
			{x: 1, y: 7, dest: "BLACKTHORN_GYM_2F", destWarp: 1},
			{x: 7, y: 9, dest: "BLACKTHORN_GYM_2F", destWarp: 2},
			{x: 2, y: 6, dest: "BLACKTHORN_GYM_2F", destWarp: 3},
			{x: 7, y: 7, dest: "BLACKTHORN_GYM_2F", destWarp: 4},
			{x: 7, y: 6, dest: "BLACKTHORN_GYM_2F", destWarp: 5},
		},
	},
	{
		name: "BLACKTHORN_GYM_2F",
		warps: []warpSpec{
			{x: 1, y: 7, dest: "BLACKTHORN_GYM_1F", destWarp: 3},
			{x: 7, y: 9, dest: "BLACKTHORN_GYM_1F", destWarp: 4},
			{x: 2, y: 5, dest: "BLACKTHORN_GYM_1F", destWarp: 5},
			{x: 8, y: 7, dest: "BLACKTHORN_GYM_1F", destWarp: 6},
			{x: 8, y: 3, dest: "BLACKTHORN_GYM_1F", destWarp: 7},
		},
	},
	{
		name: "DRAGONS_DEN_1F",
		warps: []warpSpec{
			{x: 3, y: 5, dest: "BLACKTHORN_CITY", destWarp: 8},
			{x: 3, y: 3, dest: "DRAGONS_DEN_1F", destWarp: 4},
			{x: 5, y: 15, dest: "DRAGONS_DEN_B1F", destWarp: 1},
			{x: 5, y: 13, dest: "DRAGONS_DEN_1F", destWarp: 2},
		},
	},
	{
		name: "DRAGONS_DEN_B1F",
		warps: []warpSpec{
			{x: 20, y: 3, dest: "DRAGONS_DEN_1F", destWarp: 3},
		},
	},
}
