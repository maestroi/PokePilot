package skill

import "github.com/maestroi/pokepilot/red/state"

// FieldActionKind is retained for Red compatibility callers that reason about
// the five Gen-I field actions. Portable field-move identity/capability/menu
// state lives in game.FieldMoveID / game.FieldMoveCapability.
type FieldActionKind uint8

const (
	FieldActionTargeted FieldActionKind = iota
	FieldActionMode
	FieldActionTransition
)

// FieldMoveSpec is Red's compatibility view of a field move. New generic code
// must use the active game.FieldMoveProfile rather than these native Gen-I ids.
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

	// Snapshot-only Red compatibility constants. Runtime field-action decoding
	// is profile-owned; these remain for Red tests/adapters that consume a
	// red/state.Mem snapshot directly.
	fieldStrengthActiveBit = 1 << 0
	fieldSurfingState      = 2
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

// FieldMoveSpecFor is the Red compatibility lookup used by existing roster and
// route code. Gen-II-only semantic moves intentionally have no Gen-I spec.
func FieldMoveSpecFor(move FieldMove) (FieldMoveSpec, bool) {
	if int(move) >= len(fieldMoveSpecs) {
		return FieldMoveSpec{}, false
	}
	return fieldMoveSpecs[move], true
}

// ProgressionFieldMoves is the Red party/PC retention set. It intentionally
// remains the five Gen-I moves; use SemanticFieldMoves for the portable set.
func ProgressionFieldMoves() []FieldMove {
	return []FieldMove{FieldCut, FieldFly, FieldSurf, FieldStrength, FieldFlash}
}

// FieldCapability is the Red compatibility snapshot consumed by existing
// route/story code. New generic runtime code uses game.FieldMoveCapability.
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

// FieldCapabilityFor decodes the legacy Red snapshot form.
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

func FieldCapabilities(mem *state.Mem) []FieldCapability {
	moves := ProgressionFieldMoves()
	out := make([]FieldCapability, 0, len(moves))
	for _, move := range moves {
		out = append(out, FieldCapabilityFor(mem, move))
	}
	return out
}

// CanPrepareFieldMove preserves the Red snapshot API while generic execution
// now obtains the same facts from game.FieldMoveProfile.
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
