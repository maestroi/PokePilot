// Package worldmodel defines the adapter boundary used by generic world routing.
// It contains no game-specific ROM layout knowledge.
package worldmodel

import "sync"

// MapID is the portable native map identity used by world/routing adapters.
// Gen I uses one-byte ids; Gen II needs the full map-group/map-number pair.
type MapID = uint16

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
	DestMap    MapID
	// Inert marks a ROM warp-table entry that is ordinary traversable floor:
	// it does not fire a transition and therefore must not become a graph port.
	Inert bool
}

// Connection is a portable adjacent-map seam.
type Connection struct {
	Dir    uint8
	MapID  MapID
	Offset int8
}

func (c Connection) WorldDirection() uint8 { return c.Dir }
func (c Connection) WorldMapID() MapID     { return c.MapID }
func (c Connection) WorldOffset() int8     { return c.Offset }

// Ledge is a directed two-tile movement hop in decoded collision geometry.
type Ledge struct {
	DX, DY     int
	From, Over byte
}

// ObjectMovement is the portable movement class routing needs from a map
// object. Concrete adapters translate their native object-event encoding into
// these semantics.
type ObjectMovement uint8

const (
	ObjectMovementUnknown ObjectMovement = iota
	ObjectMovementWalk
	ObjectMovementStay
)

// InteractionRole identifies built-in service actors whose interaction is not
// ordinary NPC dialogue. Games may expose only the roles they support.
type InteractionRole string

const (
	InteractionPokemonCenterNurse InteractionRole = "pokemon_center_nurse"
	InteractionMart               InteractionRole = "mart"
	InteractionBillsPC            InteractionRole = "bills_pc"
	InteractionPlayersPC          InteractionRole = "players_pc"
	InteractionPokemonCenterPC    InteractionRole = "pokemon_center_pc"
	InteractionPrizeVendor        InteractionRole = "prize_vendor"
	InteractionCableClub          InteractionRole = "cable_club"
	InteractionVendingMachine     InteractionRole = "vending_machine"
)

// MapObject is the portable object-event projection used by generic routing.
// Native fields are opaque identifiers retained for game-owned higher-level
// adapters; routing itself interprets only Slot, coordinates, Movement and Role.
type MapObject struct {
	Slot               int
	X, Y               uint8
	Movement           ObjectMovement
	Role               InteractionRole
	NativeSpriteID     uint16
	NativeTextID       uint16
	NativeItemID       uint16
	NativeTrainerClass uint16
	NativeTrainerSet   uint16
}

// MapHeader is the subset of a game's map header that generic routing consumes.
type MapHeader struct {
	ID            MapID
	NativeTileset uint16
	WidthBlocks   uint8
	HeightBlocks  uint8
	Warps         []Warp
	Connections   []Connection
	Objects       []MapObject
}

// HeaderView lets compatibility callers pass a richer game-specific map
// header to generic routing without exposing that concrete type here.
type HeaderView interface {
	WorldMapHeader() MapHeader
}

// WorldMapHeader makes the portable header itself a HeaderView.
func (h MapHeader) WorldMapHeader() MapHeader { return h }

// GridSpec is an adapter-decoded collision grid. Slices are row-major.
type GridSpec struct {
	MapID         MapID
	Width         int
	Height        int
	Walkable      []bool
	CollisionTile []uint8
	FieldTile     []uint8
	Cuttable      []bool
	TilePairs     map[[2]uint8]bool
	Ledges        []Ledge
	CounterTiles  [3]uint8
	// Traversal is the movement mode these collision semantics describe.
	Traversal TraversalMode
}

// GridHeader is the compatibility boundary used by world.Build and
// world.BuildFromBlocks. Concrete game map-header types implement it without
// making world depend on those concrete types.
type GridHeader interface {
	WorldGridSpec(romData []byte, blocks []byte, mode TraversalMode) (GridSpec, error)
}

// ElevatorFloor is the topology-relevant part of a selectable elevator floor.
type ElevatorFloor struct {
	MapID      MapID
	DestWarpID uint8
}

// ElevatorSpec describes the selectable destinations of an elevator map.
type ElevatorSpec struct {
	PanelX uint8
	PanelY uint8
	Floors []ElevatorFloor
}

// MapHeaderProvider owns map enumeration, parsing, geometry, and dynamic
// elevator topology for one game/revision. The provider owns any ROM bytes it
// needs; generic world code never interprets them.
type MapHeaderProvider interface {
	MapIDs() []MapID
	ParseMap(mapID MapID) (MapHeader, error)
	Grid(mapID MapID, blocks []byte, mode TraversalMode) (GridSpec, error)
	LookupElevator(mapID MapID) (ElevatorSpec, bool)
	ElevatorFloorForDestination(elevatorMap, destinationMap MapID) (ElevatorFloor, bool)
}

// MapParseFailureClassifier is an optional provider capability for maps that
// are deliberately enumerated but not parseable by this adapter. Returning
// ok=true makes that omission explicit; all unclassified ParseMap failures are
// fatal to normal graph construction.
type MapParseFailureClassifier interface {
	ExpectedMapParseFailure(mapID MapID, err error) (reason string, ok bool)
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
