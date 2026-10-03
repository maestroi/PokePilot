package game

// TraversalMode is the live movement mode observed by a game profile. Static
// collision interpretation remains owned by the routing adapter.
type TraversalMode uint8

const (
	TraversalLand TraversalMode = iota
	TraversalWater
)

// MapPoint is a portable tile coordinate used by routing/object observations.
type MapPoint struct {
	X int
	Y int
}

// LiveMapObject is one currently loaded map object as observed from runtime
// memory. Slot is the profile's 1-based object-event identity for the map.
//
// Clearable, when set, names the field move that removes the object (a
// smashable rock). Profiles own which sprite or movement type earns it.
type LiveMapObject struct {
	Slot      int
	X         int
	Y         int
	Clearable FieldMoveID
}

// MapShellPhase is where a cartridge's overworld map shell is in its own entry
// sequence: the map identity can be current while the block buffer is still
// being rebuilt, which makes the block bytes a half-written buffer rather than
// map data. It is deliberately the cartridge's own phase vocabulary instead of
// a boolean, so a reader can distinguish "still entering" from "settled".
type MapShellPhase uint8

const (
	// MapShellUnknown means the profile does not model the entry sequence.
	MapShellUnknown MapShellPhase = iota
	// MapShellStarting is the phase before the new map's shell is installed.
	MapShellStarting
	// MapShellEntering is the phase that rebuilds the map shell, including the
	// block buffer and its border/connection strips.
	MapShellEntering
	// MapShellSettled is the phase in which the block buffer describes the
	// current map and may be decoded into collision.
	MapShellSettled
	// MapShellDone means the overworld loop has left the map.
	MapShellDone
)

// LiveTopologyState is the runtime half of the routing boundary. Static map,
// warp and object-event data comes from worldmodel.MapHeaderProvider; this
// state supplies only the mutable map blocks and object presence/positions.
type LiveTopologyState struct {
	NativeMapID uint16

	WidthBlocks  int
	HeightBlocks int
	Blocks       []byte
	Traversal    TraversalMode

	MapShellPhase MapShellPhase
	// BlocksSettled is the profile's positive assertion that Blocks is
	// completed map data for NativeMapID. Reading collision out of a buffer
	// that is still being rebuilt answers a question about a map that does not
	// exist yet, so consumers must wait for this rather than treating the
	// bytes as geometry.
	BlocksSettled bool

	LiveObjects     []LiveMapObject
	ObjectPositions map[int]MapPoint
	HiddenObjects   map[int]bool
}

// RoutingDecoder hides game-specific WRAM layout for the live map block buffer
// and object-event runtime state.
type RoutingDecoder interface {
	DecodeLiveTopology(MemoryReader) (LiveTopologyState, error)
}

// MapTopologyProvider exposes the static map-level adjacency in the game's
// native map-id width. The returned map uses uint16 keys and values so a
// cartridge's native identity is never narrowed: Gen I fits in the low byte,
// while Gen II's (group, number) namespace needs the full 16 bits. Generic
// routing consumes only this adjacency and lets the owning adapter decide how
// to build it, so a wide-id game does not have to fit the historical uint8
// world graph.
type MapTopologyProvider interface {
	MapAdjacency(romData []byte) (map[uint16][]uint16, error)
}

// ElevatorTransition is the portable postcondition for a profile-owned
// elevator transition. Generic routing owns the requested destination and door
// coordinates; the profile owns how a cartridge represents the mutable live
// warp table that makes those doors lead there.
type ElevatorTransition struct {
	SourceMapID      uint16
	DestinationMapID uint16
	DestinationWarp  uint8
	Doors            []MapPoint
}

// ElevatorTransitionDecoder verifies whether the live elevator doors are
// already armed for a requested destination without exposing cartridge RAM
// layout to reusable routing.
type ElevatorTransitionDecoder interface {
	ElevatorTransitionReady(MemoryReader, ElevatorTransition) bool
}
