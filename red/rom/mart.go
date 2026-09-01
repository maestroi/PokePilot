package rom

import "fmt"

// martScriptID is TX_SCRIPT_MART (pokered/macros/scripts/text.asm): the first
// byte of a mart clerk's text script, followed by the item count, the item
// ids in shelf order, and a 0xff terminator.
const martScriptID = 0xfe

// MartItems returns the item ids a map's mart clerk stocks, in shelf order.
//
// The shelf is not a table keyed by map id: it is the clerk's own text
// script. Each map object carries a text id (1-based) that indexes the map's
// text pointer table, and the table whose entry for the clerk is a
// TX_SCRIPT_MART script IS the shelf. That is why this resolves the objects'
// text instead of reading a list: a Go table of which mart sells what is the
// same defect as the hardcoded POTION it replaces — a copy of the ROM's data
// that the ROM can change under.
//
// The table in use at runtime is set by the map's DEFAULT SCRIPT with
// `ld hl, <table>` (engine/home/text_script.asm: DisplayTextID indexes
// wCurMapTextPtr with (textID-1)*2). Some marts — the Viridian Mart, whose
// clerk only stocks the shelf once Oak's parcel is delivered — keep the
// shelf in a second table the default script loads after a quest, so the
// header's table alone is not enough. textPointerTables collects the header's
// table plus every table the default script (and the map-bank sub-scripts it
// calls) loads, and the first entry that is a mart script wins.
//
// A map whose objects carry no mart script reports an error. Callers must
// treat an unreadable shelf as "offer nothing", never as a reason to guess.
func MartItems(rom []byte, mapID uint8) ([]uint8, error) {
	h, err := ParseMap(rom, mapID)
	if err != nil {
		return nil, mapErr(mapID, err)
	}

	for _, table := range textPointerTables(rom, h) {
		for _, obj := range h.Objects {
			if obj.TextID == 0 || obj.TextID&0xC0 != 0 {
				continue // no text, or a trainer/item entry
			}
			if items, ok := martScriptAt(rom, table, int(obj.TextID)-1); ok {
				return items, nil
			}
		}
	}
	return nil, mapErr(mapID, fmt.Errorf("no object carries a mart script"))
}

// textPointerTables returns the file offsets of a map's text pointer tables:
// the header's primary table first, then every table the default map script
// (and the sub-scripts it calls) loads with `ld hl, <offset>`. Deduplicated,
// in discovery order.
func textPointerTables(rom []byte, h MapHeader) []int {
	var tables []int
	seen := map[int]bool{}
	add := func(off int) {
		if off >= 0 && off < len(rom) && !seen[off] {
			seen[off] = true
			tables = append(tables, off)
		}
	}

	if off, err := bankedOffset(h.Bank, h.TextsAddr); err == nil {
		add(off)
	}
	if h.ScriptAddr != 0 {
		if off, err := bankedOffset(h.Bank, h.ScriptAddr); err == nil {
			for _, t := range ldhlTables(rom, off, h.Bank) {
				add(t)
			}
		}
	}
	return tables
}

// ldhlTables scans a map script and the sub-scripts it calls for `ld hl,
// imm16` instructions, returning the file offsets of the tables they load.
//
// The default script runs in the map's bank, and both its `ld hl` and `call`
// immediates are 14-bit offsets into that bank, so every target resolves with
// the map bank. The scan reads raw bytes rather than disassembling precisely:
// a spurious match in a data byte, or a call into a routine that sits at the
// same offset in another bank, only matters if it points at a table that
// resolves an object to a TX_SCRIPT_MART script — and MartItems validates
// every candidate by reading it, so a false match is dropped, not trusted.
// The walk is bounded (16 scripts, 128 bytes each) so it cannot run away.
func ldhlTables(rom []byte, start int, bank uint8) []int {
	var tables []int
	seen := map[int]bool{}
	queue := []int{start}
	for len(queue) > 0 && len(seen) < 16 {
		at := queue[0]
		queue = queue[1:]
		if seen[at] || at < 0 || at >= len(rom) {
			continue
		}
		seen[at] = true
		limit := at + 128
		if limit > len(rom) {
			limit = len(rom)
		}
		for i := at; i+2 < limit; i++ {
			imm := uint16(rom[i+1]) | uint16(rom[i+2])<<8
			switch rom[i] {
			case 0x21: // ld hl, imm16
				if off, err := bankedOffset(bank, imm); err == nil {
					tables = append(tables, off)
				}
				i += 2
			case 0xCD: // call imm16
				if off, err := bankedOffset(bank, imm); err == nil {
					queue = append(queue, off)
				}
				i += 2
			}
		}
	}
	return tables
}

// martScriptAt reads the text pointer table entry at index i and, if it is a
// TX_SCRIPT_MART script, returns the item ids it names. The entry is a 14-bit
// offset into bank 0, where the game keeps its text.
func martScriptAt(rom []byte, table, i int) ([]uint8, bool) {
	at := table + i*2
	if at+2 > len(rom) {
		return nil, false
	}
	ptr := uint16(rom[at]) | uint16(rom[at+1])<<8
	off, err := bankedOffset(0, ptr)
	if err != nil || off+2 >= len(rom) || rom[off] != martScriptID {
		return nil, false
	}
	n := int(rom[off+1])
	if n == 0 || off+2+n+1 > len(rom) {
		return nil, false
	}
	if rom[off+2+n] != 0xff {
		return nil, false
	}
	items := make([]uint8, n)
	copy(items, rom[off+2:off+2+n])
	return items, true
}
