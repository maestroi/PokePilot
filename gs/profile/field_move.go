package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

type gsFieldMoveSpec struct {
	id           game.FieldMoveID
	name         string
	machineItem  uint16
	machineIndex uint16
	moveID       uint16
	badgeName    string
	badgeBit     byte
	menuItem     byte
}

var gsFieldMoves = [...]gsFieldMoveSpec{
	{id: game.FieldMoveCut, name: "CUT", machineItem: 0xf3, machineIndex: 50, moveID: 0x0f, badgeName: "Hive", badgeBit: 1, menuItem: 1},
	{id: game.FieldMoveFly, name: "FLY", machineItem: 0xf4, machineIndex: 51, moveID: 0x13, badgeName: "Storm", badgeBit: 4, menuItem: 2},
	{id: game.FieldMoveSurf, name: "SURF", machineItem: 0xf5, machineIndex: 52, moveID: 0x39, badgeName: "Fog", badgeBit: 3, menuItem: 3},
	{id: game.FieldMoveStrength, name: "STRENGTH", machineItem: 0xf6, machineIndex: 53, moveID: 0x46, badgeName: "Plain", badgeBit: 2, menuItem: 4},
	{id: game.FieldMoveFlash, name: "FLASH", machineItem: 0xf7, machineIndex: 54, moveID: 0x94, badgeName: "Zephyr", badgeBit: 0, menuItem: 6},
	{id: game.FieldMoveWhirlpool, name: "WHIRLPOOL", machineItem: 0xf8, machineIndex: 55, moveID: 0xfa, badgeName: "Glacier", badgeBit: 6, menuItem: 7},
	{id: game.FieldMoveWaterfall, name: "WATERFALL", machineItem: 0xf9, machineIndex: 56, moveID: 0x7f, badgeName: "Rising", badgeBit: 7, menuItem: 5},
	{id: game.FieldMoveHeadbutt, name: "HEADBUTT", machineItem: 0xc0, machineIndex: 1, moveID: 0x1d, menuItem: 11},
}

func gsFieldMoveSpecFor(id game.FieldMoveID) (gsFieldMoveSpec, bool) {
	for _, spec := range gsFieldMoves {
		if spec.id == id {
			return spec, true
		}
	}
	return gsFieldMoveSpec{}, false
}

func (*Profile) NativeFieldMove(id game.FieldMoveID) (game.NativeFieldMove, bool) {
	spec, ok := gsFieldMoveSpecFor(id)
	if !ok {
		return game.NativeFieldMove{}, false
	}
	return game.NativeFieldMove{MachineItemID: spec.machineItem, MoveID: spec.moveID}, true
}

func gsPartyMoveSlot(reader game.MemoryReader, move byte) int {
	count := int(reader.Peek8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	for slot := 0; slot < count; slot++ {
		base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize
		for moveSlot := uint16(0); moveSlot < 4; moveSlot++ {
			if reader.Peek8(base+2+moveSlot) == move {
				return slot
			}
		}
	}
	return -1
}

func gsPotentialMachineCarriers(reader game.MemoryReader) []int {
	count := int(reader.Peek8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	out := make([]int, 0, count)
	for slot := 0; slot < count; slot++ {
		rawSpecies := reader.Peek8(sym.PartyMon1 + uint16(slot)*sym.PartyMonSize)
		if rawSpecies == 0 || gsdata.IsEgg(rawSpecies) {
			continue
		}
		out = append(out, slot)
	}
	return out
}

// DecodeFieldMoveCapability owns Gold/Silver badge, machine-pocket and learned
// move decoding. Exact species compatibility is deliberately verified by the
// native TM/HM party screen during teaching; before that screen is open the
// candidate list is the non-egg party set, so execution fails closed if Gold
// reports every member as NOT ABLE.
func (*Profile) DecodeFieldMoveCapability(reader game.MemoryReader, _ []byte, id game.FieldMoveID) (game.FieldMoveCapability, bool, error) {
	spec, ok := gsFieldMoveSpecFor(id)
	if !ok {
		return game.FieldMoveCapability{}, false, nil
	}
	if reader == nil {
		return game.FieldMoveCapability{}, true, fmt.Errorf("gs profile: nil field-move reader")
	}
	slot := gsPartyMoveSlot(reader, byte(spec.moveID))
	badgeOwned := spec.badgeName == "" || reader.Peek8(sym.JohtoBadges)&(1<<spec.badgeBit) != 0
	machineOwned := reader.Peek8(sym.TMsHMs+spec.machineIndex) != 0
	candidates := gsPotentialMachineCarriers(reader)
	learned := slot >= 0
	return game.FieldMoveCapability{
		Move:                 id,
		Name:                 spec.name,
		BadgeRequired:        spec.badgeName,
		BadgeOwned:           badgeOwned,
		MachineOwned:         machineOwned,
		Learned:              learned,
		PartySlot:            slot,
		CompatiblePartySlots: candidates,
		Preparable:           badgeOwned && machineOwned && len(candidates) > 0,
		Usable:               badgeOwned && learned,
	}, true, nil
}

var gsMonMenuFieldMove = map[byte]game.FieldMoveID{
	1: game.FieldMoveCut,
	2: game.FieldMoveFly,
	3: game.FieldMoveSurf,
	4: game.FieldMoveStrength,
	5: game.FieldMoveWaterfall,
	6: game.FieldMoveFlash,
	7: game.FieldMoveWhirlpool,
	11: game.FieldMoveHeadbutt,
}

func (*Profile) DecodeFieldMoveMenu(reader game.MemoryReader) game.FieldMoveMenuState {
	if reader == nil {
		return game.FieldMoveMenuState{}
	}
	count := int(reader.Peek8(sym.MonSubmenuCount))
	if count <= 0 || count > 8 || int(reader.Peek8(sym.TwoDMenuNumRows)) != count ||
		reader.Peek8(sym.MenuJoypadFilter) != gen2PadA|gen2PadB {
		return game.FieldMoveMenuState{}
	}
	out := game.FieldMoveMenuState{Entries: make([]game.FieldMoveID, count)}
	for i := 0; i < count; i++ {
		out.Entries[i] = gsMonMenuFieldMove[reader.Peek8(sym.MonSubmenuItems+uint16(i))]
	}
	return out
}

var _ game.FieldMoveDecoder = (*Profile)(nil)
