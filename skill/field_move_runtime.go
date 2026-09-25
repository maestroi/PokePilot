package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func semanticFieldMove(move FieldMove) (game.FieldMoveID, bool) {
	switch move {
	case FieldCut:
		return game.FieldMoveCut, true
	case FieldFly:
		return game.FieldMoveFly, true
	case FieldSurf:
		return game.FieldMoveSurf, true
	case FieldStrength:
		return game.FieldMoveStrength, true
	case FieldFlash:
		return game.FieldMoveFlash, true
	case FieldWhirlpool:
		return game.FieldMoveWhirlpool, true
	case FieldWaterfall:
		return game.FieldMoveWaterfall, true
	case FieldHeadbutt:
		return game.FieldMoveHeadbutt, true
	default:
		return "", false
	}
}

func fieldMoveProfileFor(m *emu.Emu) (game.FieldMoveProfile, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: field move: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: field move: detect profile: %w", err)
	}
	field, ok := profile.(game.FieldMoveProfile)
	if !ok {
		return nil, fmt.Errorf("skill: field move: profile %s@%s does not expose field-move semantics", profile.ID(), profile.Revision())
	}
	return field, nil
}

func fieldMoveCapabilityWithProfile(profile game.FieldMoveDecoder, reader game.MemoryReader, romData []byte, move FieldMove) (game.FieldMoveCapability, error) {
	if profile == nil {
		return game.FieldMoveCapability{}, fmt.Errorf("skill: field move: nil field-move decoder")
	}
	id, ok := semanticFieldMove(move)
	if !ok {
		return game.FieldMoveCapability{}, fmt.Errorf("skill: field move %d is unknown", move)
	}
	capability, supported, err := profile.DecodeFieldMoveCapability(reader, romData, id)
	if err != nil {
		return game.FieldMoveCapability{}, err
	}
	if !supported {
		return game.FieldMoveCapability{}, fmt.Errorf("%w: %s is not supported by the active game profile", ErrFieldMovePrerequisite, id)
	}
	return capability, nil
}

func fieldMoveMenuIndexWithProfile(profile game.FieldMoveDecoder, reader game.MemoryReader, move FieldMove) int {
	if profile == nil {
		return -1
	}
	id, ok := semanticFieldMove(move)
	if !ok {
		return -1
	}
	menu := profile.DecodeFieldMoveMenu(reader)
	for i, entry := range menu.Entries {
		if entry == id {
			return i
		}
	}
	return -1
}

func selectFieldMoveMenuEntryWithDecoders(m menuMachine, fieldDecoder game.FieldMoveDecoder, menuDecoder game.MenuDecoder, move FieldMove) error {
	if fieldDecoder == nil {
		return fmt.Errorf("skill: field move menu: nil field-move decoder")
	}
	if menuDecoder == nil {
		return fmt.Errorf("skill: field move menu: nil menu decoder")
	}
	idx := fieldMoveMenuIndexWithProfile(fieldDecoder, m, move)
	if idx < 0 {
		return fmt.Errorf("skill: %s field-move entry is not available", move)
	}
	if err := selectMenuItemWithDecoder(m, menuDecoder, idx); err != nil {
		return fmt.Errorf("skill: select %s field-move entry: %w", move, err)
	}
	return nil
}

func selectFieldMoveMenuEntry(m *emu.Emu, move FieldMove) error {
	field, err := fieldMoveProfileFor(m)
	if err != nil {
		return err
	}
	menu, err := menuDecoderFor(m)
	if err != nil {
		return err
	}
	return selectFieldMoveMenuEntryWithDecoders(m, field, menu, move)
}
