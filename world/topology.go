package world

// Warp is a game-agnostic map transition port. DestMap may use 0xff as an
// adapter-supplied unresolved-return marker; BuildGraph resolves it from the
// surrounding topology without understanding any game's ROM format.
type Warp struct {
	X          uint8
	Y          uint8
	DestWarpID uint8
	DestMap    uint8
}

// Connection links a map to an adjacent map. Dir uses the world edge direction
// convention: 0=north, 1=south, 2=west, 3=east.
type Connection struct {
	Dir    uint8
	MapID  uint8
	Offset int8
}

// ElevatorFloor describes a semantic elevator edge and the destination warp
// index needed to identify its arrival port. Menu/controller mechanics remain
// adapter-owned.
type ElevatorFloor struct {
	MapID      uint8
	DestWarpID uint8
}

// MapTopology is the complete static routing input for one map. A game adapter
// parses native ROM data into this shape; world owns only the routing semantics.
type MapTopology struct {
	ID          uint8
	Width       int
	Height      int
	Warps       []Warp
	Connections []Connection
	Grid        *Grid
	Elevator    []ElevatorFloor
}
