package rom

import (
	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/worldmodel"
)

type ObjectInteractionRole = worldmodel.InteractionRole

const (
	InteractionPokemonCenterNurse = worldmodel.InteractionPokemonCenterNurse
	InteractionMart               = worldmodel.InteractionMart
	InteractionBillsPC            = worldmodel.InteractionBillsPC
	InteractionPlayersPC          = worldmodel.InteractionPlayersPC
	InteractionPokemonCenterPC    = worldmodel.InteractionPokemonCenterPC
	InteractionPrizeVendor        = worldmodel.InteractionPrizeVendor
	InteractionCableClub          = worldmodel.InteractionCableClub
	InteractionVendingMachine     = worldmodel.InteractionVendingMachine
)

type SpecialInteractionActor struct {
	X, Y uint8
	Role ObjectInteractionRole
}

const (
	textScriptPokemonCenterNurse uint8 = 0xff
	textScriptMart               uint8 = 0xfe
	textScriptBillsPC            uint8 = 0xfd
	textScriptPlayersPC          uint8 = 0xfc
	textScriptPokemonCenterPC    uint8 = 0xf9
	textScriptPrizeVendor        uint8 = 0xf7
	textScriptCableClub          uint8 = 0xf6
	textScriptVendingMachine     uint8 = 0xf5
)

// SpecialInteractionActors decodes Yellow's shared Gen-I TX_SCRIPT_* service
// dispatches from the cartridge's own text-pointer tables. Keeping the role at
// the ROM-provider boundary lets shared controllers locate nurses, marts and
// other services without importing Yellow map coordinates.
func SpecialInteractionActors(romData []byte, mapID uint8) ([]SpecialInteractionActor, error) {
	h, err := ParseMap(romData, mapID)
	if err != nil {
		return nil, err
	}

	out := make([]SpecialInteractionActor, 0)
	seen := map[[2]uint8]bool{}
	for _, table := range gen1rom.TextPointerTables(romData, gen1rom.MapHeader(h)) {
		for _, obj := range h.Objects {
			if obj.TextID == 0 || obj.TextID&0xc0 != 0 {
				continue
			}
			role, ok := specialInteractionRoleAt(romData, h.Bank, table, int(obj.TextID)-1)
			if !ok {
				continue
			}
			key := [2]uint8{obj.X, obj.Y}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, SpecialInteractionActor{X: obj.X, Y: obj.Y, Role: role})
		}
	}
	return out, nil
}

func specialInteractionRoleAt(romData []byte, mapBank uint8, table, index int) (ObjectInteractionRole, bool) {
	at := table + index*2
	if index < 0 || at < 0 || at+2 > len(romData) {
		return "", false
	}
	ptr := uint16(romData[at]) | uint16(romData[at+1])<<8
	bank := mapBank
	if ptr < 0x4000 {
		bank = 0
	}
	off, err := gen1rom.BankedOffset(bank, ptr)
	if err != nil || off < 0 || off >= len(romData) {
		return "", false
	}
	return specialInteractionRole(romData[off])
}

func specialInteractionRole(script byte) (ObjectInteractionRole, bool) {
	switch script {
	case textScriptPokemonCenterNurse:
		return InteractionPokemonCenterNurse, true
	case textScriptMart:
		return InteractionMart, true
	case textScriptBillsPC:
		return InteractionBillsPC, true
	case textScriptPlayersPC:
		return InteractionPlayersPC, true
	case textScriptPokemonCenterPC:
		return InteractionPokemonCenterPC, true
	case textScriptPrizeVendor:
		return InteractionPrizeVendor, true
	case textScriptCableClub:
		return InteractionCableClub, true
	case textScriptVendingMachine:
		return InteractionVendingMachine, true
	default:
		return "", false
	}
}
