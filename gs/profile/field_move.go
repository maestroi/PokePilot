package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/gs/sym"
)

type gsFieldMoveSpec struct {
	id          game.FieldMoveID
	name        string
	item        uint8
	move        uint8
	badgeName   string
	badgeMask   uint8
}

var gsFieldMoves = [...]gsFieldMoveSpec{
	{id: game.FieldMoveCut, name: "CUT", item: 0xf3, move: 15, badgeName: "Hive", badgeMask: 1 << 1},
	{id: game.FieldMoveFly, name: "FLY", item: 0xf4, move: 19, badgeName: "Storm", badgeMask: 1 << 5},
	{id: game.FieldMoveSurf, name: "SURF", item: 0xf5, move: 57, badgeName: "Fog", badgeMask: 1 << 3},
	{id: game.FieldMoveStrength, name: "STRENGTH", item: 0xf6, move: 70, badgeName: "Plain", badgeMask: 1 << 2},
	{id: game.FieldMoveFlash, name: "FLASH", item: 0xf7, move: 148, badgeName: "Zephyr", badgeMask: 1 << 0},
	{id: game.FieldMoveWhirlpool, name: "WHIRLPOOL", item: 0xf8, move: 250, badgeName: "Glacier", badgeMask: 1 << 6},
	{id: game.FieldMoveWaterfall, name: "WATERFALL", item: 0xf9, move: 127, badgeName: "Rising", badgeMask: 1 << 7},
	{id: game.FieldMoveHeadbutt, name: "HEADBUTT", item: 0xc0, move: 29},
}

func gsFieldMoveByID(id game.FieldMoveID) (gsFieldMoveSpec, bool) {
	for _, spec := range gsFieldMoves {
		if spec.id == id {
			return spec, true
		}
	}
	return gsFieldMoveSpec{}, false
}

func gsFieldMoveByMove(move uint8) (game.FieldMoveID, bool) {
	for _, spec := range gsFieldMoves {
		if spec.move == move {
			return spec.id, true
		}
	}
	return "", false
}

func gsPartyMoveSlot(reader game.MemoryReader, move uint8) int {
	count := int(reader.Peek8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	for slot := 0; slot < count; slot++ {
		base := sym.PartyMon1 + uint16(slot)*0x30
		for i := uint16(0); i < 4; i++ {
			if reader.Peek8(base+2+i) == move {
				return slot
			}
		}
	}
	return -1
}

func gsIsHMMove(move uint8) bool {
	switch move {
	case 15, 19, 57, 70, 148, 250, 127:
		return true
	default:
		return false
	}
}

func gsMonCanPlaceMachine(reader game.MemoryReader, table gsrom.TMHMCompatibility, slot int, item, move uint8) (bool, error) {
	base := sym.PartyMon1 + uint16(slot)*0x30
	species := reader.Peek8(base)
	if species == 0 || species == 0xfd {
		return false, nil
	}
	compatible, err := table.CanLearn(species, item)
	if err != nil {
		return false, err
	}
	if !compatible {
		return false, nil
	}
	for i := uint16(0); i < 4; i++ {
		known := reader.Peek8(base + 2 + i)
		if known == 0 || known == move {
			return true, nil
		}
	}
	for i := uint16(0); i < 4; i++ {
		if !gsIsHMMove(reader.Peek8(base + 2 + i)) {
			return true, nil
		}
	}
	return false, nil
}

// DecodeFieldMoveCapability keeps Gen-II badge bits, machine quantities, party
// move layout and BaseData compatibility behind the Gold/Silver profile.
func (*Profile) DecodeFieldMoveCapability(reader game.MemoryReader, romData []byte, id game.FieldMoveID) (game.FieldMoveCapability, bool, error) {
	spec, ok := gsFieldMoveByID(id)
	if !ok {
		return game.FieldMoveCapability{}, false, nil
	}
	if reader == nil {
		return game.FieldMoveCapability{}, true, fmt.Errorf("gs profile: nil memory reader")
	}

	machine, ok := gsrom.MachineNumberForItem(spec.item)
	if !ok {
		return game.FieldMoveCapability{}, true, fmt.Errorf("gs profile: %s native machine %#02x is not in the TM/HM pocket", spec.name, spec.item)
	}
	slot := gsPartyMoveSlot(reader, spec.move)
	badgeOwned := spec.badgeMask == 0 || reader.Peek8(sym.JohtoBadges)&spec.badgeMask != 0
	machineOwned := reader.Peek8(sym.TMsHMs+uint16(machine-1)) != 0
	capability := game.FieldMoveCapability{
		Move:          spec.id,
		Name:          spec.name,
		BadgeRequired: spec.badgeName,
		BadgeOwned:    badgeOwned,
		MachineOwned:  machineOwned,
		Learned:       slot >= 0,
		PartySlot:     slot,
		Usable:        badgeOwned && slot >= 0,
	}
	if capability.Usable {
		capability.Preparable = true
		return capability, true, nil
	}
	if !badgeOwned || !machineOwned || len(romData) == 0 {
		return capability, true, nil
	}

	table, err := gsrom.ParseTMHMCompatibility(romData)
	if err != nil {
		return game.FieldMoveCapability{}, true, fmt.Errorf("gs profile: parse TM/HM compatibility for %s: %w", spec.name, err)
	}
	count := int(reader.Peek8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	for partySlot := 0; partySlot < count; partySlot++ {
		canPlace, err := gsMonCanPlaceMachine(reader, table, partySlot, spec.item, spec.move)
		if err != nil {
			return game.FieldMoveCapability{}, true, fmt.Errorf("gs profile: %s compatibility for party slot %d: %w", spec.name, partySlot, err)
		}
		if canPlace {
			capability.CompatiblePartySlots = append(capability.CompatiblePartySlots, partySlot)
		}
	}
	capability.Preparable = len(capability.CompatiblePartySlots) > 0
	return capability, true, nil
}

// DecodeFieldMoveMenu mirrors GetMonSubmenuItems: field moves from the selected
// mon are emitted first, in move-slot order, before STATS/SWITCH/etc. Returning
// that prefix is sufficient for generic cursor selection and avoids exporting
// MONMENUITEM_* ids.
func (*Profile) DecodeFieldMoveMenu(reader game.MemoryReader) game.FieldMoveMenuState {
	if reader == nil {
		return game.FieldMoveMenuState{}
	}
	slot := int(reader.Peek8(sym.CurPartyMon))
	count := int(reader.Peek8(sym.PartyCount))
	if slot < 0 || slot >= count || slot >= 6 {
		return game.FieldMoveMenuState{}
	}
	base := sym.PartyMon1 + uint16(slot)*0x30
	entries := make([]game.FieldMoveID, 0, 4)
	for i := uint16(0); i < 4; i++ {
		move := reader.Peek8(base + 2 + i)
		if id, ok := gsFieldMoveByMove(move); ok {
			entries = append(entries, id)
		}
	}
	return game.FieldMoveMenuState{Entries: entries}
}

func (*Profile) NativeFieldMove(id game.FieldMoveID) (game.NativeFieldMove, bool) {
	spec, ok := gsFieldMoveByID(id)
	if !ok {
		return game.NativeFieldMove{}, false
	}
	return game.NativeFieldMove{MachineItemID: uint16(spec.item), MoveID: uint16(spec.move)}, true
}

var _ game.FieldMoveDecoder = (*Profile)(nil)
