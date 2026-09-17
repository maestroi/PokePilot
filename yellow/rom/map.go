// Package rom parses Pokémon Yellow's ROM map tables.
//
// Yellow shares Gen I's map-header and object format with Red; only the table
// addresses differ. This package parameterizes red/rom's parser with Yellow's
// addresses rather than duplicating the format, so a fix to the shared parser
// serves both games. See docs/POKEYELLOW.md.
package rom

import (
	"github.com/maestroi/pokepilot/red/rom"
	yellowsym "github.com/maestroi/pokepilot/yellow/sym"
)

// Tables are Yellow's ROM map-table addresses, read from pokeyellow.sym.
func Tables() rom.Tables {
	return rom.NewTables(
		yellowsym.MapHeaderPointersBank, yellowsym.MapHeaderPointersAddr,
		yellowsym.MapHeaderBanksBank, yellowsym.MapHeaderBanksAddr,
		yellowsym.TilesetsBank, yellowsym.TilesetsAddr,
		validMapID, maxMapID,
		// Yellow's tileset collision lists were assembled into bank 1
		// (Overworld_Coll is 01:4AC2; Red's equivalent is 00:1735). The game
		// dereferences the pointer with no bank switch, so the bank is a
		// build-layout fact the pointer cannot reveal.
		1,
	)
}

// maxMapID is one above the highest map id Yellow defines: SUMMER_BEACH_HOUSE
// ($F8) is appended past Red's last map (AGATHAS_ROOM, $F7).
const maxMapID uint8 = 0xf9

// ParseMap reads one Yellow map header and its object data.
func ParseMap(romData []byte, mapID uint8) (rom.MapHeader, error) {
	return Tables().ParseMap(romData, mapID)
}

// validMapID reports whether a slot is a playable Yellow map. The id space is
// Red's almost unchanged: the only real difference at the tail is that
// SUMMER_BEACH_HOUSE is appended at $F8. AGATHAS_ROOM ($F7) is a real map in
// BOTH games, so unlike Red's predicate this one accepts $F7 and extends past
// it. Unused slots parse cleanly but invent geometry, so they must be rejected.
func validMapID(id uint8) bool {
	if id >= maxMapID {
		return false
	}
	switch id {
	case 0x0b, 0x69, 0x6a, 0x6b, 0x6d, 0x6e, 0x6f, 0x70,
		0x72, 0x73, 0x74, 0x75, 0xcc, 0xcd, 0xce, 0xe7,
		0xed, 0xee, 0xf1, 0xf2, 0xf3, 0xf4:
		return false
	}
	return true
}
