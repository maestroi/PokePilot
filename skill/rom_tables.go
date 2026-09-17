package skill

import (
	"sync"

	"github.com/maestroi/pokepilot/emu"
	redstate "github.com/maestroi/pokepilot/red/state"
)

// Gen I games share one wild-data and tileset format but not the addresses of
// the tables holding them. Pokémon Red and Pokémon Yellow both read
// WildDataPointers the same way (the engine files are byte-identical), and
// the tileset header layout is the same; only the bank:addr of each table
// moves. The readers in this package historically took just romData, so this
// resolves which table set a ROM needs from the ROM itself and caches it.
//
// A ROM is immutable for the lifetime of a run, and the pointer-into-slice
// key retains the backing allocation, exactly like routeGraphCache.
type romTableSet struct {
	tilesetsBank uint8
	tilesetsAddr uint16
	wildBank     uint8
	wildAddr     uint16

	// passableSpriteSlot is the sprite slot occupied by an overworld
	// follower that the player can walk through. 0 means no such follower:
	// every decoded sprite is a solid blocker. Pokémon Yellow's Pikachu
	// occupies slot 15 (PIKACHU_SPRITE_INDEX = NUM_SPRITESTATEDATA_STRUCTS
	// - 1) and CollisionCheckOnLand lets the player bump it a bounded number
	// of times and then pass through, or pass immediately while holding B,
	// so its tile must not be planned around. Red has no follower.
	passableSpriteSlot int

	// wram is the WRAM address set this image needs. Yellow shifted a
	// contiguous WRAM region one byte lower than Red, so the skill layer
	// reads every live address through here rather than through a game's sym
	// package. See wramAddresses.
	wram wramAddresses
}

var redRomTables = romTableSet{
	tilesetsBank: trainTilesetsBank,
	tilesetsAddr: trainTilesetsAddr,
	wildBank:     trainWildBank,
	wildAddr:     trainWildAddr,
	wram:         redWram(),
}

type romTablesKey struct {
	first  *byte
	length int
}

var romTablesCache = struct {
	sync.Mutex
	entries map[romTablesKey]romTableSet
}{entries: make(map[romTablesKey]romTableSet)}

func tablesForROM(romData []byte) romTableSet {
	if len(romData) == 0 {
		return redRomTables
	}
	key := romTablesKey{first: &romData[0], length: len(romData)}
	romTablesCache.Lock()
	defer romTablesCache.Unlock()
	if cached, ok := romTablesCache.entries[key]; ok {
		return cached
	}

	set := redRomTables
	if isYellowROM(romData) {
		set = yellowRomTables()
	}
	romTablesCache.entries[key] = set
	return set
}

// ram resolves the WRAM address set for the image the emulator holds. It is
// the only way the skill layer should reach a live address: reading a game
// constant directly silently reads the wrong byte on the other image.
func ram(m *emu.Emu) wramAddresses {
	return tablesForROM(m.ROM()).wram
}

// AddressesForROM resolves the WRAM address set for a ROM image. It is the
// exported boundary for callers outside the skill layer (the agent adapters)
// that must decode or read live state without knowing which image they hold:
// the set is decided by the profile registry, exactly as the table set is.
func AddressesForROM(romData []byte) redstate.Addresses {
	return tablesForROM(romData).wram
}

// AddressesFor resolves the WRAM address set for the image an emulator holds.
func AddressesFor(m *emu.Emu) redstate.Addresses { return ram(m) }

// RedAddresses is the exported form of redWram: the address set for the
// supported Pokémon Red image. External test packages (package skill_test)
// decode synthetic Red RAM and need the same set the runtime would resolve
// for that image, without reaching into unexported state.
func RedAddresses() redstate.Addresses { return redWram() }
