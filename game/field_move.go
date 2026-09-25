package game

// FieldMoveID is the portable identity of an out-of-battle move. These ids are
// deliberately independent of native move numbers, machine item ids, menu ids,
// badge bits, and generation-specific ordering.
type FieldMoveID string

const (
	FieldMoveCut       FieldMoveID = "cut"
	FieldMoveFly       FieldMoveID = "fly"
	FieldMoveSurf      FieldMoveID = "surf"
	FieldMoveStrength  FieldMoveID = "strength"
	FieldMoveFlash     FieldMoveID = "flash"
	FieldMoveWhirlpool FieldMoveID = "whirlpool"
	FieldMoveWaterfall FieldMoveID = "waterfall"
	FieldMoveHeadbutt  FieldMoveID = "headbutt"
)

// ProgressionFieldMoves is the generation-neutral semantic vocabulary shared
// by routing/planning. A concrete profile may support only a subset.
func ProgressionFieldMoves() []FieldMoveID {
	return []FieldMoveID{
		FieldMoveCut,
		FieldMoveFly,
		FieldMoveSurf,
		FieldMoveStrength,
		FieldMoveFlash,
		FieldMoveWhirlpool,
		FieldMoveWaterfall,
		FieldMoveHeadbutt,
	}
}

// FieldMoveCapability is a snapshot of one move's current prerequisites.
// MachineOwned covers the game's teachable source (HM in Gen I/II, or a TM for
// moves such as Gen II Headbutt). PartySlot is the current learned carrier, or
// -1. CompatiblePartySlots are current-party members that could legally become
// a carrier without changing party composition.
//
// Preparable is intentionally stronger than MachineOwned: the unlock/badge,
// machine, and at least one legal current-party placement must all exist.
// Usable means the game would currently permit the already-learned move.
type FieldMoveCapability struct {
	Move                 FieldMoveID
	Name                 string
	BadgeRequired        string
	BadgeOwned           bool
	MachineOwned         bool
	Learned              bool
	PartySlot            int
	CompatiblePartySlots []int
	Preparable           bool
	Usable               bool
}

// NativeFieldMove contains the concrete ids needed by the current game's
// machine-teaching implementation. They are uint16 so Gen II adapters are not
// forced through Gen I's byte-sized item/move ids.
type NativeFieldMove struct {
	MachineItemID uint16
	MoveID        uint16
}

// FieldMoveMenuState is the semantic projection of the selected party member's
// field-move submenu. Entries preserve native menu positions. Unknown/non-
// progression field actions are represented by the empty FieldMoveID so a
// profile can keep indices stable without teaching generic code their ids.
type FieldMoveMenuState struct {
	Entries []FieldMoveID
}

// FieldMoveDecoder owns badge/machine/carrier decoding and field-move submenu
// encodings for one game profile.
type FieldMoveDecoder interface {
	DecodeFieldMoveCapability(MemoryReader, []byte, FieldMoveID) (FieldMoveCapability, bool, error)
	DecodeFieldMoveMenu(MemoryReader) FieldMoveMenuState
	NativeFieldMove(FieldMoveID) (NativeFieldMove, bool)
}

// FieldMoveProfile is a game profile that exposes both field-action effects and
// the capability/menu mechanics needed to prepare and dispatch those actions.
type FieldMoveProfile interface {
	GameProfile
	FieldActionDecoder
	FieldMoveDecoder
}
