package rom

import "github.com/maestroi/pokepilot/gen1rom"

// redTables is the Red/Blue table layout (pokered.sym). Blue's linker output
// places every table at the same address as Red's, so it needs no binding of
// its own; it is also the layout for synthetic test images.
//
// Another Gen-I revision (Yellow) registers its own layout through
// gen1rom.RegisterTableLayout. The parsers in this package then decode that
// cartridge with the same byte formats instead of forking them.
var redTables = gen1rom.TableLayout{
	MapCount: 0xF8, // NUM_MAPS: ids 0x00..0xF7
	ValidMap: redValidMapID,

	MapHeaderPointers:       gen1rom.Symbol{Bank: 0x00, Addr: 0x01AE},
	MapHeaderBanks:          gen1rom.Symbol{Bank: 0x03, Addr: 0x423D},
	Tilesets:                gen1rom.Symbol{Bank: 0x03, Addr: 0x47BE},
	TilePairCollisionsLand:  gen1rom.Symbol{Bank: 0x00, Addr: 0x0C7E},
	TilePairCollisionsWater: gen1rom.Symbol{Bank: 0x00, Addr: 0x0CA0},
	LedgeTiles:              gen1rom.Symbol{Bank: 0x06, Addr: 0x66CF},

	WildDataPointers:      gen1rom.Symbol{Bank: 0x03, Addr: 0x4EEB},
	EvosMovesPointerTable: gen1rom.Symbol{Bank: 0x0E, Addr: 0x705C},
	Moves:                 gen1rom.Symbol{Bank: 0x0E, Addr: 0x4000},
	TechnicalMachines:     gen1rom.Symbol{Bank: 0x04, Addr: 0x7773},
	PokedexOrder:          gen1rom.Symbol{Bank: 0x10, Addr: 0x5024},
	BaseStats:             gen1rom.Symbol{Bank: 0x0E, Addr: 0x43DE},
	TradeMons:             gen1rom.Symbol{Bank: 0x1C, Addr: 0x5B7B},
	TypeEffects:           gen1rom.Symbol{Bank: 0x0F, Addr: 0x6474},

	ItemUseOldRod:  gen1rom.Symbol{Bank: 0x03, Addr: 0x624C},
	GoodRodMons:    gen1rom.Symbol{Bank: 0x03, Addr: 0x627F},
	SuperRod:       gen1rom.Symbol{Bank: 0x03, Addr: 0x6919},
	SuperRodFormat: gen1rom.SuperRodGrouped,
}

// Tables returns the table layout bound to romData, or Red's.
func Tables(romData []byte) gen1rom.TableLayout {
	if layout, ok := gen1rom.TableLayoutFor(romData); ok {
		return layout
	}
	return redTables
}

// EventFlagRef resolves an event-flag reference embedded in romData (a
// trainer header's `dw wEventFlags + n/8` plus its bit) to the canonical RAM
// byte and mask shared decoders read. ok is false when the cartridge's flag
// has no canonical counterpart.
func EventFlagRef(romData []byte, nativeByte uint16, bit uint8) (addr uint16, mask uint8, ok bool) {
	layout := Tables(romData)
	if layout.EventFlag != nil {
		return layout.EventFlag(nativeByte, bit)
	}
	return nativeByte + uint16(bit)/8, 1 << (bit & 7), true
}
