// Code generated from the canonical Pokémon Yellow EN rev0 symbol/layout data.
// Source structure: pret/pokeyellow @ e89ead1 (vendored as pokeyellow/).
// Do not put these native ids/addresses in generic packages.
package rom

import "github.com/maestroi/pokepilot/gen1rom"

var yellowMapNames = [0xF9]string{
	0x00: "PALLET_TOWN",
	0x01: "VIRIDIAN_CITY",
	0x02: "PEWTER_CITY",
	0x03: "CERULEAN_CITY",
	0x04: "LAVENDER_TOWN",
	0x05: "VERMILION_CITY",
	0x06: "CELADON_CITY",
	0x07: "FUCHSIA_CITY",
	0x08: "CINNABAR_ISLAND",
	0x09: "INDIGO_PLATEAU",
	0x0a: "SAFFRON_CITY",
	0x0b: "UNUSED_MAP_0B",
	0x0c: "ROUTE_1",
	0x0d: "ROUTE_2",
	0x0e: "ROUTE_3",
	0x0f: "ROUTE_4",
	0x10: "ROUTE_5",
	0x11: "ROUTE_6",
	0x12: "ROUTE_7",
	0x13: "ROUTE_8",
	0x14: "ROUTE_9",
	0x15: "ROUTE_10",
	0x16: "ROUTE_11",
	0x17: "ROUTE_12",
	0x18: "ROUTE_13",
	0x19: "ROUTE_14",
	0x1a: "ROUTE_15",
	0x1b: "ROUTE_16",
	0x1c: "ROUTE_17",
	0x1d: "ROUTE_18",
	0x1e: "ROUTE_19",
	0x1f: "ROUTE_20",
	0x20: "ROUTE_21",
	0x21: "ROUTE_22",
	0x22: "ROUTE_23",
	0x23: "ROUTE_24",
	0x24: "ROUTE_25",
	0x25: "REDS_HOUSE_1F",
	0x26: "REDS_HOUSE_2F",
	0x27: "BLUES_HOUSE",
	0x28: "OAKS_LAB",
	0x29: "VIRIDIAN_POKECENTER",
	0x2a: "VIRIDIAN_MART",
	0x2b: "VIRIDIAN_SCHOOL_HOUSE",
	0x2c: "VIRIDIAN_NICKNAME_HOUSE",
	0x2d: "VIRIDIAN_GYM",
	0x2e: "DIGLETTS_CAVE_ROUTE_2",
	0x2f: "VIRIDIAN_FOREST_NORTH_GATE",
	0x30: "ROUTE_2_TRADE_HOUSE",
	0x31: "ROUTE_2_GATE",
	0x32: "VIRIDIAN_FOREST_SOUTH_GATE",
	0x33: "VIRIDIAN_FOREST",
	0x34: "MUSEUM_1F",
	0x35: "MUSEUM_2F",
	0x36: "PEWTER_GYM",
	0x37: "PEWTER_NIDORAN_HOUSE",
	0x38: "PEWTER_MART",
	0x39: "PEWTER_SPEECH_HOUSE",
	0x3a: "PEWTER_POKECENTER",
	0x3b: "MT_MOON_1F",
	0x3c: "MT_MOON_B1F",
	0x3d: "MT_MOON_B2F",
	0x3e: "CERULEAN_TRASHED_HOUSE",
	0x3f: "CERULEAN_MELANIES_HOUSE",
	0x40: "CERULEAN_POKECENTER",
	0x41: "CERULEAN_GYM",
	0x42: "BIKE_SHOP",
	0x43: "CERULEAN_MART",
	0x44: "MT_MOON_POKECENTER",
	0x45: "CERULEAN_TRASHED_HOUSE_COPY",
	0x46: "ROUTE_5_GATE",
	0x47: "UNDERGROUND_PATH_ROUTE_5",
	0x48: "DAYCARE",
	0x49: "ROUTE_6_GATE",
	0x4a: "UNDERGROUND_PATH_ROUTE_6",
	0x4b: "UNDERGROUND_PATH_ROUTE_6_COPY",
	0x4c: "ROUTE_7_GATE",
	0x4d: "UNDERGROUND_PATH_ROUTE_7",
	0x4e: "UNDERGROUND_PATH_ROUTE_7_COPY",
	0x4f: "ROUTE_8_GATE",
	0x50: "UNDERGROUND_PATH_ROUTE_8",
	0x51: "ROCK_TUNNEL_POKECENTER",
	0x52: "ROCK_TUNNEL_1F",
	0x53: "POWER_PLANT",
	0x54: "ROUTE_11_GATE_1F",
	0x55: "DIGLETTS_CAVE_ROUTE_11",
	0x56: "ROUTE_11_GATE_2F",
	0x57: "ROUTE_12_GATE_1F",
	0x58: "BILLS_HOUSE",
	0x59: "VERMILION_POKECENTER",
	0x5a: "POKEMON_FAN_CLUB",
	0x5b: "VERMILION_MART",
	0x5c: "VERMILION_GYM",
	0x5d: "VERMILION_PIDGEY_HOUSE",
	0x5e: "VERMILION_DOCK",
	0x5f: "SS_ANNE_1F",
	0x60: "SS_ANNE_2F",
	0x61: "SS_ANNE_3F",
	0x62: "SS_ANNE_B1F",
	0x63: "SS_ANNE_BOW",
	0x64: "SS_ANNE_KITCHEN",
	0x65: "SS_ANNE_CAPTAINS_ROOM",
	0x66: "SS_ANNE_1F_ROOMS",
	0x67: "SS_ANNE_2F_ROOMS",
	0x68: "SS_ANNE_B1F_ROOMS",
	0x69: "UNUSED_MAP_69",
	0x6a: "UNUSED_MAP_6A",
	0x6b: "UNUSED_MAP_6B",
	0x6c: "VICTORY_ROAD_1F",
	0x6d: "UNUSED_MAP_6D",
	0x6e: "UNUSED_MAP_6E",
	0x6f: "UNUSED_MAP_6F",
	0x70: "UNUSED_MAP_70",
	0x71: "LANCES_ROOM",
	0x72: "UNUSED_MAP_72",
	0x73: "UNUSED_MAP_73",
	0x74: "UNUSED_MAP_74",
	0x75: "UNUSED_MAP_75",
	0x76: "HALL_OF_FAME",
	0x77: "UNDERGROUND_PATH_NORTH_SOUTH",
	0x78: "CHAMPIONS_ROOM",
	0x79: "UNDERGROUND_PATH_WEST_EAST",
	0x7a: "CELADON_MART_1F",
	0x7b: "CELADON_MART_2F",
	0x7c: "CELADON_MART_3F",
	0x7d: "CELADON_MART_4F",
	0x7e: "CELADON_MART_ROOF",
	0x7f: "CELADON_MART_ELEVATOR",
	0x80: "CELADON_MANSION_1F",
	0x81: "CELADON_MANSION_2F",
	0x82: "CELADON_MANSION_3F",
	0x83: "CELADON_MANSION_ROOF",
	0x84: "CELADON_MANSION_ROOF_HOUSE",
	0x85: "CELADON_POKECENTER",
	0x86: "CELADON_GYM",
	0x87: "GAME_CORNER",
	0x88: "CELADON_MART_5F",
	0x89: "GAME_CORNER_PRIZE_ROOM",
	0x8a: "CELADON_DINER",
	0x8b: "CELADON_CHIEF_HOUSE",
	0x8c: "CELADON_HOTEL",
	0x8d: "LAVENDER_POKECENTER",
	0x8e: "POKEMON_TOWER_1F",
	0x8f: "POKEMON_TOWER_2F",
	0x90: "POKEMON_TOWER_3F",
	0x91: "POKEMON_TOWER_4F",
	0x92: "POKEMON_TOWER_5F",
	0x93: "POKEMON_TOWER_6F",
	0x94: "POKEMON_TOWER_7F",
	0x95: "MR_FUJIS_HOUSE",
	0x96: "LAVENDER_MART",
	0x97: "LAVENDER_CUBONE_HOUSE",
	0x98: "FUCHSIA_MART",
	0x99: "FUCHSIA_BILLS_GRANDPAS_HOUSE",
	0x9a: "FUCHSIA_POKECENTER",
	0x9b: "WARDENS_HOUSE",
	0x9c: "SAFARI_ZONE_GATE",
	0x9d: "FUCHSIA_GYM",
	0x9e: "FUCHSIA_MEETING_ROOM",
	0x9f: "SEAFOAM_ISLANDS_B1F",
	0xa0: "SEAFOAM_ISLANDS_B2F",
	0xa1: "SEAFOAM_ISLANDS_B3F",
	0xa2: "SEAFOAM_ISLANDS_B4F",
	0xa3: "VERMILION_OLD_ROD_HOUSE",
	0xa4: "FUCHSIA_GOOD_ROD_HOUSE",
	0xa5: "POKEMON_MANSION_1F",
	0xa6: "CINNABAR_GYM",
	0xa7: "CINNABAR_LAB",
	0xa8: "CINNABAR_LAB_TRADE_ROOM",
	0xa9: "CINNABAR_LAB_METRONOME_ROOM",
	0xaa: "CINNABAR_LAB_FOSSIL_ROOM",
	0xab: "CINNABAR_POKECENTER",
	0xac: "CINNABAR_MART",
	0xad: "CINNABAR_MART_COPY",
	0xae: "INDIGO_PLATEAU_LOBBY",
	0xaf: "COPYCATS_HOUSE_1F",
	0xb0: "COPYCATS_HOUSE_2F",
	0xb1: "FIGHTING_DOJO",
	0xb2: "SAFFRON_GYM",
	0xb3: "SAFFRON_PIDGEY_HOUSE",
	0xb4: "SAFFRON_MART",
	0xb5: "SILPH_CO_1F",
	0xb6: "SAFFRON_POKECENTER",
	0xb7: "MR_PSYCHICS_HOUSE",
	0xb8: "ROUTE_15_GATE_1F",
	0xb9: "ROUTE_15_GATE_2F",
	0xba: "ROUTE_16_GATE_1F",
	0xbb: "ROUTE_16_GATE_2F",
	0xbc: "ROUTE_16_FLY_HOUSE",
	0xbd: "ROUTE_12_SUPER_ROD_HOUSE",
	0xbe: "ROUTE_18_GATE_1F",
	0xbf: "ROUTE_18_GATE_2F",
	0xc0: "SEAFOAM_ISLANDS_1F",
	0xc1: "ROUTE_22_GATE",
	0xc2: "VICTORY_ROAD_2F",
	0xc3: "ROUTE_12_GATE_2F",
	0xc4: "VERMILION_TRADE_HOUSE",
	0xc5: "DIGLETTS_CAVE",
	0xc6: "VICTORY_ROAD_3F",
	0xc7: "ROCKET_HIDEOUT_B1F",
	0xc8: "ROCKET_HIDEOUT_B2F",
	0xc9: "ROCKET_HIDEOUT_B3F",
	0xca: "ROCKET_HIDEOUT_B4F",
	0xcb: "ROCKET_HIDEOUT_ELEVATOR",
	0xcc: "UNUSED_MAP_CC",
	0xcd: "UNUSED_MAP_CD",
	0xce: "UNUSED_MAP_CE",
	0xcf: "SILPH_CO_2F",
	0xd0: "SILPH_CO_3F",
	0xd1: "SILPH_CO_4F",
	0xd2: "SILPH_CO_5F",
	0xd3: "SILPH_CO_6F",
	0xd4: "SILPH_CO_7F",
	0xd5: "SILPH_CO_8F",
	0xd6: "POKEMON_MANSION_2F",
	0xd7: "POKEMON_MANSION_3F",
	0xd8: "POKEMON_MANSION_B1F",
	0xd9: "SAFARI_ZONE_EAST",
	0xda: "SAFARI_ZONE_NORTH",
	0xdb: "SAFARI_ZONE_WEST",
	0xdc: "SAFARI_ZONE_CENTER",
	0xdd: "SAFARI_ZONE_CENTER_REST_HOUSE",
	0xde: "SAFARI_ZONE_SECRET_HOUSE",
	0xdf: "SAFARI_ZONE_WEST_REST_HOUSE",
	0xe0: "SAFARI_ZONE_EAST_REST_HOUSE",
	0xe1: "SAFARI_ZONE_NORTH_REST_HOUSE",
	0xe2: "CERULEAN_CAVE_2F",
	0xe3: "CERULEAN_CAVE_B1F",
	0xe4: "CERULEAN_CAVE_1F",
	0xe5: "NAME_RATERS_HOUSE",
	0xe6: "CERULEAN_BADGE_HOUSE",
	0xe7: "UNUSED_MAP_E7",
	0xe8: "ROCK_TUNNEL_B1F",
	0xe9: "SILPH_CO_9F",
	0xea: "SILPH_CO_10F",
	0xeb: "SILPH_CO_11F",
	0xec: "SILPH_CO_ELEVATOR",
	0xed: "UNUSED_MAP_ED",
	0xee: "UNUSED_MAP_EE",
	0xef: "TRADE_CENTER",
	0xf0: "COLOSSEUM",
	0xf1: "UNUSED_MAP_F1",
	0xf2: "UNUSED_MAP_F2",
	0xf3: "UNUSED_MAP_F3",
	0xf4: "UNUSED_MAP_F4",
	0xf5: "LORELEIS_ROOM",
	0xf6: "BRUNOS_ROOM",
	0xf7: "AGATHAS_ROOM",
	0xf8: "SUMMER_BEACH_HOUSE",
}

