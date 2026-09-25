package rom

import (
	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

// Tables is Yellow's Gen-I table layout (pokeyellow.sym). Binding it to the
// Yellow cartridge lets the shared Gen-I ROM decoders (red/rom, which serves
// the Gen-I engine) read Yellow's tables in their own places, with the byte
// formats they already share. Only the Super Rod table has a Yellow-only
// format; the layout names it rather than a second parser copy.
var Tables = gen1rom.TableLayout{
	MapCount: yellowMapCount,
	ValidMap: validMapID,

	MapHeaderPointers:       gen1rom.Symbol{Bank: 0x3F, Addr: 0x41F2},
	MapHeaderBanks:          gen1rom.Symbol{Bank: 0x3F, Addr: 0x43E4},
	Tilesets:                gen1rom.Symbol{Bank: yellowTilesetsBank, Addr: yellowTilesetsAddr},
	CollisionBank:           yellowCollisionBank,
	TilePairCollisionsLand:  gen1rom.Symbol{Bank: 0x00, Addr: yellowTilePairCollisionsLand},
	TilePairCollisionsWater: gen1rom.Symbol{Bank: 0x00, Addr: yellowTilePairCollisionsWater},
	LedgeTiles:              gen1rom.Symbol{Bank: 0x06, Addr: 0x6851},

	WildDataPointers:      gen1rom.Symbol{Bank: yellowWildPointersBank, Addr: yellowWildPointersAddr},
	EvosMovesPointerTable: gen1rom.Symbol{Bank: 0x0E, Addr: 0x71E5},
	Moves:                 gen1rom.Symbol{Bank: 0x0E, Addr: 0x4000},
	TechnicalMachines:     gen1rom.Symbol{Bank: 0x04, Addr: 0x632D},
	PokedexOrder:          gen1rom.Symbol{Bank: 0x10, Addr: 0x50B1},
	BaseStats:             gen1rom.Symbol{Bank: 0x0E, Addr: 0x43DE},
	TradeMons:             gen1rom.Symbol{Bank: 0x1C, Addr: 0x5C1D},
	TypeEffects:           gen1rom.Symbol{Bank: 0x0F, Addr: 0x65FA},

	ItemUseOldRod:  gen1rom.Symbol{Bank: 0x03, Addr: 0x60F9},
	GoodRodMons:    gen1rom.Symbol{Bank: 0x03, Addr: 0x612C},
	SuperRod:       gen1rom.Symbol{Bank: 0x3D, Addr: 0x5EDA}, // SuperRodFishingSlots
	SuperRodFormat: gen1rom.SuperRodInline,

	// Yellow's trainer headers point at native wEventFlags bits; shared
	// decoders read the canonical view, so renumber through it.
	EventFlag: sym.Canonical.CanonicalFlag,
}

func init() {
	gen1rom.RegisterTableLayout(IsCartridge, Tables)
}
