package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

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
	fieldBoulderPictureID  = 0x3F
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
	if spec, ok := FieldMoveSpecFor(m); ok {
		return spec.Name
	}
	return fmt.Sprintf("field-move(%d)", uint8(m))
}

// ProgressionFieldMoves is the set party/PC planning must treat as strategic
// capabilities. Returning a fresh slice keeps callers from mutating package
// state while still giving storage/roster code one authoritative list.
func ProgressionFieldMoves() []FieldMove {
	return []FieldMove{FieldCut, FieldFly, FieldSurf, FieldStrength, FieldFlash}
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
	spec, ok := FieldMoveSpecFor(move)
	if !ok {
		return -1, fmt.Errorf("skill: field move %d is unknown", move)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.DecodeProgress(&mem).Has(spec.Badge) {
		return -1, fmt.Errorf("skill: %s requires the %s Badge", spec.Name, spec.Badge)
	}
	if slot := partyMoveSlot(&mem, spec.MoveID); slot >= 0 {
		return slot, nil
	}
	if _, qty := bagEntry(&mem, spec.HMItem); qty == 0 {
		return -1, fmt.Errorf("skill: %s requires HM item %#02x in the bag", spec.Name, spec.HMItem)
	}

	result, err := TeachTMHM(m, spec.HMItem, true)
	if err != nil {
		return -1, fmt.Errorf("skill: teach %s: %w", spec.Name, err)
	}
	if result.Decision.Machine.Move != spec.MoveID {
		return -1, fmt.Errorf("skill: teach %s: HM %#02x mapped to move %d, want %d", spec.Name, spec.HMItem, result.Decision.Machine.Move, spec.MoveID)
	}

	state.Snapshot(m, &mem)
	slot := partyMoveSlot(&mem, spec.MoveID)
	if slot < 0 {
		return -1, fmt.Errorf("skill: teach %s: move %d was not verified in party RAM", spec.Name, spec.MoveID)
	}
	return slot, nil
}

func fieldMoveMenuIndex(m *emu.Emu, menuID uint8) int {
	// A Pokemon can know at most four moves, so wFieldMoves can expose at
	// most four entries before its zero terminator.
	for i := 0; i < 4; i++ {
		id := m.Peek8(sym.FieldMoves + uint16(i))
		if id == 0 {
			break
		}
		if id == menuID {
			return i
		}
	}
	return -1
}

func frontCoordinates(mem *state.Mem) (int, int, bool) {
	p := state.DecodePlayer(mem)
	x, y := int(p.X), int(p.Y)
	switch p.Facing {
	case state.FacingUp:
		y--
	case state.FacingDown:
		y++
	case state.FacingLeft:
		x--
	case state.FacingRight:
		x++
	default:
		return 0, 0, false
	}
	return x, y, true
}

func boulderAhead(mem *state.Mem) bool {
	x, y, ok := frontCoordinates(mem)
	if !ok {
		return false
	}
	for _, sprite := range state.DecodeSprites(mem) {
		if sprite.X == x && sprite.Y == y && sprite.PictureID == fieldBoulderPictureID {
			return true
		}
	}
	return false
}

func validateFieldActionContext(mem *state.Mem, spec FieldMoveSpec) error {
	switch spec.Move {
	case FieldCut:
		if tile := mem.U8(sym.TileInFrontOfPlayer); !cuttableFrontTile(tile) {
			return fmt.Errorf("tile in front is %#02x, not a Cut tree", tile)
		}
	case FieldStrength:
		if !boulderAhead(mem) {
			return fmt.Errorf("no boulder is directly in front of the player")
		}
	case FieldSurf:
		if mem.U8(sym.WalkBikeSurfState) == fieldSurfingState {
			return fmt.Errorf("player is already surfing")
		}
	case FieldFlash:
		if mem.U8(sym.MapPalOffset) == 0 {
			return fmt.Errorf("current area is already lit")
		}
	case FieldFly:
		return fmt.Errorf("Fly requires a destination selection; use a destination-aware transition")
	}
	return nil
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

func fieldActionComplete(mem *state.Mem, spec FieldMoveSpec) bool {
	if !state.Controllable(mem) {
		return false
	}
	switch spec.Move {
	case FieldCut:
		return mem.U8(sym.ActionResult) == 1
	case FieldSurf:
		return mem.U8(sym.ActionResult) == 1 && mem.U8(sym.WalkBikeSurfState) == fieldSurfingState
	case FieldStrength:
		return mem.U8(sym.StatusFlags1)&fieldStrengthActiveBit != 0
	case FieldFlash:
		return mem.U8(sym.MapPalOffset) == 0
	default:
		return false
	}
}

// UseFieldMove executes one supported field move through the real START ->
// POKEMON -> field-move menu. It auto-teaches the HM only when the badge,
// owned HM, and generic compatibility policy make that legal, then verifies
// the ROM-side effect from live state rather than trusting timing/dialogue.
// Fly is represented by the same capability abstraction but needs a caller-
// supplied destination, so destination-free execution rejects it explicitly.
func UseFieldMove(m *emu.Emu, move FieldMove) (FieldActionResult, error) {
	spec, ok := FieldMoveSpecFor(move)
	if !ok {
		return FieldActionResult{}, fmt.Errorf("skill: field move %d is unknown", move)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return FieldActionResult{}, fmt.Errorf("skill: %s: player is not controllable", spec.Name)
	}
	if err := validateFieldActionContext(&mem, spec); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: invalid context: %w", spec.Name, err)
	}

	slot, err := EnsureFieldMove(m, move)
	if err != nil {
		return FieldActionResult{}, err
	}
	state.Snapshot(m, &mem)

	wantMax, itemIndex := startMenuShape(&mem)
	if err := openStartMenuEntry(m, itemIndex-1, wantMax); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: open POKEMON: %w", spec.Name, err)
	}
	if _, err := m.StepUntil(1000, normalPartyMenuUp); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: party menu did not appear", spec.Name)
	}
	if err := selectFieldMoveUser(m, slot); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: select party slot %d: %w", spec.Name, slot, err)
	}

	idx := fieldMoveMenuIndex(m, spec.MenuID)
	if idx < 0 {
		return FieldActionResult{}, fmt.Errorf("skill: %s: party slot %d knows move %d but menu id %d is absent from wFieldMoves", spec.Name, slot, spec.MoveID, spec.MenuID)
	}
	if err := SelectMenuItem(m, idx); err != nil {
		return FieldActionResult{}, fmt.Errorf("skill: %s: select field move: %w", spec.Name, err)
	}
	m.StepFrames(30)
	if _, err := m.StepUntil(fieldActionBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return fieldActionComplete(&mem, spec)
	}); err != nil {
		state.Snapshot(m, &mem)
		return FieldActionResult{}, fmt.Errorf("skill: %s did not complete: action=%d surfing=%d strength=%#02x palette=%d screen=%q",
			spec.Name, mem.U8(sym.ActionResult), mem.U8(sym.WalkBikeSurfState), mem.U8(sym.StatusFlags1), mem.U8(sym.MapPalOffset), state.ScreenText(&mem))
	}

	state.Snapshot(m, &mem)
	return FieldActionResult{
		Move:           move,
		PartySlot:      slot,
		ActionResult:   mem.U8(sym.ActionResult),
		Surfing:        mem.U8(sym.WalkBikeSurfState) == fieldSurfingState,
		StrengthActive: mem.U8(sym.StatusFlags1)&fieldStrengthActiveBit != 0,
		Lit:            mem.U8(sym.MapPalOffset) == 0,
	}, nil
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
