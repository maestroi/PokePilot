package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gsPartyMovesOffset uint16 = 0x02

	gsPlainBadgeMask   byte = 1 << 2
	gsFogBadgeMask     byte = 1 << 3
	gsStormBadgeMask   byte = 1 << 5
	gsGlacierBadgeMask byte = 1 << 6
	gsRisingBadgeMask  byte = 1 << 7
)

type gsFieldMoveSpec struct {
	id            game.FieldMoveID
	name          string
	move          uint8
	machineNumber int
	badgeName     string
	badgeMask     byte
}

var gsFieldMoves = [...]gsFieldMoveSpec{
	{id: game.FieldMoveCut, name: "CUT", move: 0x0f, machineNumber: 51, badgeName: "Hive", badgeMask: johtoBadgeHiveMask},
	{id: game.FieldMoveFly, name: "FLY", move: 0x13, machineNumber: 52, badgeName: "Storm", badgeMask: gsStormBadgeMask},
	{id: game.FieldMoveSurf, name: "SURF", move: 0x39, machineNumber: 53, badgeName: "Fog", badgeMask: gsFogBadgeMask},
	{id: game.FieldMoveStrength, name: "STRENGTH", move: 0x46, machineNumber: 54, badgeName: "Plain", badgeMask: gsPlainBadgeMask},
	{id: game.FieldMoveFlash, name: "FLASH", move: 0x94, machineNumber: 55, badgeName: "Zephyr", badgeMask: johtoBadgeZephyrMask},
	{id: game.FieldMoveWhirlpool, name: "WHIRLPOOL", move: 0xfa, machineNumber: 56, badgeName: "Glacier", badgeMask: gsGlacierBadgeMask},
	{id: game.FieldMoveWaterfall, name: "WATERFALL", move: 0x7f, machineNumber: 57, badgeName: "Rising", badgeMask: gsRisingBadgeMask},
	{id: game.FieldMoveHeadbutt, name: "HEADBUTT", move: 0x1d, machineNumber: 2},
	{id: game.FieldMoveRockSmash, name: "ROCK SMASH", move: 0xf9, machineNumber: 8},
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

// gsIsMenuFieldMove mirrors pret/pokegold data/mon_menu.asm. The Gen-II mon
// submenu includes both progression field moves and utility actions such as
// Dig/Rock Smash. Unknown utility actions still need a placeholder entry so a
// later semantic move keeps its native menu index.
func gsIsMenuFieldMove(move uint8) bool {
	if _, ok := gsFieldMoveByMove(move); ok {
		return true
	}
	switch move {
	case 0x5b, // Dig
		0x64, // Teleport
		0x87, // Softboiled
		0xd0, // Milk Drink
		0xe6: // Sweet Scent
		return true
	default:
		return false
	}
}

func gsMachineItem(spec gsFieldMoveSpec) (uint8, bool) {
	index := spec.machineNumber - 1
	if index < 0 || index >= len(gsdata.MachineItems) {
		return 0, false
	}
	return gsdata.MachineItems[index], true
}

func gsMachineOwned(reader game.MemoryReader, machineNumber int) bool {
	index := machineNumber - 1
	if reader == nil || index < 0 || index >= sym.TMsHMsCount {
		return false
	}
	return reader.Peek8(sym.TMsHMs+uint16(index)) != 0
}

func gsPartyCount(reader game.MemoryReader) (int, error) {
	count := int(reader.Peek8(sym.PartyCount))
	if count > 6 {
		return 0, fmt.Errorf("gs profile: party count %d outside 0..6", count)
	}
	return count, nil
}

func gsPartyMoveSlot(reader game.MemoryReader, count int, move uint8) int {
	for slot := 0; slot < count; slot++ {
		base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize + gsPartyMovesOffset
		for moveSlot := 0; moveSlot < 4; moveSlot++ {
			if reader.Peek8(base+uint16(moveSlot)) == move {
				return slot
			}
		}
	}
	return -1
}

func gsIsHMMove(move uint8) bool {
	switch move {
	case 0x0f, 0x13, 0x39, 0x46, 0x94, 0xfa, 0x7f:
		return true
	default:
		return false
	}
}

func gsMonCanPlaceMachine(reader game.MemoryReader, romData []byte, baseData int, slot int, spec gsFieldMoveSpec) (bool, error) {
	base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize
	species := reader.Peek8(base)
	if species == 0 || gsPartySlotIsEgg(reader, slot) {
		return false, nil
	}
	compatible, err := gsrom.CanLearnTMHMAt(romData, baseData, species, spec.machineNumber)
	if err != nil {
		return false, err
	}
	if !compatible {
		return false, nil
	}

	moves := base + gsPartyMovesOffset
	for i := 0; i < 4; i++ {
		known := reader.Peek8(moves + uint16(i))
		if known == 0 || known == spec.move {
			return true, nil
		}
	}
	for i := 0; i < 4; i++ {
		if !gsIsHMMove(reader.Peek8(moves + uint16(i))) {
			return true, nil
		}
	}
	return false, nil
}

// DecodeFieldMoveCapability projects Gold/Silver's Johto badge bits, TM/HM
// pocket, party move layout, and ROM-owned compatibility bitmap into the
// generation-neutral field-move contract.
func (*Profile) DecodeFieldMoveCapability(reader game.MemoryReader, romData []byte, id game.FieldMoveID) (game.FieldMoveCapability, bool, error) {
	spec, ok := gsFieldMoveByID(id)
	if !ok {
		return game.FieldMoveCapability{}, false, nil
	}
	if reader == nil {
		return game.FieldMoveCapability{}, true, fmt.Errorf("gs profile: nil memory reader")
	}

	count, err := gsPartyCount(reader)
	if err != nil {
		return game.FieldMoveCapability{}, true, err
	}
	slot := gsPartyMoveSlot(reader, count, spec.move)
	badgeOwned := spec.badgeMask == 0 || reader.Peek8(sym.JohtoBadges)&spec.badgeMask != 0
	machineOwned := gsMachineOwned(reader, spec.machineNumber)

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

	baseData, err := gsrom.LocateBaseData(romData)
	if err != nil {
		return game.FieldMoveCapability{}, true, fmt.Errorf("gs profile: locate BaseData for %s compatibility: %w", spec.name, err)
	}
	for partySlot := 0; partySlot < count; partySlot++ {
		canPlace, err := gsMonCanPlaceMachine(reader, romData, baseData, partySlot, spec)
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

// DecodeFieldMoveMenu projects the selected Gen-II party member's native mon
// submenu in exactly the order the ROM builds it: scan the four move slots,
// keep only moves recognized by IsFieldMove, and then append the non-move menu
// entries. Generic field-move execution only needs that leading move prefix.
// Empty semantic ids intentionally preserve native positions for field actions
// outside the portable progression vocabulary.
func (*Profile) DecodeFieldMoveMenu(reader game.MemoryReader) game.FieldMoveMenuState {
	if reader == nil {
		return game.FieldMoveMenuState{}
	}
	count := int(reader.Peek8(sym.PartyCount))
	slot := int(reader.Peek8(sym.CurPartyMon))
	if count <= 0 || count > 6 || slot < 0 || slot >= count {
		return game.FieldMoveMenuState{}
	}

	moves := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize + gsPartyMovesOffset
	entries := make([]game.FieldMoveID, 0, 4)
	for i := 0; i < 4; i++ {
		move := reader.Peek8(moves + uint16(i))
		if move == 0 || !gsIsMenuFieldMove(move) {
			continue
		}
		id, _ := gsFieldMoveByMove(move)
		entries = append(entries, id)
	}
	return game.FieldMoveMenuState{Entries: entries}
}

func (*Profile) NativeFieldMove(id game.FieldMoveID) (game.NativeFieldMove, bool) {
	spec, ok := gsFieldMoveByID(id)
	if !ok {
		return game.NativeFieldMove{}, false
	}
	item, ok := gsMachineItem(spec)
	if !ok {
		return game.NativeFieldMove{}, false
	}
	return game.NativeFieldMove{MachineItemID: uint16(item), MoveID: uint16(spec.move)}, true
}

var _ game.FieldMoveDecoder = (*Profile)(nil)
