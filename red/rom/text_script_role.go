package rom

import "github.com/maestroi/pokepilot/gen1rom"

// ObjectInteractionRole describes Red's built-in text-script dispatch for a
// map object. These scripts do not behave like ordinary NPC dialogue: the home
// text dispatcher transfers control to a service/menu handler instead of just
// printing text. Generic Talk must therefore not own them.
type ObjectInteractionRole string

const (
	InteractionPokemonCenterNurse ObjectInteractionRole = "pokemon_center_nurse"
	InteractionMart               ObjectInteractionRole = "mart"
	InteractionBillsPC            ObjectInteractionRole = "bills_pc"
	InteractionPlayersPC          ObjectInteractionRole = "players_pc"
	InteractionPokemonCenterPC    ObjectInteractionRole = "pokemon_center_pc"
	InteractionPrizeVendor        ObjectInteractionRole = "prize_vendor"
	InteractionCableClub          ObjectInteractionRole = "cable_club"
	InteractionVendingMachine     ObjectInteractionRole = "vending_machine"
)

// SpecialInteractionActor is a map object whose text pointer starts with one
// of Red's TX_SCRIPT_* service dispatch bytes rather than an ordinary text
// script. Coordinates are the object's ROM home coordinates.
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

// SpecialInteractionActors returns person-like objects whose text entry is one
// of Red's built-in service scripts. The ROM is the source of truth: this does
// not maintain a map/coordinate list of nurses, clerks, receptionists, etc.
//
// textPointerTables includes alternate tables selected by map scripts (notably
// Viridian Mart before/after Oak's parcel). If any reachable table classifies
// an object as a service actor, generic Talk must not claim that object.
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
				continue // no text, or a trainer/item object
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
		// Home-bank scripts such as mart shelves live below $4000 even when the
		// map itself is banked elsewhere.
		bank = 0
	}
	off, err := bankedOffset(bank, ptr)
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
