package game

// MachinePocket identifies the carried-item pocket currently shown by a
// game's inventory UI. The vocabulary is intentionally semantic: concrete
// profiles own native pocket numbers and menu states.
type MachinePocket string

const (
	MachinePocketUnknown  MachinePocket = ""
	MachinePocketItems    MachinePocket = "items"
	MachinePocketBalls    MachinePocket = "balls"
	MachinePocketKeyItems MachinePocket = "key_items"
	MachinePocketTMHM     MachinePocket = "tm_hm"
)

// MachineMenuState is the portable live state needed to navigate to an owned
// TM/HM. Position is the zero-based absolute index among owned machines in the
// current TM/HM pocket; Count excludes the CANCEL row.
type MachineMenuState struct {
	Visible  bool
	Pocket   MachinePocket
	Ready    bool
	Position int
	Count    int
}

// MachineMenuDecoder hides a game's pocket ids, cursor/scroll RAM and native
// machine ordering from reusable execution code.
type MachineMenuDecoder interface {
	DecodeMachineMenu(MemoryReader) MachineMenuState
	MachineMenuEntryIndex(MemoryReader, NativeFieldMove) (int, bool)
}

// MachineMenuProfile is a game profile that exposes semantic TM/HM menu state.
type MachineMenuProfile interface {
	GameProfile
	MachineMenuDecoder
}
