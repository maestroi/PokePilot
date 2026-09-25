package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrFieldMovePrerequisite reports that a field move cannot be used or
// prepared from the current party/progress: the required badge or HM is
// missing. That is a stable, replan-able blockage, not a controller fault.
var ErrFieldMovePrerequisite = errors.New("skill: field move prerequisite is missing")

// FieldMove identifies one progression-relevant Gen 1 out-of-battle move.
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

// FieldActionKind distinguishes moves that operate on a nearby world target
// from moves that enter a mode or transition. Strength is target-oriented
// even though the ROM implements it by enabling a temporary map-wide flag:
// callers use it in the context of the boulder directly in front of Red.
type FieldActionKind uint8

const (
	FieldActionTargeted FieldActionKind = iota
	FieldActionMode
	FieldActionTransition
)

// FieldMoveSpec is the stable definition of one field capability. HMItem,
// MoveID, and MenuID are the ROM encodings used by the generic teaching/menu
// primitives; Badge is the badge the party menu checks before dispatching it.
type FieldMoveSpec struct {
	Move   FieldMove
	Name   string
	HMItem uint8
	MoveID uint8
	MenuID uint8
	Badge  state.Badge
	Kind   FieldActionKind
}

const (
	fieldHM02Item uint8 = 0xC5
	fieldHM03Item uint8 = 0xC6
	fieldHM04Item uint8 = 0xC7
	fieldHM05Item uint8 = 0xC8

	fieldFlyMove      uint8 = 0x13
	fieldSurfMove     uint8 = 0x39
	fieldStrengthMove uint8 = 0x46
	fieldFlashMove    uint8 = 0x94

	fieldFlyMenuID      uint8 = 2
	fieldSurfMenuID     uint8 = 4
	fieldStrengthMenuID uint8 = 5
	fieldFlashMenuID    uint8 = 6

	fieldStrengthActiveBit = 1 << 0
	fieldSurfingState      = 2
	fieldActionBudget      = 3000
)

var fieldMoveSpecs = [...]FieldMoveSpec{
	{
		Move: FieldCut, Name: "CUT", HMItem: hm01Item, MoveID: cutMove,
		MenuID: cutFieldMove, Badge: state.BadgeCascade, Kind: FieldActionTargeted,
	},
	{
		Move: FieldFly, Name: "FLY", HMItem: fieldHM02Item, MoveID: fieldFlyMove,
		MenuID: fieldFlyMenuID, Badge: state.BadgeThunder, Kind: FieldActionTransition,
	},
	{
		Move: FieldSurf, Name: "SURF", HMItem: fieldHM03Item, MoveID: fieldSurfMove,
		MenuID: fieldSurfMenuID, Badge: state.BadgeSoul, Kind: FieldActionMode,
	},
	{
		Move: FieldStrength, Name: "STRENGTH", HMItem: fieldHM04Item, MoveID: fieldStrengthMove,
		MenuID: fieldStrengthMenuID, Badge: state.BadgeRainbow, Kind: FieldActionTargeted,
	},
	{
		Move: FieldFlash, Name: "FLASH", HMItem: fieldHM05Item, MoveID: fieldFlashMove,
		MenuID: fieldFlashMenuID, Badge: state.BadgeBoulder, Kind: FieldActionMode,
	},
}

// FieldMoveSpecFor returns the definition for move.
func FieldMoveSpecFor(move FieldMove) (FieldMoveSpec, bool) {
	if int(move) >= len(fieldMoveSpecs) {
		return FieldMoveSpec{}, false
	}
	return fieldMoveSpecs[move], true
}

func (m FieldMove) String() string {
	if id, ok := semanticFieldMove(m); ok {
		return strings.ToUpper(string(id))
	}
	return fmt.Sprintf("field-move(%d)", uint8(m))
}

