package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const (
	gen2HM01Item uint16 = 0xf3
	gen2HM02Item uint16 = 0xf4
	gen2HM03Item uint16 = 0xf5
	gen2HM04Item uint16 = 0xf6
	gen2HM05Item uint16 = 0xf7
	gen2HM06Item uint16 = 0xf8
	gen2HM07Item uint16 = 0xf9
	gen2TM02Item uint16 = 0xc0

	gen2MoveCut       uint16 = 0x0f
	gen2MoveFly       uint16 = 0x13
	gen2MoveHeadbutt  uint16 = 0x1d
	gen2MoveSurf      uint16 = 0x39
	gen2MoveStrength  uint16 = 0x46
	gen2MoveWaterfall uint16 = 0x7f
	gen2MoveFlash     uint16 = 0x94
	gen2MoveWhirlpool uint16 = 0xfa

	gen2TMHMPocket = 3
)

type gen2FieldMoveSpec struct {
	move         game.FieldMoveID
	name         string
	item         uint16
	nativeMove   uint16
	tmhmIndex    uint16
	badge        string
	badgeMask    byte
}

var gen2FieldMoveSpecs = map[game.FieldMoveID]gen2FieldMoveSpec{
	game.FieldMoveCut:       {move: game.FieldMoveCut, name: "Cut", item: gen2HM01Item, nativeMove: gen2MoveCut, tmhmIndex: 50, badge: "hive", badgeMask: 1 << 1},
	game.FieldMoveFly:       {move: game.FieldMoveFly, name: "Fly", item: gen2HM02Item, nativeMove: gen2MoveFly, tmhmIndex: 51, badge: "storm", badgeMask: 1 << 4},
	game.FieldMoveSurf:      {move: game.FieldMoveSurf, name: "Surf", item: gen2HM03Item, nativeMove: gen2MoveSurf, tmhmIndex: 52, badge: "fog", badgeMask: 1 << 3},
	game.FieldMoveStrength:  {move: game.FieldMoveStrength, name: "Strength", item: gen2HM04Item, nativeMove: gen2MoveStrength, tmhmIndex: 53, badge: "plain", badgeMask: 1 << 2},
	game.FieldMoveFlash:     {move: game.FieldMoveFlash, name: "Flash", item: gen2HM05Item, nativeMove: gen2MoveFlash, tmhmIndex: 54, badge: "zephyr", badgeMask: 1 << 0},
	game.FieldMoveWhirlpool: {move: game.FieldMoveWhirlpool, name: "Whirlpool", item: gen2HM06Item, nativeMove: gen2MoveWhirlpool, tmhmIndex: 55, badge: "glacier", badgeMask: 1 << 6},
	game.FieldMoveWaterfall: {move: game.FieldMoveWaterfall, name: "Waterfall", item: gen2HM07Item, nativeMove: gen2MoveWaterfall, tmhmIndex: 56, badge: "rising", badgeMask: 1 << 7},
	game.FieldMoveHeadbutt:  {move: game.FieldMoveHeadbutt, name: "Headbutt", item: gen2TM02Item, nativeMove: gen2MoveHeadbutt, tmhmIndex: 1},
}

// TMHMPocketState is the small Gen-II-only projection needed by the Johto
// controller while teaching a machine through the real PACK UI.
type TMHMPocketState struct {
	Visible        bool
	CurrentMachine int // retail TM/HM number, 1..57
}

func (*Profile) DecodeTMHMPocket(reader game.MemoryReader) TMHMPocketState {
	if reader == nil || reader.Peek8(sym.CurPocket) != gen2TMHMPocket {
		return TMHMPocketState{}
	}
	current := int(reader.Peek8(sym.CurItem))
	if current < 1 || current > sym.TMsHMsCount {
		current = 0
	}
	return TMHMPocketState{Visible: true, CurrentMachine: current}
}

