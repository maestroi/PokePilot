// Package worldmodel defines the adapter boundary used by generic world routing.
// It contains no game-specific ROM layout knowledge.
package worldmodel

import "sync"

// TraversalMode selects movement-specific collision semantics supplied by a game adapter.
type TraversalMode uint8

const (
	TraversalLand TraversalMode = iota
	TraversalWater
)

// Warp is the portable topology needed by the world graph.
type Warp struct {
	X          uint8
	Y          uint8
	DestWarpID uint8
	DestMap    uint8
}

// Connection is a portable adjacent-map seam.
type Connection struct {
	Dir    uint8
	MapID  uint8
	Offset int8
}

// Ledge is a directed two-tile movement hop in decoded collision geometry.
type Ledge struct {
	DX, DY     int
	From, Over byte
}

// MapHeader is the subset of a game's map header that generic routing consumes.
type MapHeader struct {
	ID           uint8
	WidthBlocks  uint8
	HeightBlocks uint8
	Warps        []Warp
	Connections  []Connection
}

// GridSpec is an adapter-decoded collision grid. Slices are row-major.
type GridSpec struct {
	MapID         uint8
	Width          int
	Height         int
	Walkable       []bool
	CollisionTile  []uint8
	FieldTile      []uint8
	TilePairs      map[[2]uint8]bool
	Ledges         []Ledge
	CounterTiles   [3]uint8
}

// GridHeader is the compatibility boundary used by world.Build and
// world.BuildFromBlocks. Concrete game map-header types implement it without
// making world depend on those concrete types.
type GridHeader interface {
	WorldGridSpec(romData []byte, blocks []byte, mode TraversalMode) (GridSpec, error)
}

// ElevatorFloor is the topology-relevant part of a selectable elevator floor.
type ElevatorFloor struct {
	MapID      uint8
	DestWarpID uint8
}

// ElevatorSpec describes the selectable destinations of an elevator map.
type ElevatorSpec struct {
	Floors []ElevatorFloor
}

// MapHeaderProvider owns map enumeration, parsing, geometry, and dynamic
// elevator topology for one game/revision. The provider owns any ROM bytes it
// needs; generic world code never interprets them.
type MapHeaderProvider interface {
	MapIDs() []uint8
	ParseMap(mapID uint8) (MapHeader, error)
	Grid(mapID uint8, blocks []byte, mode TraversalMode) (GridSpec, error)
	LookupElevator(mapID uint8) (ElevatorSpec, bool)
	ElevatorFloorForDestination(elevatorMap, destinationMap uint8) (ElevatorFloor, bool)
}

// ROMProviderFactory is retained as a compatibility bridge for callers that
// still pass raw ROM bytes to world.BuildGraph. New adapters/callers should
// pass a MapHeaderProvider directly.
type ROMProviderFactory func(romData []byte) (MapHeaderProvider, bool)

var (
	factoryMu sync.RWMutex
	factories []ROMProviderFactory
)

// RegisterROMProviderFactory registers a game-owned ROM detector/provider.
// It is intended for adapter package init functions.
func RegisterROMProviderFactory(factory ROMProviderFactory) {
	if factory == nil {
		return
	}
	factoryMu.Lock()
	factories = append(factories, factory)
	factoryMu.Unlock()
}

// ProviderForROM resolves a legacy raw-ROM call through registered adapters.
func ProviderForROM(romData []byte) (MapHeaderProvider, bool) {
	factoryMu.RLock()
	copyFactories := append([]ROMProviderFactory(nil), factories...)
	factoryMu.RUnlock()
	for _, factory := range copyFactories {
		if provider, ok := factory(romData); ok && provider != nil {
			return provider, true
		}
	}
	return nil, false
}
