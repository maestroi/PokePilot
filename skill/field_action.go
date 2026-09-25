package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// ErrFieldMovePrerequisite reports that a field move cannot be used or
// prepared from the current party/progress: the required badge or HM is
// missing. That is a stable, replan-able blockage, not a controller fault.
var ErrFieldMovePrerequisite = errors.New("skill: field move prerequisite is missing")

// FieldMove identifies one progression-relevant out-of-battle move.
// It is deliberately separate from the raw move ID and wFieldMoves menu ID:
// those are ROM encodings, while this is the capability vocabulary used by
// routing, party retention, and execution code.
type FieldMove uint8

const (
	FieldCut FieldMove = iota
	FieldFly
	FieldSurf
	FieldStrength
	FieldFlash
	// Gen II extends the portable field-move vocabulary. Red/Blue profiles
	// deliberately report these as unsupported rather than assigning fake
	// native ids.
	FieldWhirlpool
	FieldWaterfall
	FieldHeadbutt
)

func (m FieldMove) String() string {
	if id, ok := semanticFieldMove(m); ok {
		return strings.ToUpper(string(id))
	}
	return fmt.Sprintf("field-move(%d)", uint8(m))
}

// SemanticFieldMoves returns the portable field-move vocabulary. Concrete
// profiles decide which moves exist and how their prerequisites/menu entries
// are encoded.
func SemanticFieldMoves() []FieldMove {
	return []FieldMove{
		FieldCut, FieldFly, FieldSurf, FieldStrength, FieldFlash,
		FieldWhirlpool, FieldWaterfall, FieldHeadbutt,
	}
}

// EnsureFieldMove makes move usable by the current party. Capability and
// carrier decisions are generation-neutral; the active move-learning executor
// is only an adapter for applying the profile's native machine mapping.
func EnsureFieldMove(m *emu.Emu, move FieldMove) (int, error) {
	profile, err := fieldMoveProfileFor(m)
	if err != nil {
		return -1, err
	}
	return ensureFieldMoveWithProfile(profile, m, m.ROM(), move, func(native game.NativeFieldMove) error {
		// The current move-learning executor is still Gen-I-shaped. Keep that
		// limitation at this adapter edge instead of baking it into generic
		// field capability/preparation semantics.
		if native.MachineItemID == 0 || native.MachineItemID > 0xff || native.MoveID == 0 || native.MoveID > 0xff {
			return fmt.Errorf("native machine/item ids %#04x/%#04x exceed current move-learning executor range",
				native.MachineItemID, native.MoveID)
		}
		result, err := TeachTMHM(m, uint8(native.MachineItemID), true)
		if err != nil {
			return err
		}
		if uint16(result.Decision.Machine.Move) != native.MoveID {
			return fmt.Errorf("machine %#04x mapped to move %#04x, want %#04x",
				native.MachineItemID, result.Decision.Machine.Move, native.MoveID)
		}
		return nil
	})
}

type fieldMoveTeachFunc func(game.NativeFieldMove) error