var yellowHeaderRefs = [0xF9]gen1rom.HeaderRef{
	0x00: {Bank: 0x06, Addr: 0x42a1}, // PALLET_TOWN
	0x01: {Bank: 0x06, Addr: 0x4357}, // VIRIDIAN_CITY
	0x02: {Bank: 0x06, Addr: 0x455a}, // PEWTER_CITY
	0x03: {Bank: 0x06, Addr: 0x4754}, // CERULEAN_CITY
	0x04: {Bank: 0x11, Addr: 0x4000}, // LAVENDER_TOWN
	0x05: {Bank: 0x06, Addr: 0x499e}, // VERMILION_CITY
	0x06: {Bank: 0x06, Addr: 0x4000}, // CELADON_CITY
	0x07: {Bank: 0x06, Addr: 0x4bb3}, // FUCHSIA_CITY
	0x08: {Bank: 0x07, Addr: 0x4000}, // CINNABAR_ISLAND
	0x09: {Bank: 0x14, Addr: 0x4924}, // INDIGO_PLATEAU
	0x0a: {Bank: 0x14, Addr: 0x49aa}, // SAFFRON_CITY
	0x0c: {Bank: 0x07, Addr: 0x40c3}, // ROUTE_1
	0x0d: {Bank: 0x15, Addr: 0x4000}, // ROUTE_2
	0x0e: {Bank: 0x15, Addr: 0x41ee}, // ROUTE_3
	0x0f: {Bank: 0x15, Addr: 0x4398}, // ROUTE_4
	0x10: {Bank: 0x15, Addr: 0x4589}, // ROUTE_5
	0x11: {Bank: 0x16, Addr: 0x4000}, // ROUTE_6
	0x12: {Bank: 0x12, Addr: 0x4000}, // ROUTE_7
	0x13: {Bank: 0x16, Addr: 0x412d}, // ROUTE_8
	0x14: {Bank: 0x15, Addr: 0x468e}, // ROUTE_9
	0x15: {Bank: 0x16, Addr: 0x42d4}, // ROUTE_10
	0x16: {Bank: 0x16, Addr: 0x44be}, // ROUTE_11
	0x17: {Bank: 0x16, Addr: 0x466d}, // ROUTE_12
	0x18: {Bank: 0x15, Addr: 0x4814}, // ROUTE_13
	0x19: {Bank: 0x15, Addr: 0x49a1}, // ROUTE_14
	0x1a: {Bank: 0x16, Addr: 0x492c}, // ROUTE_15
	0x1b: {Bank: 0x16, Addr: 0x4ada}, // ROUTE_16
	0x1c: {Bank: 0x15, Addr: 0x4b28}, // ROUTE_17
	0x1d: {Bank: 0x16, Addr: 0x4c38}, // ROUTE_18
	0x1e: {Bank: 0x15, Addr: 0x4e80}, // ROUTE_19
	0x1f: {Bank: 0x14, Addr: 0x40f1}, // ROUTE_20
	0x20: {Bank: 0x15, Addr: 0x500f}, // ROUTE_21
	0x21: {Bank: 0x14, Addr: 0x4000}, // ROUTE_22
	0x22: {Bank: 0x14, Addr: 0x433f}, // ROUTE_23
	0x23: {Bank: 0x14, Addr: 0x4682}, // ROUTE_24
	0x24: {Bank: 0x14, Addr: 0x47a1}, // ROUTE_25
	0x25: {Bank: 0x12, Addr: 0x40f6}, // REDS_HOUSE_1F
	0x26: {Bank: 0x17, Addr: 0x40a4}, // REDS_HOUSE_2F
	0x27: {Bank: 0x06, Addr: 0x5c2f}, // BLUES_HOUSE
	0x28: {Bank: 0x07, Addr: 0x4386}, // OAKS_LAB
	0x29: {Bank: 0x11, Addr: 0x4251}, // VIRIDIAN_POKECENTER
	0x2a: {Bank: 0x07, Addr: 0x4c6e}, // VIRIDIAN_MART
	0x2b: {Bank: 0x07, Addr: 0x4d6d}, // VIRIDIAN_SCHOOL_HOUSE
	0x2c: {Bank: 0x07, Addr: 0x4dc6}, // VIRIDIAN_NICKNAME_HOUSE
	0x2d: {Bank: 0x1d, Addr: 0x40d4}, // VIRIDIAN_GYM
	0x2e: {Bank: 0x07, Addr: 0x57ae}, // DIGLETTS_CAVE_ROUTE_2
	0x2f: {Bank: 0x17, Addr: 0x5485}, // VIRIDIAN_FOREST_NORTH_GATE
	0x30: {Bank: 0x07, Addr: 0x57eb}, // ROUTE_2_TRADE_HOUSE
	0x31: {Bank: 0x17, Addr: 0x54d2}, // ROUTE_2_GATE
	0x32: {Bank: 0x17, Addr: 0x555a}, // VIRIDIAN_FOREST_SOUTH_GATE
	0x33: {Bank: 0x18, Addr: 0x50ed}, // VIRIDIAN_FOREST
	0x34: {Bank: 0x17, Addr: 0x40e3}, // MUSEUM_1F
	0x35: {Bank: 0x17, Addr: 0x41b4}, // MUSEUM_2F
	0x36: {Bank: 0x17, Addr: 0x4257}, // PEWTER_GYM
	0x37: {Bank: 0x07, Addr: 0x4e30}, // PEWTER_NIDORAN_HOUSE
	0x38: {Bank: 0x1d, Addr: 0x44de}, // PEWTER_MART
	0x39: {Bank: 0x07, Addr: 0x4e86}, // PEWTER_SPEECH_HOUSE
	0x3a: {Bank: 0x17, Addr: 0x446e}, // PEWTER_POKECENTER
	0x3b: {Bank: 0x12, Addr: 0x5953}, // MT_MOON_1F
	0x3c: {Bank: 0x14, Addr: 0x5a78}, // MT_MOON_B1F
	0x3d: {Bank: 0x12, Addr: 0x5c7e}, // MT_MOON_B2F
	0x3e: {Bank: 0x07, Addr: 0x4ec3}, // CERULEAN_TRASHED_HOUSE
	0x3f: {Bank: 0x07, Addr: 0x4f34}, // CERULEAN_MELANIES_HOUSE
	0x40: {Bank: 0x17, Addr: 0x44f5}, // CERULEAN_POKECENTER
	0x41: {Bank: 0x17, Addr: 0x4577}, // CERULEAN_GYM
	0x42: {Bank: 0x07, Addr: 0x5038}, // BIKE_SHOP
	0x43: {Bank: 0x17, Addr: 0x4757}, // CERULEAN_MART
	0x44: {Bank: 0x12, Addr: 0x52a9}, // MT_MOON_POKECENTER
	0x45: {Bank: 0x07, Addr: 0x4ec3}, // CERULEAN_TRASHED_HOUSE_COPY
	0x46: {Bank: 0x07, Addr: 0x5831}, // ROUTE_5_GATE
	0x47: {Bank: 0x17, Addr: 0x55a8}, // UNDERGROUND_PATH_ROUTE_5
	0x48: {Bank: 0x15, Addr: 0x6233}, // DAYCARE
	0x49: {Bank: 0x07, Addr: 0x593b}, // ROUTE_6_GATE
	0x4a: {Bank: 0x17, Addr: 0x55ee}, // UNDERGROUND_PATH_ROUTE_6
	0x4b: {Bank: 0x17, Addr: 0x55ee}, // UNDERGROUND_PATH_ROUTE_6_COPY
	0x4c: {Bank: 0x07, Addr: 0x59fe}, // ROUTE_7_GATE
	0x4d: {Bank: 0x17, Addr: 0x562b}, // UNDERGROUND_PATH_ROUTE_7
	0x4e: {Bank: 0x17, Addr: 0x562b}, // UNDERGROUND_PATH_ROUTE_7_COPY
	0x4f: {Bank: 0x07, Addr: 0x5ac5}, // ROUTE_8_GATE
	0x50: {Bank: 0x07, Addr: 0x5b87}, // UNDERGROUND_PATH_ROUTE_8
	0x51: {Bank: 0x12, Addr: 0x5330}, // ROCK_TUNNEL_POKECENTER
	0x52: {Bank: 0x11, Addr: 0x4571}, // ROCK_TUNNEL_1F
	0x53: {Bank: 0x07, Addr: 0x5bc4}, // POWER_PLANT
	0x54: {Bank: 0x12, Addr: 0x5396}, // ROUTE_11_GATE_1F
	0x55: {Bank: 0x07, Addr: 0x5eb8}, // DIGLETTS_CAVE_ROUTE_11
	0x56: {Bank: 0x12, Addr: 0x53de}, // ROUTE_11_GATE_2F
	0x57: {Bank: 0x12, Addr: 0x548f}, // ROUTE_12_GATE_1F
	0x58: {Bank: 0x07, Addr: 0x606e}, // BILLS_HOUSE
	0x59: {Bank: 0x17, Addr: 0x4865}, // VERMILION_POKECENTER
	0x5a: {Bank: 0x16, Addr: 0x5a00}, // POKEMON_FAN_CLUB
	0x5b: {Bank: 0x17, Addr: 0x48cb}, // VERMILION_MART
	0x5c: {Bank: 0x17, Addr: 0x4910}, // VERMILION_GYM
	0x5d: {Bank: 0x07, Addr: 0x53f8}, // VERMILION_PIDGEY_HOUSE
	0x5e: {Bank: 0x07, Addr: 0x544e}, // VERMILION_DOCK
	0x5f: {Bank: 0x18, Addr: 0x52a4}, // SS_ANNE_1F
	0x60: {Bank: 0x18, Addr: 0x53de}, // SS_ANNE_2F
	0x61: {Bank: 0x11, Addr: 0x49bf}, // SS_ANNE_3F
	0x62: {Bank: 0x18, Addr: 0x5650}, // SS_ANNE_B1F
	0x63: {Bank: 0x18, Addr: 0x56d0}, // SS_ANNE_BOW
	0x64: {Bank: 0x18, Addr: 0x57d5}, // SS_ANNE_KITCHEN
	0x65: {Bank: 0x18, Addr: 0x58b7}, // SS_ANNE_CAPTAINS_ROOM
	0x66: {Bank: 0x18, Addr: 0x5993}, // SS_ANNE_1F_ROOMS
	0x67: {Bank: 0x18, Addr: 0x5b68}, // SS_ANNE_2F_ROOMS
	0x68: {Bank: 0x18, Addr: 0x5d60}, // SS_ANNE_B1F_ROOMS
	0x6c: {Bank: 0x17, Addr: 0x5909}, // VICTORY_ROAD_1F
	0x71: {Bank: 0x16, Addr: 0x623d}, // LANCES_ROOM
	0x76: {Bank: 0x16, Addr: 0x642d}, // HALL_OF_FAME
	0x77: {Bank: 0x18, Addr: 0x5f31}, // UNDERGROUND_PATH_NORTH_SOUTH
	0x78: {Bank: 0x1d, Addr: 0x57a0}, // CHAMPIONS_ROOM
	0x79: {Bank: 0x18, Addr: 0x5f55}, // UNDERGROUND_PATH_WEST_EAST
	0x7a: {Bank: 0x11, Addr: 0x42b7}, // CELADON_MART_1F
	0x7b: {Bank: 0x15, Addr: 0x60d9}, // CELADON_MART_2F
	0x7c: {Bank: 0x12, Addr: 0x4157}, // CELADON_MART_3F
	0x7d: {Bank: 0x12, Addr: 0x4251}, // CELADON_MART_4F
	0x7e: {Bank: 0x12, Addr: 0x42d0}, // CELADON_MART_ROOF
	0x7f: {Bank: 0x12, Addr: 0x44ff}, // CELADON_MART_ELEVATOR
	0x80: {Bank: 0x12, Addr: 0x4593}, // CELADON_MANSION_1F
	0x81: {Bank: 0x12, Addr: 0x465a}, // CELADON_MANSION_2F
	0x82: {Bank: 0x12, Addr: 0x46b0}, // CELADON_MANSION_3F
	0x83: {Bank: 0x12, Addr: 0x4861}, // CELADON_MANSION_ROOF
	0x84: {Bank: 0x07, Addr: 0x5636}, // CELADON_MANSION_ROOF_HOUSE
	0x85: {Bank: 0x12, Addr: 0x48af}, // CELADON_POKECENTER
	0x86: {Bank: 0x12, Addr: 0x4915}, // CELADON_GYM
	0x87: {Bank: 0x12, Addr: 0x4bc8}, // GAME_CORNER
	0x88: {Bank: 0x12, Addr: 0x507f}, // CELADON_MART_5F
	0x89: {Bank: 0x12, Addr: 0x5107}, // GAME_CORNER_PRIZE_ROOM
	0x8a: {Bank: 0x12, Addr: 0x5168}, // CELADON_DINER
	0x8b: {Bank: 0x12, Addr: 0x51e8}, // CELADON_CHIEF_HOUSE
	0x8c: {Bank: 0x12, Addr: 0x5243}, // CELADON_HOTEL
	0x8d: {Bank: 0x17, Addr: 0x479c}, // LAVENDER_POKECENTER
	0x8e: {Bank: 0x18, Addr: 0x4420}, // POKEMON_TOWER_1F
	0x8f: {Bank: 0x18, Addr: 0x44e7}, // POKEMON_TOWER_2F
	0x90: {Bank: 0x18, Addr: 0x46af}, // POKEMON_TOWER_3F
	0x91: {Bank: 0x18, Addr: 0x47d9}, // POKEMON_TOWER_4F
	0x92: {Bank: 0x18, Addr: 0x4915}, // POKEMON_TOWER_5F
	0x93: {Bank: 0x18, Addr: 0x4ad2}, // POKEMON_TOWER_6F
	0x94: {Bank: 0x18, Addr: 0x4ce8}, // POKEMON_TOWER_7F
	0x95: {Bank: 0x07, Addr: 0x51a4}, // MR_FUJIS_HOUSE
	0x96: {Bank: 0x17, Addr: 0x4802}, // LAVENDER_MART
	0x97: {Bank: 0x07, Addr: 0x52aa}, // LAVENDER_CUBONE_HOUSE
	0x98: {Bank: 0x07, Addr: 0x5685}, // FUCHSIA_MART
	0x99: {Bank: 0x1d, Addr: 0x4851}, // FUCHSIA_BILLS_GRANDPAS_HOUSE
	0x9a: {Bank: 0x1d, Addr: 0x489c}, // FUCHSIA_POKECENTER
	0x9b: {Bank: 0x1d, Addr: 0x4902}, // WARDENS_HOUSE
	0x9c: {Bank: 0x1d, Addr: 0x4a1a}, // SAFARI_ZONE_GATE
	0x9d: {Bank: 0x1d, Addr: 0x4bd9}, // FUCHSIA_GYM
	0x9e: {Bank: 0x1d, Addr: 0x4e7f}, // FUCHSIA_MEETING_ROOM
	0x9f: {Bank: 0x11, Addr: 0x6578}, // SEAFOAM_ISLANDS_B1F
	0xa0: {Bank: 0x11, Addr: 0x66b4}, // SEAFOAM_ISLANDS_B2F
	0xa1: {Bank: 0x11, Addr: 0x67f0}, // SEAFOAM_ISLANDS_B3F
	0xa2: {Bank: 0x11, Addr: 0x69fc}, // SEAFOAM_ISLANDS_B4F
	0xa3: {Bank: 0x15, Addr: 0x6054}, // VERMILION_OLD_ROD_HOUSE
	0xa4: {Bank: 0x15, Addr: 0x6160}, // FUCHSIA_GOOD_ROD_HOUSE
	0xa5: {Bank: 0x11, Addr: 0x4344}, // POKEMON_MANSION_1F
	0xa6: {Bank: 0x1d, Addr: 0x4ee6}, // CINNABAR_GYM
	0xa7: {Bank: 0x1d, Addr: 0x53fb}, // CINNABAR_LAB
	0xa8: {Bank: 0x1d, Addr: 0x5490}, // CINNABAR_LAB_TRADE_ROOM
	0xa9: {Bank: 0x1d, Addr: 0x54f6}, // CINNABAR_LAB_METRONOME_ROOM
	0xaa: {Bank: 0x1d, Addr: 0x55a0}, // CINNABAR_LAB_FOSSIL_ROOM
	0xab: {Bank: 0x1d, Addr: 0x569b}, // CINNABAR_POKECENTER
	0xac: {Bank: 0x1d, Addr: 0x5701}, // CINNABAR_MART
	0xad: {Bank: 0x1d, Addr: 0x5701}, // CINNABAR_MART_COPY
	0xae: {Bank: 0x06, Addr: 0x5d45}, // INDIGO_PLATEAU_LOBBY
	0xaf: {Bank: 0x1d, Addr: 0x5746}, // COPYCATS_HOUSE_1F
	0xb0: {Bank: 0x17, Addr: 0x4b5b}, // COPYCATS_HOUSE_2F
	0xb1: {Bank: 0x17, Addr: 0x4c47}, // FIGHTING_DOJO
	0xb2: {Bank: 0x17, Addr: 0x4ef7}, // SAFFRON_GYM
	0xb3: {Bank: 0x07, Addr: 0x56db}, // SAFFRON_PIDGEY_HOUSE
	0xb4: {Bank: 0x17, Addr: 0x52f3}, // SAFFRON_MART
	0xb5: {Bank: 0x17, Addr: 0x5338}, // SILPH_CO_1F
	0xb6: {Bank: 0x17, Addr: 0x541f}, // SAFFRON_POKECENTER
	0xb7: {Bank: 0x07, Addr: 0x573a}, // MR_PSYCHICS_HOUSE
	0xb8: {Bank: 0x12, Addr: 0x558d}, // ROUTE_15_GATE_1F
	0xb9: {Bank: 0x12, Addr: 0x55d5}, // ROUTE_15_GATE_2F
	0xba: {Bank: 0x12, Addr: 0x5649}, // ROUTE_16_GATE_1F
	0xbb: {Bank: 0x12, Addr: 0x5796}, // ROUTE_16_GATE_2F
	0xbc: {Bank: 0x07, Addr: 0x5ef6}, // ROUTE_16_FLY_HOUSE
	0xbd: {Bank: 0x15, Addr: 0x64a5}, // ROUTE_12_SUPER_ROD_HOUSE
	0xbe: {Bank: 0x12, Addr: 0x5801}, // ROUTE_18_GATE_1F
	0xbf: {Bank: 0x12, Addr: 0x5900}, // ROUTE_18_GATE_2F
	0xc0: {Bank: 0x11, Addr: 0x487e}, // SEAFOAM_ISLANDS_1F
	0xc1: {Bank: 0x07, Addr: 0x5f81}, // ROUTE_22_GATE
	0xc2: {Bank: 0x14, Addr: 0x57cc}, // VICTORY_ROAD_2F
	0xc3: {Bank: 0x12, Addr: 0x54eb}, // ROUTE_12_GATE_2F
	0xc4: {Bank: 0x06, Addr: 0x5d05}, // VERMILION_TRADE_HOUSE
	0xc5: {Bank: 0x18, Addr: 0x5f79}, // DIGLETTS_CAVE
	0xc6: {Bank: 0x11, Addr: 0x4a0d}, // VICTORY_ROAD_3F
	0xc7: {Bank: 0x11, Addr: 0x4c5e}, // ROCKET_HIDEOUT_B1F
	0xc8: {Bank: 0x11, Addr: 0x4ebb}, // ROCKET_HIDEOUT_B2F
	0xc9: {Bank: 0x11, Addr: 0x52b9}, // ROCKET_HIDEOUT_B3F
	0xca: {Bank: 0x11, Addr: 0x54f1}, // ROCKET_HIDEOUT_B4F
	0xcb: {Bank: 0x11, Addr: 0x5958}, // ROCKET_HIDEOUT_ELEVATOR
	0xcf: {Bank: 0x16, Addr: 0x5c80}, // SILPH_CO_2F
	0xd0: {Bank: 0x16, Addr: 0x5eea}, // SILPH_CO_3F
	0xd1: {Bank: 0x06, Addr: 0x5e09}, // SILPH_CO_4F
	0xd2: {Bank: 0x06, Addr: 0x6035}, // SILPH_CO_5F
	0xd3: {Bank: 0x06, Addr: 0x62a7}, // SILPH_CO_6F
	0xd4: {Bank: 0x14, Addr: 0x5b97}, // SILPH_CO_7F
	0xd5: {Bank: 0x15, Addr: 0x652a}, // SILPH_CO_8F
	0xd6: {Bank: 0x14, Addr: 0x5ff5}, // POKEMON_MANSION_2F
	0xd7: {Bank: 0x14, Addr: 0x620b}, // POKEMON_MANSION_3F
	0xd8: {Bank: 0x14, Addr: 0x63d6}, // POKEMON_MANSION_B1F
	0xd9: {Bank: 0x11, Addr: 0x5ab3}, // SAFARI_ZONE_EAST
	0xda: {Bank: 0x11, Addr: 0x5bf3}, // SAFARI_ZONE_NORTH
	0xdb: {Bank: 0x12, Addr: 0x635a}, // SAFARI_ZONE_WEST
	0xdc: {Bank: 0x11, Addr: 0x5dfa}, // SAFARI_ZONE_CENTER
	0xdd: {Bank: 0x11, Addr: 0x5f35}, // SAFARI_ZONE_CENTER_REST_HOUSE
	0xde: {Bank: 0x12, Addr: 0x64bc}, // SAFARI_ZONE_SECRET_HOUSE
	0xdf: {Bank: 0x11, Addr: 0x5f72}, // SAFARI_ZONE_WEST_REST_HOUSE
	0xe0: {Bank: 0x11, Addr: 0x5fbd}, // SAFARI_ZONE_EAST_REST_HOUSE
	0xe1: {Bank: 0x11, Addr: 0x6008}, // SAFARI_ZONE_NORTH_REST_HOUSE
	0xe2: {Bank: 0x11, Addr: 0x6053}, // CERULEAN_CAVE_2F
	0xe3: {Bank: 0x11, Addr: 0x6141}, // CERULEAN_CAVE_B1F
	0xe4: {Bank: 0x1d, Addr: 0x453d}, // CERULEAN_CAVE_1F
	0xe5: {Bank: 0x07, Addr: 0x530e}, // NAME_RATERS_HOUSE
	0xe6: {Bank: 0x1d, Addr: 0x4643}, // CERULEAN_BADGE_HOUSE
	0xe8: {Bank: 0x11, Addr: 0x624e}, // ROCK_TUNNEL_B1F
	0xe9: {Bank: 0x17, Addr: 0x56ba}, // SILPH_CO_9F
	0xea: {Bank: 0x16, Addr: 0x60c8}, // SILPH_CO_10F
	0xeb: {Bank: 0x18, Addr: 0x6105}, // SILPH_CO_11F
	0xec: {Bank: 0x11, Addr: 0x5a08}, // SILPH_CO_ELEVATOR
	0xef: {Bank: 0x13, Addr: 0x7e79}, // TRADE_CENTER
	0xf0: {Bank: 0x13, Addr: 0x7ee6}, // COLOSSEUM
	0xf5: {Bank: 0x1d, Addr: 0x59ef}, // LORELEIS_ROOM
	0xf6: {Bank: 0x1d, Addr: 0x5b4a}, // BRUNOS_ROOM
	0xf7: {Bank: 0x1d, Addr: 0x5ca1}, // AGATHAS_ROOM
	0xf8: {Bank: 0x3c, Addr: 0x620e}, // SUMMER_BEACH_HOUSE
}

const yellowPlayableMapCount = 227