func (*Profile) NativeFieldMove(move game.FieldMoveID) (game.NativeFieldMove, bool) {
	spec, ok := gen2FieldMoveSpecs[move]
	if !ok {
		return game.NativeFieldMove{}, false
	}
	return game.NativeFieldMove{MachineItemID: spec.item, MoveID: spec.nativeMove}, true
}

func (*Profile) DecodeFieldMoveCapability(
	reader game.MemoryReader,
	_ []byte,
	move game.FieldMoveID,
) (game.FieldMoveCapability, bool, error) {
	spec, ok := gen2FieldMoveSpecs[move]
	if !ok || reader == nil {
		return game.FieldMoveCapability{}, ok, nil
	}

	capability := game.FieldMoveCapability{
		Move:          move,
		Name:          spec.name,
		BadgeRequired: spec.badge,
		BadgeOwned:    spec.badgeMask == 0 || reader.Peek8(sym.JohtoBadges)&spec.badgeMask != 0,
		MachineOwned:  reader.Peek8(sym.TMsHMs+spec.tmhmIndex) != 0,
		PartySlot:     -1,
	}

	count := int(reader.Peek8(sym.PartyCount))
	if count > 6 {
		count = 6
	}
	for partySlot := 0; partySlot < count; partySlot++ {
		base := sym.PartyMon1 + uint16(partySlot)*sym.PartyMonSize
		species := reader.Peek8(base)
		for moveSlot := 0; moveSlot < 4; moveSlot++ {
			if uint16(reader.Peek8(base+gen2PartyMovesOffset+uint16(moveSlot))) == spec.nativeMove {
				capability.Learned = true
				capability.PartySlot = partySlot
				break
			}
		}
		if capability.Learned {
			break
		}
		if move == game.FieldMoveCut && gen2EarlyCutCompatible(species) {
			capability.CompatiblePartySlots = append(capability.CompatiblePartySlots, partySlot)
		}
	}

	// Gen-II machine teaching is intentionally controller-owned for now. Do
	// not advertise Preparable to the generic EnsureFieldMove path because its
	// retained teacher is still Gen-I-shaped. Once the Johto controller has
	// taught the move, the shared field-action path sees Usable and proceeds.
	capability.Preparable = false
	capability.Usable = capability.Learned && capability.BadgeOwned
	return capability, true, nil
}

func gen2EarlyCutCompatible(species byte) bool {
	// All three starter families can learn Cut in the pinned retail
	// pokegold/pokesilver base-stat tables. This bounded early-Johto fact is
	// enough for the Ilex slice; the full 251-species TM/HM compatibility
	// parser belongs in gs/rom.
	return species >= 0x98 && species <= 0xa0
}

func gen2FieldMoveForNative(move uint16) game.FieldMoveID {
	for id, spec := range gen2FieldMoveSpecs {
		if spec.nativeMove == move {
			return id
		}
	}
	return ""
}

func (*Profile) DecodeFieldMoveMenu(reader game.MemoryReader) game.FieldMoveMenuState {
	if reader == nil {
		return game.FieldMoveMenuState{}
	}
	text := gsScreenText(reader)
	if !strings.Contains(text, "STATS") || !strings.Contains(text, "CANCEL") {
		return game.FieldMoveMenuState{}
	}
	partySlot := int(reader.Peek8(sym.CurPartyMon))
	if partySlot < 0 || partySlot >= int(reader.Peek8(sym.PartyCount)) || partySlot >= 6 {
		return game.FieldMoveMenuState{}
	}
	base := sym.PartyMon1 + uint16(partySlot)*sym.PartyMonSize + gen2PartyMovesOffset
	entries := make([]game.FieldMoveID, 0, 4)
	for slot := 0; slot < 4; slot++ {
		id := gen2FieldMoveForNative(uint16(reader.Peek8(base + uint16(slot))))
		if id != "" {
			entries = append(entries, id)
		}
	}
	return game.FieldMoveMenuState{Entries: entries}
}

var _ game.FieldMoveDecoder = (*Profile)(nil)