// ProgressionFieldMoves is the set party/PC planning must treat as strategic
// capabilities. Returning a fresh slice keeps callers from mutating package
// state while still giving storage/roster code one authoritative list.
func ProgressionFieldMoves() []FieldMove {
	return []FieldMove{FieldCut, FieldFly, FieldSurf, FieldStrength, FieldFlash}
}

// SemanticFieldMoves is the generation-neutral field-move vocabulary exposed
// by the generic lane. Red's roster planner intentionally keeps using
// ProgressionFieldMoves above; a Gen II adapter can additionally implement
// Whirlpool, Waterfall, and Headbutt without changing generic callers.
func SemanticFieldMoves() []FieldMove {
	return []FieldMove{
		FieldCut, FieldFly, FieldSurf, FieldStrength, FieldFlash,
		FieldWhirlpool, FieldWaterfall, FieldHeadbutt,
	}
}

// FieldCapability is a snapshot of one capability's prerequisites. Usable is
// intentionally strict: owning an HM never makes a field move usable. The
// required badge must be owned and a current party member must already know
// the move. HMOwned is reported separately so callers may decide to prepare
// the capability through generic TM/HM teaching.
type FieldCapability struct {
	Move       FieldMove
	Name       string
	Badge      state.Badge
	BadgeOwned bool
	HMOwned    bool
	Learned    bool
	PartySlot  int
	Usable     bool
}

// FieldCapabilityFor decodes one field capability from a RAM snapshot.
func FieldCapabilityFor(mem *state.Mem, move FieldMove) FieldCapability {
	spec, ok := FieldMoveSpecFor(move)
	if !ok {
		return FieldCapability{Move: move, PartySlot: -1}
	}
	slot := partyMoveSlot(mem, spec.MoveID)
	_, qty := bagEntry(mem, spec.HMItem)
	badge := state.DecodeProgress(mem).Has(spec.Badge)
	learned := slot >= 0
	return FieldCapability{
		Move:       move,
		Name:       spec.Name,
		Badge:      spec.Badge,
		BadgeOwned: badge,
		HMOwned:    qty > 0,
		Learned:    learned,
		PartySlot:  slot,
		Usable:     badge && learned,
	}
}

// FieldCapabilities returns all progression field capabilities in stable
// order. This is the shared query surface for routing and future party/PC
// retention: callers do not need to know HM item IDs or badge mappings.
func FieldCapabilities(mem *state.Mem) []FieldCapability {
	moves := ProgressionFieldMoves()
	out := make([]FieldCapability, 0, len(moves))
	for _, move := range moves {
		out = append(out, FieldCapabilityFor(mem, move))
	}
	return out
}

// CanPrepareFieldMove reports whether a currently unusable capability can be
// made usable without changing the party composition: the badge and HM must
// be present and the generic TM/HM policy must find a compatible legal party
// slot. This is deliberately stronger than "HM owned" and is safe for Travel
// to use before deciding a blocked route is recoverable.
func CanPrepareFieldMove(romData []byte, mem *state.Mem, move FieldMove) bool {
	cap := FieldCapabilityFor(mem, move)
	if cap.Usable {
		return true
	}
	if !cap.BadgeOwned || !cap.HMOwned {
		return false
	}
	spec, ok := FieldMoveSpecFor(move)
	if !ok {
		return false
	}
	decision, err := DecideTMHM(romData, state.DecodeParty(mem), spec.HMItem, true)
	return err == nil && decision.PartySlot >= 0
}

