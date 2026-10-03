package worldmodel

// NativeWarp is the topology of one map warp keyed by the cartridge's native
// map identity. DestWarpID is zero-based so generic graph code can index the
// destination map's warp list directly.
type NativeWarp struct {
	X          uint8
	Y          uint8
	DestWarpID uint8
	DestMap    uint16
	Inert      bool
}

// NativeConnection is an adjacent-map seam keyed by the cartridge's native
// map identity. Offset is the decomp's own value: signed blocks, positive when
// the neighbour's origin lies further along the shared edge than this map's.
// The narrow Gen-I Connection type carries a pre-negated tile alignment instead.
type NativeConnection struct {
	Dir    uint8
	MapID  uint16
	Offset int8
}

// NativeMapHeader is the map-level topology needed before collision decoding.
// It deliberately uses uint16 map identities: Gen II identifies a map by
// (group, number), and Gold/Silver have more than 255 maps.
type NativeMapHeader struct {
	ID           uint16
	WidthBlocks  uint8
	HeightBlocks uint8
	Warps        []NativeWarp
	Connections  []NativeConnection
}

// NativeMapTopologyProvider is the wide map-identity boundary for cartridges
// whose native map namespace does not fit in the historical Gen-I uint8 graph.
type NativeMapTopologyProvider interface {
	MapIDs() []uint16
	ParseMap(mapID uint16) (NativeMapHeader, error)
}

// NativeDirectionMask uses bits 0..3 for down, up, left and right. Adapters
// may attach a mask to one collision byte without leaking the native meaning
// of that byte into generic routing.
type NativeDirectionMask uint8

const (
	NativeBlockDown NativeDirectionMask = 1 << iota
	NativeBlockUp
	NativeBlockLeft
	NativeBlockRight
)

// NativeJump describes a directed two-tile hop triggered while standing on a
// collision tile. DirectionMask uses the same down/up/left/right bit layout.
type NativeJump struct {
	Collision     uint8
	DirectionMask NativeDirectionMask
}

// NativeObstacle names the field move that clears a blocking tile or object
// ("" means no obstacle). Which collision byte or sprite is an obstacle is
// adapter-owned; generic routing only sees the semantic kind.
type NativeObstacle string

const (
	ObstacleCutTree   NativeObstacle = "cut"
	ObstacleSmashRock NativeObstacle = "rock_smash"
	ObstacleWhirlpool NativeObstacle = "whirlpool"
)

// NativeGridSpec is the wide-id counterpart to GridSpec. Collision decoding is
// adapter-owned; generic native routing receives only tile-level semantics.
type NativeGridSpec struct {
	MapID         uint16
	Width         int
	Height        int
	Walkable      []bool
	CollisionTile []uint8
	Obstacles     []NativeObstacle // per cell; nil when the map has none
	// WarpTrigger says, per cell, whether standing there fires a warp event
	// from the map's warp table. Nil means the adapter does not say and every
	// warp is assumed to trigger. A warp table entry on a non-triggering tile
	// is only a landing spot for warps from elsewhere.
	WarpTrigger []bool
	Blocked     map[uint8]NativeDirectionMask
	Jumps       []NativeJump
	Traversal   TraversalMode
}

// NativeGridProvider is an optional extension of NativeMapTopologyProvider for
// games whose map ids do not fit the legacy uint8 grid boundary.
type NativeGridProvider interface {
	NativeMapTopologyProvider
	Grid(mapID uint16, blocks []byte, mode TraversalMode) (NativeGridSpec, error)
}