func ensureFieldMoveWithProfile(
	profile game.FieldMoveDecoder,
	reader game.MemoryReader,
	romData []byte,
	move FieldMove,
	teach fieldMoveTeachFunc,
) (int, error) {
	capability, err := fieldMoveCapabilityWithProfile(profile, reader, romData, move)
	if err != nil {
		return -1, err
	}
	name := capability.Name
	if name == "" {
		name = move.String()
	}
	if capability.BadgeRequired != "" && !capability.BadgeOwned {
		return -1, fmt.Errorf("%w: %s requires the %s Badge", ErrFieldMovePrerequisite, name, capability.BadgeRequired)
	}
	if capability.Usable && capability.PartySlot >= 0 {
		return capability.PartySlot, nil
	}
	if !capability.MachineOwned {
		return -1, fmt.Errorf("%w: %s teaching machine is not owned", ErrFieldMovePrerequisite, name)
	}
	if !capability.Preparable {
		return -1, fmt.Errorf("%w: %s has no compatible current-party carrier", ErrFieldMovePrerequisite, name)
	}
	if teach == nil {
		return -1, fmt.Errorf("skill: teach %s: no move-learning executor is available", name)
	}

	id, _ := semanticFieldMove(move)
	native, ok := profile.NativeFieldMove(id)
	if !ok {
		return -1, fmt.Errorf("%w: %s has no native teaching mapping", ErrFieldMovePrerequisite, name)
	}
	if err := teach(native); err != nil {
		return -1, fmt.Errorf("skill: teach %s: %w", name, err)
	}

	capability, err = fieldMoveCapabilityWithProfile(profile, reader, romData, move)
	if err != nil {
		return -1, fmt.Errorf("skill: teach %s: verify capability: %w", name, err)
	}
	if !capability.Usable || capability.PartySlot < 0 {
		return -1, fmt.Errorf("skill: teach %s: learned field capability was not verified", name)
	}
	return capability.PartySlot, nil
}

func fieldMoveMenuIndex(m *emu.Emu, move FieldMove) int {
	profile, err := fieldMoveProfileFor(m)
	if err != nil {
		return -1
	}
	return fieldMoveMenuIndexWithProfile(profile, m, move)
}

const fieldActionBudget = 3000

// FieldActionResult is the positively observed result of UseFieldMove.
type FieldActionResult struct {
	Move           FieldMove
	PartySlot      int
	ActionResult   uint8
	Surfing        bool
	StrengthActive bool
	Lit            bool
}

// settleFieldAction waits for the ROM-side effect and for control to return.
// Field moves may print ordinary text after changing state (Strength is the
// important case). Page those text boxes with A, but never select an open menu
// blindly; the active profile distinguishes result text from choice surfaces.
func settleFieldAction(m menuMachine, spec FieldMoveSpec, decoder game.FieldActionDecoder) error {
	for spent := 0; spent < fieldActionBudget; spent += 10 {
		runtime := decoder.DecodeFieldAction(m)
		if fieldActionCompleteState(runtime, spec) {
			return nil
		}
		if runtime.ChoiceVisible {
			return fmt.Errorf("field move exposed an unexpected choice prompt")
		}
		if runtime.ResultTextActive {
			m.Tap(emu.A, 3, 7)
			continue
		}
		// A failed field action returns to the overworld without the positive
		// effect. Once that has happened there is nothing useful to wait for.
		if spent >= 50 && runtime.Controllable && !fieldActionEffectObservedState(runtime, spec) {
			return fmt.Errorf("field move returned to the overworld without its expected effect")
		}
		m.StepFrames(10)
	}
	return fmt.Errorf("field move did not settle within %d frames", fieldActionBudget)
}

func closeFieldActionToOverworld(m menuMachine, decoder game.FieldActionDecoder) error {
	for i := 0; i < 80; i++ {
		runtime := decoder.DecodeFieldAction(m)
		if runtime.Controllable && !runtime.ResultTextActive {
			return nil
		}
		if runtime.ChoiceVisible {
			return fmt.Errorf("unexpected choice prompt while closing field-action UI")
		}
		m.Tap(emu.B, 3, 7)
		m.StepFrames(20)
	}
	runtime := decoder.DecodeFieldAction(m)
	return fmt.Errorf("field-action UI did not close to overworld: %s", runtime.DebugText)
}

// UseFieldMove executes one supported field move through the real START ->
// POKEMON -> field-move menu. It auto-teaches the HM only when the badge,
// owned HM, and generic compatibility policy make that legal, then verifies
// the ROM-side effect from live state rather than trusting timing/dialogue.
// Fly is represented by the same capability abstraction but needs a caller-
// supplied destination, so destination-free execution rejects it explicitly.
func UseFieldMove(m *emu.Emu, move FieldMove) (FieldActionResult, error) {
	decoder, err := fieldActionDecoderFor(m)
	if err != nil {
		return FieldActionResult{}, err
	}
	return useFieldMoveWithDecoder(m, move, decoder)
}