// EnsureFieldMove makes move usable by the current party. It reuses the
// generic TM/HM teaching path and verifies the learned move from party RAM.
// Existing learned moves are idempotent; an HM in the bag by itself is never
// reported as success.
func EnsureFieldMove(m *emu.Emu, move FieldMove) (int, error) {
	profile, err := fieldMoveProfileFor(m)
	if err != nil {
		return -1, err
	}
	capability, err := fieldMoveCapabilityWithProfile(profile, m, m.ROM(), move)
	if err != nil {
		return -1, err
	}
	name := capability.Name
	if name == "" {
		name = move.String()
	}
	if !capability.BadgeOwned {
		if capability.BadgeRequired == "" {
			return -1, fmt.Errorf("%w: %s is not unlocked for field use", ErrFieldMovePrerequisite, name)
		}
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

	id, _ := semanticFieldMove(move)
	native, ok := profile.NativeFieldMove(id)
	if !ok {
		return -1, fmt.Errorf("%w: %s has no native teaching mapping", ErrFieldMovePrerequisite, name)
	}
	if native.MachineItemID == 0 || native.MachineItemID > 0xff || native.MoveID == 0 || native.MoveID > 0xff {
		return -1, fmt.Errorf("skill: teach %s: native machine/item ids %#04x/%#04x exceed current teaching executor range",
			name, native.MachineItemID, native.MoveID)
	}

	// TM/HM button sequencing is still shared with the existing move-learning
	// executor. The field lane no longer decodes Red badge/HM/carrier state or
	// native ids itself; a later move-learning slice can replace this byte-sized
	// executor without changing the field-move contract.
	result, err := TeachTMHM(m, uint8(native.MachineItemID), true)
	if err != nil {
		return -1, fmt.Errorf("skill: teach %s: %w", name, err)
	}
	if uint16(result.Decision.Machine.Move) != native.MoveID {
		return -1, fmt.Errorf("skill: teach %s: machine %#04x mapped to move %#04x, want %#04x",
			name, native.MachineItemID, result.Decision.Machine.Move, native.MoveID)
	}

	capability, err = fieldMoveCapabilityWithProfile(profile, m, m.ROM(), move)
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
// blindly; MenuUp distinguishes a cursor menu from ordinary dialogue.
func settleFieldAction(m *emu.Emu, mem *state.Mem, spec FieldMoveSpec, decoder game.FieldActionDecoder) error {
	for spent := 0; spent < fieldActionBudget; spent += 10 {
		state.Snapshot(m, mem)
		runtime := decoder.DecodeFieldAction(m)
		if fieldActionCompleteState(runtime, spec) {
			return nil
		}
		if mem.U8(sym.FontLoaded) != 0 && !state.MenuUp(mem) {
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

	var mem state.Mem
	slot, err := EnsureFieldMove(m, move)
	if err != nil {
		return FieldActionResult{}, err
	}
	state.Snapshot(m, &mem)

	if err := openStartMenuEntry(m, startMenuPokemon); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: open POKEMON: %w", spec.Name, err)
	}
	if _, err := m.StepUntil(1000, normalPartyMenuUp); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: party menu did not appear", spec.Name)
	}
	if err := selectFieldMoveUser(m, slot); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: select party slot %d: %w", spec.Name, slot, err)
	}

	idx := fieldMoveMenuIndex(m, move)
	if idx < 0 {
		return FieldActionResult{}, fmt.Errorf("skill: %s: party slot %d knows the move but the semantic field-move entry is absent", spec.Name, slot)
	}
	if err := SelectMenuItem(m, idx); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: select field move: %w", spec.Name, err)
	}
	m.StepFrames(30)
	if err := settleFieldAction(m, &mem, spec, decoder); err != nil {
		state.Snapshot(m, &mem)
		runtime = decoder.DecodeFieldAction(m)
		closeErr := closeToOverworld(m)
		if closeErr != nil {
			return FieldActionResult{}, fmt.Errorf("skill: %s did not complete: %v; succeeded=%v surfing=%v strength=%v lit=%v screen=%q; cleanup: %v",
				spec.Name, err, runtime.ActionSucceeded, runtime.Surfing, runtime.StrengthActive, runtime.Lit, state.ScreenText(&mem), closeErr)
		}
		return FieldActionResult{}, fmt.Errorf("skill: %s did not complete: %v; succeeded=%v surfing=%v strength=%v lit=%v screen=%q",
			spec.Name, err, runtime.ActionSucceeded, runtime.Surfing, runtime.StrengthActive, runtime.Lit, state.ScreenText(&mem))
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
