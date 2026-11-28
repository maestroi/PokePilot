package rom

import "fmt"

// Tilesets (pret data/tilesets.asm) is indexed by TILESET_* from 0; each
// entry is `dba GFX, Meta, Coll` then three words, so Coll's bank+address
// sits at byte 6 of a 15-byte entry.
const (
	tilesetEntryLen   = 15
	tilesetCollOffset = 6
	numTilesets       = 0x1c
	// Block ids are bytes, so no map can index past 256 blocks * 4 tiles.
	maxCollisionLen = 256 * 4
)

// locateTilesets finds the Tilesets table from the verified Johto collision
// table (tileset 1): the unique entry whose Coll pointer reaches those bytes.
// Tileset 2 must then match its verified table too, or the image is refused.
func locateTilesets(rom []byte) (int, error) {
	johto, _ := firstBadgeCollisionTable(0x01)
	collOff, err := uniqueIndex(rom, johto, "Johto collision data")
	if err != nil {
		return 0, err
	}
	ptr := bankedPointer(collOff)
	found := -1
	for i := 0; i+len(ptr) <= len(rom); i++ {
		if !bytesEqual(rom[i:i+len(ptr)], ptr) {
			continue
		}
		table := i - tilesetCollOffset - tilesetEntryLen
		if table < 0 || table+numTilesets*tilesetEntryLen > len(rom) {
			continue
		}
		modern, _ := firstBadgeCollisionTable(0x02)
		got, err := tilesetCollisionAt(rom, table, 0x02)
		if err != nil || len(got) < len(modern) || !bytesEqual(got[:len(modern)], modern) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous Tilesets table at %#x and %#x", found, table)
		}
		found = table
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: Tilesets table not found")
	}
	return found, nil
}

// tilesetCollisionAt returns the collision bytes for one tileset, up to the
// most a map can index and never across the end of its ROM bank.
func tilesetCollisionAt(rom []byte, tilesets int, tileset uint8) ([]uint8, error) {
	if tileset == 0 || tileset > numTilesets {
		return nil, fmt.Errorf("gs/rom: tileset %#02x out of range", tileset)
	}
	entry := tilesets + int(tileset)*tilesetEntryLen + tilesetCollOffset
	if err := mustInROM(rom, entry, 3, "tileset collision pointer"); err != nil {
		return nil, err
	}
	off, err := bankedOffset(rom[entry], uint16(rom[entry+1])|uint16(rom[entry+2])<<8)
	if err != nil {
		return nil, err
	}
	end := min(off+maxCollisionLen, (off/0x4000+1)*0x4000, len(rom))
	if off >= end {
		return nil, fmt.Errorf("gs/rom: tileset %#02x collision at %#x is outside the ROM", tileset, off)
	}
	return rom[off:end], nil
}

func bankedPointer(off int) []byte {
	bank, addr := off/0x4000, 0x4000+off%0x4000
	if bank == 0 {
		addr = off
	}
	return []byte{byte(bank), byte(addr), byte(addr >> 8)}
}

func uniqueIndex(rom, want []byte, what string) (int, error) {
	found := -1
	for i := 0; i+len(want) <= len(rom); i++ {
		if !bytesEqual(rom[i:i+len(want)], want) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("gs/rom: ambiguous %s at %#x and %#x", what, found, i)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("gs/rom: %s not found", what)
	}
	return found, nil
}
