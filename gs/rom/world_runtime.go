package rom

import (
	"crypto/sha1"
	"fmt"
	"sync"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/worldmodel"
)

// runtimeWorldProvider is the routing topology the Gold/Silver profile uses:
// the hand-verified first-badge slice wherever it covers a map (its warps,
// tilesets and elevators carry qualified fixes), plus the post-League world
// read from the cartridge itself: ROM headers for warps/connections and the
// ROM Tilesets table for collision.
//
// Johto maps outside the slice stay excluded. The Johto story never needed
// them, and admitting them lets fewest-transition routing detour through
// unverified trainer routes (Route 45 from Blackthorn during a League reset).
type runtimeWorldProvider struct {
	rom      []byte
	tables   Tables
	tilesets int
	ids      []uint16
	slice    *firstBadgeWorldProvider
}

// Locating the ROM tables scans the image (~100ms); routing asks for the
// provider on every leg, so the result is cached per ROM image.
// ponytail: unbounded, but a process holds one or two cartridges.
var runtimeWorldCache sync.Map // [sha1.Size]byte -> runtimeWorldResult

type runtimeWorldResult struct {
	provider worldmodel.NativeGridProvider
	err      error
}

// NewRuntimeWorldProvider fails closed if the ROM's map or tileset tables
// cannot be located; callers then keep the slice alone.
func NewRuntimeWorldProvider(romData []byte) (worldmodel.NativeGridProvider, error) {
	key := sha1.Sum(romData)
	if cached, ok := runtimeWorldCache.Load(key); ok {
		r := cached.(runtimeWorldResult)
		return r.provider, r.err
	}
	provider, err := newRuntimeWorldProvider(romData)
	runtimeWorldCache.Store(key, runtimeWorldResult{provider, err})
	return provider, err
}

func newRuntimeWorldProvider(romData []byte) (worldmodel.NativeGridProvider, error) {
	full, err := NewWorldProvider(romData)
	if err != nil {
		return nil, err
	}
	tilesets, err := locateTilesets(romData)
	if err != nil {
		return nil, err
	}
	rp := full.(*romWorldProvider)
	slice, ok := NewFirstBadgeWorldProvider(romData).(*firstBadgeWorldProvider)
	if !ok {
		return nil, fmt.Errorf("gs/rom: unexpected first-badge provider type")
	}
	out := &runtimeWorldProvider{rom: romData, tables: rp.tables, tilesets: tilesets, slice: slice}
	for _, id := range rp.ids {
		info, _ := gsdata.Map(id)
		h, err := parseMapAt(romData, rp.tables, id)
		if err != nil {
			return nil, err
		}
		if out.inSlice(id) || postLeagueMap(info.Name, h.Landmark) {
			out.ids = append(out.ids, id)
		}
	}
	return out, nil
}

// Landmarks (pret constants/landmark_constants.asm) of the post-League world.
const (
	landmarkSilverCave = 0x2d
	landmarkKantoFirst = 0x2e // PALLET_TOWN
	landmarkKantoEnd   = 0x57 // VICTORY_ROAD: Johto's side again
	landmarkRoute28    = 0x5d
	landmarkFastShip   = 0x5e
)

// postLeagueJohtoTerminals are the Johto ends of the ferry and Magnet Train.
var postLeagueJohtoTerminals = map[string]bool{
	"OLIVINE_PORT_PASSAGE":           true,
	"OLIVINE_PORT":                   true,
	"GOLDENROD_MAGNET_TRAIN_STATION": true,
}

func postLeagueMap(name string, landmark uint8) bool {
	return postLeagueJohtoTerminals[name] ||
		(landmark >= landmarkKantoFirst && landmark < landmarkKantoEnd) ||
		landmark == landmarkSilverCave || landmark == landmarkRoute28 || landmark == landmarkFastShip
}

func (p *runtimeWorldProvider) MapIDs() []uint16 { return append([]uint16(nil), p.ids...) }

// PostLeagueMap reports whether mapID is in the post-League world the
// runtime routing admits from the cartridge (Kanto, ports, Fast Ship, Magnet
// Train, Route 28, Mt. Silver). Controllers that own interruptions along a
// route use it so they accept exactly the maps routing can enter.
func PostLeagueMap(romData []byte, mapID uint16) bool {
	provider, err := NewRuntimeWorldProvider(romData)
	if err != nil {
		return false
	}
	p := provider.(*runtimeWorldProvider)
	if p.inSlice(mapID) {
		return false
	}
	for _, id := range p.ids {
		if id == mapID {
			return true
		}
	}
	return false
}

func (p *runtimeWorldProvider) inSlice(mapID uint16) bool {
	_, ok := p.slice.specs[mapID]
	return ok
}

// ParseMap keeps a slice map's hand-verified warps and connections, adding
// only the ROM exits the slice omitted because their destination lay outside
// it (Victory Road Gate's west door to Route 28, Olivine's port passage).
// Edges between slice maps are never changed.
func (p *runtimeWorldProvider) ParseMap(mapID uint16) (worldmodel.NativeMapHeader, error) {
	rom, err := parseMapAt(p.rom, p.tables, mapID)
	if err != nil {
		return worldmodel.NativeMapHeader{}, err
	}
	full := rom.NativeHeader()
	if !p.inSlice(mapID) {
		return full, nil
	}
	out, err := p.slice.ParseMap(mapID)
	if err != nil {
		return out, err
	}
	hasWarp := map[[2]uint8]bool{}
	for _, w := range out.Warps {
		hasWarp[[2]uint8{w.X, w.Y}] = true
	}
	for _, w := range full.Warps {
		if !hasWarp[[2]uint8{w.X, w.Y}] && !p.inSlice(w.DestMap) {
			out.Warps = append(out.Warps, w)
		}
	}
	hasDir := map[uint8]bool{}
	for _, c := range out.Connections {
		hasDir[c.Dir] = true
	}
	for _, c := range full.Connections {
		if !hasDir[c.Dir] && !p.inSlice(c.MapID) {
			out.Connections = append(out.Connections, c)
		}
	}
	return out, nil
}

func (p *runtimeWorldProvider) Grid(mapID uint16, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.NativeGridSpec, error) {
	if p.inSlice(mapID) {
		return p.slice.Grid(mapID, blocks, mode)
	}
	info, ok := gsdata.Map(mapID)
	if !ok {
		return worldmodel.NativeGridSpec{}, fmt.Errorf("gs/rom: generated map %#04x is missing", mapID)
	}
	h, err := parseMapAt(p.rom, p.tables, mapID)
	if err != nil {
		return worldmodel.NativeGridSpec{}, err
	}
	table, err := tilesetCollisionAt(p.rom, p.tilesets, h.Tileset)
	if err != nil {
		return worldmodel.NativeGridSpec{}, fmt.Errorf("gs/rom: %s: %w", info.Name, err)
	}
	return buildGen2Grid(p.rom, mapID, info, info.Name, h.Tileset, table, blocks, mode)
}

func (p *runtimeWorldProvider) LookupNativeElevator(mapID uint16) (worldmodel.NativeElevatorSpec, bool) {
	return p.slice.LookupNativeElevator(mapID)
}
