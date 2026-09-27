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
// map identity. Offset uses the same signed tile/block-relative convention as
// the existing narrow Connection type.
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
//
// This interface is intentionally topology-only. Collision decoding is added
// separately so adapters never need to invent geometry merely to make a map
// graph buildable.
type NativeMapTopologyProvider interface {
	MapIDs() []uint16
	ParseMap(mapID uint16) (NativeMapHeader, error)
}
