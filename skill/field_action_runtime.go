package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/world"
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
		if state.CutTargetKnown && !state.CuttableAhead {
			return fmt.Errorf("no Cut target is directly in front of the player")
		}
	case FieldStrength:
		if state.BoulderTargetKnown && !state.BoulderAhead {
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
	case FieldWaterfall:
		if !state.Surfing {
			return fmt.Errorf("Waterfall requires surfing")
		}
	case FieldFly:
		return fmt.Errorf("Fly requires a destination selection; use a destination-aware transition")
	}
	return nil
}

func fieldActionEffectObservedState(state game.FieldActionState, spec FieldMoveSpec) bool {
	switch spec.Move {
	case FieldCut, FieldRockSmash, FieldWhirlpool:
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

// Waterfall queues a forced movement script. The queued-success byte alone
// is not a completed climb: prove the player actually ascended on the same
// native map and returned control. Other field moves retain their existing
// effect contract (Cut/Whirlpool clear a target; Strength/Flash set modes).
func fieldActionEffectObservedFromState(start, state game.FieldActionState, spec FieldMoveSpec) bool {
	if spec.Move == FieldWaterfall {
		return start.PositionKnown && state.PositionKnown &&
			start.NativeMapID == state.NativeMapID && start.X == state.X &&
			state.Y < start.Y && state.ActionSucceeded
	}
	return fieldActionEffectObservedState(state, spec)
}

func fieldActionCompleteFromState(start, state game.FieldActionState, spec FieldMoveSpec) bool {
	return state.Controllable && fieldActionEffectObservedFromState(start, state, spec)
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

func traversalModeForFieldActionState(state game.FieldActionState) world.TraversalMode {
	if state.Surfing {
		return world.TraversalWater
	}
	return world.TraversalLand
}