func useFieldMoveWithDecoder(m *emu.Emu, move FieldMove, decoder game.FieldActionDecoder) (FieldActionResult, error) {
	menu, err := menuDecoderFor(m)
	if err != nil {
		return FieldActionResult{}, err
	}
	party, err := partyMenuDecoderFor(m)
	if err != nil {
		return FieldActionResult{}, err
	}
	spec, ok := FieldMoveSpecFor(move)
	if !ok {
		return FieldActionResult{}, fmt.Errorf("skill: field move %d is unknown", move)
	}
	runtime := decoder.DecodeFieldAction(m)
	if !runtime.Controllable {
		return FieldActionResult{}, fmt.Errorf("skill: %s: player is not controllable", spec.Name)
	}
	if err := validateFieldActionRuntime(runtime, spec); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: invalid context: %w", spec.Name, err)
	}

	slot, err := EnsureFieldMove(m, move)
	if err != nil {
		return FieldActionResult{}, err
	}
	if err := openStartMenuEntryWithDecoder(m, menu, startMenuPokemon); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: open POKEMON: %w", spec.Name, err)
	}
	if !waitMenuUntil(m, 1000, func() bool {
		s := party.DecodePartyMenu(m)
		return s.Visible && s.Kind == game.PartyMenuFieldMove
	}) {
		return FieldActionResult{}, fmt.Errorf("skill: %s: field-move party menu did not appear", spec.Name)
	}
	if err := selectPartySlotWithDecoder(m, party, slot); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: select party slot %d: %w", spec.Name, slot, err)
	}

	if err := selectFieldMoveMenuEntry(m, move); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: party slot %d: %w", spec.Name, slot, err)
	}
	m.StepFrames(30)
	if err := settleFieldAction(m, spec, decoder); err != nil {
		runtime = decoder.DecodeFieldAction(m)
		closeErr := closeFieldActionToOverworld(m, decoder)
		if closeErr != nil {
			return FieldActionResult{}, fmt.Errorf("skill: %s did not complete: %v; succeeded=%v surfing=%v strength=%v lit=%v screen=%q; cleanup: %v",
				spec.Name, err, runtime.ActionSucceeded, runtime.Surfing, runtime.StrengthActive, runtime.Lit, runtime.DebugText, closeErr)
		}
		return FieldActionResult{}, fmt.Errorf("skill: %s did not complete: %v; succeeded=%v surfing=%v strength=%v lit=%v screen=%q",
			spec.Name, err, runtime.ActionSucceeded, runtime.Surfing, runtime.StrengthActive, runtime.Lit, runtime.DebugText)
	}

	runtime = decoder.DecodeFieldAction(m)
	return fieldActionResultFromState(move, slot, runtime), nil
}

// TeachSurf and TeachStrength are compatibility-sized entry points for story
// code that needs to prepare a field capability before reaching its target.
func TeachSurf(m *emu.Emu) (int, error)     { return EnsureFieldMove(m, FieldSurf) }
func TeachStrength(m *emu.Emu) (int, error) { return EnsureFieldMove(m, FieldStrength) }

// Surf enters surfing mode through the shared field-action executor.
func Surf(m *emu.Emu) error {
	_, err := UseFieldMove(m, FieldSurf)
	return err
}

// StrengthAhead enables Strength while facing a live boulder through the
// shared field-action executor.
func StrengthAhead(m *emu.Emu) error {
	_, err := UseFieldMove(m, FieldStrength)
	return err
}

// Flash lights a dark area through the shared field-action executor.
func Flash(m *emu.Emu) error {
	_, err := UseFieldMove(m, FieldFlash)
	return err
}
