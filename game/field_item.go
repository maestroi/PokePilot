package game

// FieldItemSemantics describes the execution shape of one native item without
// exposing generation-specific item ids to generic skill code.
type FieldItemSemantics struct {
	SingleMoveTarget bool
	PPRestore        bool
	RepelSteps       int
}

// FieldItemPartyMon is the portable party projection needed to verify
// overworld item effects.
type FieldItemPartyMon struct {
	NativeSpeciesID uint16
	Level           uint8
	HP              uint16
	MaxHP           uint16
	Status          string
	Moves           [4]uint16
	PP              [4]uint8
}

// FieldItemState is the live semantic execution state around overworld item
// use. Concrete profiles own prompt coordinates, text engine flags, move-menu
// encodings, and repel counters.
type FieldItemState struct {
	Party            []FieldItemPartyMon
	UsePromptVisible bool
	UseSelected      bool
	MoveMenuVisible  bool
	MoveCursor       MenuCursorState
	ResultTextActive bool
	UIOpen           bool
	OverworldReady   bool
	InBattle         bool
	ChoiceVisible    bool
	RepelSteps       int
	DebugText        string
}

// FieldItemDecoder hides game-specific item-use UI and effect state.
type FieldItemDecoder interface {
	DecodeFieldItem(MemoryReader) FieldItemState
	FieldItemSemantics(nativeItemID uint16) FieldItemSemantics
	PreferredRepels() []uint16
}

type FieldItemProfile interface {
	GameProfile
	FieldItemDecoder
}
