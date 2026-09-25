package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func fieldActionDecoderFor(m *emu.Emu) (game.FieldActionDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: field action: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: field action: detect profile: %w", err)
	}
	decoder, ok := profile.(game.FieldActionDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: field action: profile %s@%s does not expose field-action semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func validateFieldActionRuntime(state game.FieldActionState, spec FieldMoveSpec) error {
	switch spec.Move {
	case FieldCut:
		if !state.CuttableAhead {
			return fmt.Errorf("no Cut target is directly in front of the player")
		}
	case FieldStrength:
		if !state.BoulderAhead {
			return fmt.Errorf("no boulder is directly in front of the player")
		}
	case FieldSurf:
		if state.Surfing {
			return fmt.Errorf("player is already surfing")
		}
	case FieldFlash:
		if state.Lit {
			return fmt.Errorf("current area is already lit")
		}
	case FieldFly:
		return fmt.Errorf("Fly requires a destination selection; use a destination-aware transition")
	}
	return nil
}

func fieldActionEffectObservedState(state game.FieldActionState, spec FieldMoveSpec) bool {
	switch spec.Move {
	case FieldCut:
		return state.ActionSucceeded
	case FieldSurf:
		return state.ActionSucceeded && state.Surfing
	case FieldStrength:
		return state.StrengthActive
	case FieldFlash:
		return state.Lit
	default:
		return false
	}
}

func fieldActionCompleteState(state game.FieldActionState, spec FieldMoveSpec) bool {
	return fieldActionEffectObservedState(state, spec) && state.Controllable
}

func fieldActionResultFromState(move FieldMove, slot int, state game.FieldActionState) FieldActionResult {
	var actionResult uint8
	if state.ActionSucceeded {
		actionResult = 1
	}
	return FieldActionResult{
		Move:           move,
		PartySlot:      slot,
		ActionResult:   actionResult,
		Surfing:        state.Surfing,
		StrengthActive: state.StrengthActive,
		Lit:            state.Lit,
	}
}
