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
type LiveMapObject struct {
	Slot int
	X    int
	Y    int
}

// LiveTopologyState is the runtime half of the routing boundary. Static map,
// warp and object-event data comes from worldmodel.MapHeaderProvider; this
// state supplies only the mutable map blocks and object presence/positions.
type LiveTopologyState struct {
	NativeMapID uint16

	WidthBlocks  int
	HeightBlocks int
	Blocks       []byte
	Traversal    TraversalMode

	LiveObjects     []LiveMapObject
	ObjectPositions map[int]MapPoint
	HiddenObjects   map[int]bool
}

// RoutingDecoder hides game-specific WRAM layout for the live map block buffer
// and object-event runtime state.
type RoutingDecoder interface {
	DecodeLiveTopology(MemoryReader) (LiveTopologyState, error)
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
