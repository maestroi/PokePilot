package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

type redFieldMoveSpec struct {
	id      game.FieldMoveID
	name    string
	item    uint8
	move    uint8
	menu    uint8
	badge   state.Badge
}

var redFieldMoves = [...]redFieldMoveSpec{
	{id: game.FieldMoveCut, name: "CUT", item: redrom.HM01Item, move: 0x0f, menu: 1, badge: state.BadgeCascade},
	{id: game.FieldMoveFly, name: "FLY", item: redrom.HM01Item + 1, move: 0x13, menu: 2, badge: state.BadgeThunder},
	{id: game.FieldMoveSurf, name: "SURF", item: redrom.HM01Item + 2, move: 0x39, menu: 4, badge: state.BadgeSoul},
	{id: game.FieldMoveStrength, name: "STRENGTH", item: redrom.HM01Item + 3, move: 0x46, menu: 5, badge: state.BadgeRainbow},
	{id: game.FieldMoveFlash, name: "FLASH", item: redrom.HM01Item + 4, move: 0x94, menu: 6, badge: state.BadgeBoulder},
}

func redFieldMoveByID(id game.FieldMoveID) (redFieldMoveSpec, bool) {
	for _, spec := range redFieldMoves {
		if spec.id == id {
			return spec, true
		}
	}
	return redFieldMoveSpec{}, false
}

func redFieldMoveByMenuID(id uint8) (game.FieldMoveID, bool) {
	for _, spec := range redFieldMoves {
		if spec.menu == id {
			return spec.id, true
		}
	}
	return "", false
}

func redPartyMoveSlot(party state.PartyState, move uint8) int {
	for slot, mon := range party.Mons {
		for _, known := range mon.Moves {
			if known == move {
				return slot
			}
		}
	}
	return -1
}

func redBagHasItem(inv state.InventoryState, item uint8) bool {
	for _, entry := range inv.Items {
		if entry.ID == item && entry.Quantity > 0 {
			return true
		}
	}
	return false
}

func redMonCanPlaceMachine(romData []byte, mon state.Mon, item, move uint8) (bool, error) {
	compatible, err := redrom.CanLearnTMHM(romData, mon.Species, item)
	if err != nil {
		return false, err
	}
	if !compatible {
		return false, nil
	}
	for _, known := range mon.Moves {
		if known == move || known == 0 {
			return true, nil
		}
	}
	for _, known := range mon.Moves {
		isHM, err := redrom.IsHMMove(romData, known)
		if err != nil {
			return false, err
		}
		if !isHM {
			return true, nil
		}
	}
	return false, nil
}

// DecodeFieldMoveCapability keeps Gen I badge bits, HM item ids, party move
// layout, and TM/HM compatibility policy behind the Red/Blue profile boundary.
func (*Profile) DecodeFieldMoveCapability(reader game.MemoryReader, romData []byte, id game.FieldMoveID) (game.FieldMoveCapability, bool, error) {
	spec, ok := redFieldMoveByID(id)
	if !ok {
		return game.FieldMoveCapability{}, false, nil
	}
	if reader == nil {
		return game.FieldMoveCapability{}, true, fmt.Errorf("red profile: nil memory reader")
	}

	var mem state.Mem
	reader.PeekInto(0, mem[:])
	party := state.DecodeParty(&mem)
	slot := redPartyMoveSlot(party, spec.move)
	badgeOwned := state.DecodeProgress(&mem).Has(spec.badge)
	machineOwned := redBagHasItem(state.DecodeInventory(&mem), spec.item)

	capability := game.FieldMoveCapability{
		Move:          spec.id,
		Name:          spec.name,
		BadgeRequired: spec.badge.String(),
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

	for partySlot, mon := range party.Mons {
		canPlace, err := redMonCanPlaceMachine(romData, mon, spec.item, spec.move)
		if err != nil {
			return game.FieldMoveCapability{}, true, fmt.Errorf("red profile: %s compatibility for party slot %d: %w", spec.name, partySlot, err)
		}
		if canPlace {
			capability.CompatiblePartySlots = append(capability.CompatiblePartySlots, partySlot)
		}
	}
	capability.Preparable = len(capability.CompatiblePartySlots) > 0
	return capability, true, nil
}

// DecodeFieldMoveMenu projects Red's wFieldMoves list to semantic identities.
// Empty entries preserve the native index for non-progression actions such as
// Dig/Teleport so generic selection never shifts a later Surf/Strength entry.
func (*Profile) DecodeFieldMoveMenu(reader game.MemoryReader) game.FieldMoveMenuState {
	if reader == nil {
		return game.FieldMoveMenuState{}
	}
	entries := make([]game.FieldMoveID, 0, 4)
	for i := 0; i < 4; i++ {
		menuID := reader.Peek8(sym.FieldMoves + uint16(i))
		if menuID == 0 {
			break
		}
		id, _ := redFieldMoveByMenuID(menuID)
		entries = append(entries, id)
	}
	return game.FieldMoveMenuState{Entries: entries}
}

func (*Profile) NativeFieldMove(id game.FieldMoveID) (game.NativeFieldMove, bool) {
	spec, ok := redFieldMoveByID(id)
	if !ok {
		return game.NativeFieldMove{}, false
	}
	return game.NativeFieldMove{MachineItemID: uint16(spec.item), MoveID: uint16(spec.move)}, true
}
