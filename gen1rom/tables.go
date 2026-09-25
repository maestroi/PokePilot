package gen1rom

import (
	"sync"
	"unsafe"
)

// Symbol is a banked ROM address as a .sym file writes it (bank:addr).
type Symbol struct {
	Bank uint8
	Addr uint16
}

// Offset returns the ROM file offset of s.
func (s Symbol) Offset() (int, error) { return BankedOffset(s.Bank, s.Addr) }

// SuperRodFormat names the byte layout of a cartridge's Super Rod table.
type SuperRodFormat uint8

const (
	// SuperRodGrouped is Red/Blue's SuperRodData: rows of (map, dw group)
	// terminated by $ff, each group a count followed by (level, species)
	// pairs, all in the table's own bank.
	SuperRodGrouped SuperRodFormat = iota
	// SuperRodInline is Yellow's SuperRodFishingSlots: rows of map followed
	// by four inline (species, level) pairs, terminated by $ff.
	SuperRodInline
)

// TableLayout locates one Gen-I cartridge's static data tables.
//
// Red, Blue and Yellow decode these tables with the same byte formats; what
// differs between the revisions is where the linker placed them, how many
// map slots exist, and which slots are real maps. A TableLayout is that
// per-game fact. It is owned by the game's ROM package and bound to the
// cartridge through RegisterTableLayout, so shared Gen-I decoders read another
// revision's ROM without learning its addresses.
type TableLayout struct {
	// MapCount is NUM_MAPS: the length of every per-map pointer table.
	MapCount int
	// ValidMap reports whether a map slot holds a real, playable map. Unused
	// slots still carry pointer-table entries that parse into garbage.
	ValidMap func(mapID uint8) bool

	MapHeaderPointers Symbol
	MapHeaderBanks    Symbol
	Tilesets          Symbol
	// CollisionBank is the bank holding switchable-bank collision lists; zero
	// reads them from the tileset's graphics bank (Red/Blue).
	CollisionBank           uint8
	TilePairCollisionsLand  Symbol
	TilePairCollisionsWater Symbol
	LedgeTiles              Symbol

	WildDataPointers      Symbol
	EvosMovesPointerTable Symbol
	Moves                 Symbol
	TechnicalMachines     Symbol
	PokedexOrder          Symbol
	BaseStats             Symbol
	TradeMons             Symbol
	TypeEffects           Symbol

	// ItemUseOldRod is the Old Rod item handler; its `lb bc, level, species`
	// immediate is the Old Rod's only encounter.
	ItemUseOldRod  Symbol
	GoodRodMons    Symbol
	SuperRod       Symbol
	SuperRodFormat SuperRodFormat

	// EventFlag translates an event-flag reference embedded in ROM data
	// (trainer headers write `dw wEventFlags + n/8` and the bit) into the
	// canonical Gen-I RAM coordinates that shared decoders read. Nil means
	// the cartridge's native RAM layout is canonical.
	EventFlag func(nativeByte uint16, bit uint8) (canonByte uint16, mask uint8, ok bool)
}

type tableBinding struct {
	match  func(rom []byte) bool
	layout TableLayout
}

// romKey identifies a ROM image cheaply enough to cache per call: the backing
// array, its length, and the cartridge header, so a reused allocation holding
// a different cartridge never inherits a stale binding.
type romKey struct {
	data   uintptr
	n      int
	header [0x1c]byte
}

const tableCacheLimit = 16

var (
	tableMu       sync.RWMutex
	tableBindings []tableBinding
	tableCache    = map[romKey]int{} // index into tableBindings, -1 for none
)

// RegisterTableLayout binds layout to every ROM image match accepts. match
// may be expensive (a whole-image hash): its answer is cached per image.
func RegisterTableLayout(match func(rom []byte) bool, layout TableLayout) {
	if match == nil {
		return
	}
	tableMu.Lock()
	defer tableMu.Unlock()
	tableBindings = append(tableBindings, tableBinding{match: match, layout: layout})
	clear(tableCache)
}

// TableLayoutFor returns the layout registered for rom. Callers fall back to
// their own default layout when none matches, which keeps synthetic test
// images decoding as the historical Red layout.
func TableLayoutFor(rom []byte) (TableLayout, bool) {
	if len(rom) == 0 {
		return TableLayout{}, false
	}
	key := romKey{data: uintptr(unsafe.Pointer(unsafe.SliceData(rom))), n: len(rom)}
	if len(rom) >= 0x150 {
		copy(key.header[:], rom[0x134:0x150])
	}

	tableMu.RLock()
	idx, cached := tableCache[key]
	bindings := tableBindings
	tableMu.RUnlock()
	if !cached {
		idx = -1
		for i, b := range bindings {
			if b.match(rom) {
				idx = i
				break
			}
		}
		tableMu.Lock()
		if len(tableCache) >= tableCacheLimit {
			clear(tableCache)
		}
		tableCache[key] = idx
		tableMu.Unlock()
	}
	if idx < 0 || idx >= len(bindings) {
		return TableLayout{}, false
	}
	return bindings[idx].layout, true
}
