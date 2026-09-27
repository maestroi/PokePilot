// Package rom owns Gold/Silver static world facts that are not part of the
// portable semantic catalog.
package rom

import (
	"fmt"
	"sort"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/worldmodel"
)

const (
	dirNorth uint8 = iota
	dirSouth
	dirWest
	dirEast
)

type warpSpec struct {
	x, y     uint8
	dest     string
	destWarp uint8 // decomp warp ids are 1-based
}

type connectionSpec struct {
	dir    uint8
	dest   string
	offset int8
}

type mapSpec struct {
	name        string
	warps       []warpSpec
	connections []connectionSpec
}

// firstBadgeTopology is the verified fresh-save corridor needed to leave the
// player's room, run Elm's opening errand, reach Violet, enter Sprout Tower,
// and enter Falkner's gym.
//
// Facts are projected from pret/pokegold at gs/data.SourceRevision. Keeping the
// names here (rather than raw group/number ids) makes the generated gs/data
// catalog the single source of native map identity.
//
// This deliberately is not the full #973 parser yet. Unsupported destinations
// may appear in a source map's warp/connection table, but BuildNativeGraph
// drops an edge unless the destination is also enumerated by MapIDs.
var firstBadgeTopology = []mapSpec{
	{
		name: "PLAYERS_HOUSE_2F",
		warps: []warpSpec{
			{x: 7, y: 0, dest: "PLAYERS_HOUSE_1F", destWarp: 3},
		},
	},
	{
		name: "PLAYERS_HOUSE_1F",
		warps: []warpSpec{
			{x: 6, y: 7, dest: "NEW_BARK_TOWN", destWarp: 2},
			{x: 7, y: 7, dest: "NEW_BARK_TOWN", destWarp: 2},
			{x: 9, y: 0, dest: "PLAYERS_HOUSE_2F", destWarp: 1},
		},
	},
	{
		name: "NEW_BARK_TOWN",
		warps: []warpSpec{
			{x: 6, y: 3, dest: "ELMS_LAB", destWarp: 1},
			{x: 13, y: 5, dest: "PLAYERS_HOUSE_1F", destWarp: 1},
			{x: 3, y: 11, dest: "PLAYERS_NEIGHBORS_HOUSE", destWarp: 1},
			{x: 11, y: 13, dest: "ELMS_HOUSE", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirWest, dest: "ROUTE_29", offset: 0},
			{dir: dirEast, dest: "ROUTE_27", offset: 0},
		},
	},
	{
		name: "ELMS_LAB",
		warps: []warpSpec{
			{x: 4, y: 11, dest: "NEW_BARK_TOWN", destWarp: 1},
			{x: 5, y: 11, dest: "NEW_BARK_TOWN", destWarp: 1},
		},
	},
	{
		name: "ROUTE_29",
		warps: []warpSpec{
			{x: 27, y: 1, dest: "ROUTE_29_ROUTE_46_GATE", destWarp: 3},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_46", offset: 10},
			{dir: dirWest, dest: "CHERRYGROVE_CITY", offset: 0},
			{dir: dirEast, dest: "NEW_BARK_TOWN", offset: 0},
		},
	},
	{
		name: "CHERRYGROVE_CITY",
		warps: []warpSpec{
			{x: 23, y: 3, dest: "CHERRYGROVE_MART", destWarp: 2},
			{x: 29, y: 3, dest: "CHERRYGROVE_POKECENTER_1F", destWarp: 1},
			{x: 17, y: 7, dest: "CHERRYGROVE_GYM_SPEECH_HOUSE", destWarp: 1},
			{x: 25, y: 9, dest: "GUIDE_GENTS_HOUSE", destWarp: 1},
			{x: 31, y: 11, dest: "CHERRYGROVE_EVOLUTION_SPEECH_HOUSE", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_30", offset: 5},
			{dir: dirEast, dest: "ROUTE_29", offset: 0},
		},
	},
	{
		name: "CHERRYGROVE_MART",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "CHERRYGROVE_CITY", destWarp: 1},
			{x: 3, y: 7, dest: "CHERRYGROVE_CITY", destWarp: 1},
		},
	},
	{
		name: "CHERRYGROVE_POKECENTER_1F",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "CHERRYGROVE_CITY", destWarp: 2},
			{x: 4, y: 7, dest: "CHERRYGROVE_CITY", destWarp: 2},
			{x: 0, y: 7, dest: "POKECENTER_2F", destWarp: 1},
		},
	},
	{
		name: "ROUTE_30",
		warps: []warpSpec{
			{x: 7, y: 39, dest: "ROUTE_30_BERRY_HOUSE", destWarp: 1},
			{x: 17, y: 5, dest: "MR_POKEMONS_HOUSE", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirNorth, dest: "ROUTE_31", offset: -10},
			{dir: dirSouth, dest: "CHERRYGROVE_CITY", offset: -5},
		},
	},
	{
		name: "ROUTE_30_BERRY_HOUSE",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "ROUTE_30", destWarp: 1},
			{x: 3, y: 7, dest: "ROUTE_30", destWarp: 1},
		},
	},
	{
		name: "MR_POKEMONS_HOUSE",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "ROUTE_30", destWarp: 2},
			{x: 3, y: 7, dest: "ROUTE_30", destWarp: 2},
		},
	},
	{
		name: "ROUTE_31",
		warps: []warpSpec{
			{x: 4, y: 6, dest: "ROUTE_31_VIOLET_GATE", destWarp: 3},
			{x: 4, y: 7, dest: "ROUTE_31_VIOLET_GATE", destWarp: 4},
			{x: 34, y: 5, dest: "DARK_CAVE_VIOLET_ENTRANCE", destWarp: 1},
		},
		connections: []connectionSpec{
			{dir: dirSouth, dest: "ROUTE_30", offset: 10},
			{dir: dirWest, dest: "VIOLET_CITY", offset: -9},
		},
	},
	{
		name: "ROUTE_31_VIOLET_GATE",
		warps: []warpSpec{
			{x: 0, y: 4, dest: "VIOLET_CITY", destWarp: 8},
			{x: 0, y: 5, dest: "VIOLET_CITY", destWarp: 9},
			{x: 9, y: 4, dest: "ROUTE_31", destWarp: 1},
			{x: 9, y: 5, dest: "ROUTE_31", destWarp: 2},
		},
	},
	{
		name: "VIOLET_CITY",
		warps: []warpSpec{
			{x: 9, y: 17, dest: "VIOLET_MART", destWarp: 2},
			{x: 18, y: 17, dest: "VIOLET_GYM", destWarp: 1},
			{x: 30, y: 17, dest: "EARLS_POKEMON_ACADEMY", destWarp: 1},
			{x: 3, y: 15, dest: "VIOLET_NICKNAME_SPEECH_HOUSE", destWarp: 1},
			{x: 31, y: 25, dest: "VIOLET_POKECENTER_1F", destWarp: 1},
			{x: 21, y: 29, dest: "VIOLET_KYLES_HOUSE", destWarp: 1},
			{x: 23, y: 5, dest: "SPROUT_TOWER_1F", destWarp: 1},
			{x: 39, y: 24, dest: "ROUTE_31_VIOLET_GATE", destWarp: 1},
			{x: 39, y: 25, dest: "ROUTE_31_VIOLET_GATE", destWarp: 2},
		},
		connections: []connectionSpec{
			{dir: dirSouth, dest: "ROUTE_32", offset: 0},
			{dir: dirWest, dest: "ROUTE_36", offset: 0},
			{dir: dirEast, dest: "ROUTE_31", offset: 9},
		},
	},
	{
		name: "VIOLET_MART",
		warps: []warpSpec{
			{x: 2, y: 7, dest: "VIOLET_CITY", destWarp: 1},
			{x: 3, y: 7, dest: "VIOLET_CITY", destWarp: 1},
		},
	},
	{
		name: "VIOLET_POKECENTER_1F",
		warps: []warpSpec{
			{x: 3, y: 7, dest: "VIOLET_CITY", destWarp: 5},
			{x: 4, y: 7, dest: "VIOLET_CITY", destWarp: 5},
			{x: 0, y: 7, dest: "POKECENTER_2F", destWarp: 1},
		},
	},
	{
		name: "VIOLET_GYM",
		warps: []warpSpec{
			{x: 4, y: 15, dest: "VIOLET_CITY", destWarp: 2},
			{x: 5, y: 15, dest: "VIOLET_CITY", destWarp: 2},
		},
	},
	{
		name: "SPROUT_TOWER_1F",
		warps: []warpSpec{
			{x: 9, y: 15, dest: "VIOLET_CITY", destWarp: 7},
			{x: 10, y: 15, dest: "VIOLET_CITY", destWarp: 7},
			{x: 6, y: 4, dest: "SPROUT_TOWER_2F", destWarp: 1},
			{x: 2, y: 6, dest: "SPROUT_TOWER_2F", destWarp: 2},
			{x: 17, y: 3, dest: "SPROUT_TOWER_2F", destWarp: 3},
		},
	},
	{
		name: "SPROUT_TOWER_2F",
		warps: []warpSpec{
			{x: 6, y: 4, dest: "SPROUT_TOWER_1F", destWarp: 3},
			{x: 2, y: 6, dest: "SPROUT_TOWER_1F", destWarp: 4},
			{x: 17, y: 3, dest: "SPROUT_TOWER_1F", destWarp: 5},
			{x: 10, y: 14, dest: "SPROUT_TOWER_3F", destWarp: 1},
		},
	},
	{
		name: "SPROUT_TOWER_3F",
		warps: []warpSpec{
			{x: 10, y: 14, dest: "SPROUT_TOWER_2F", destWarp: 4},
		},
	},
}

type firstBadgeWorldProvider struct {
	ids   []uint16
	specs map[uint16]mapSpec
}

// NewFirstBadgeWorldProvider returns the verified Gold/Silver topology needed
// for the fresh-save -> Violet/Falkner vertical slice.
//
// romData is accepted now so this constructor can grow into the real #973 ROM
// parser without changing its profile-facing shape. The current slice uses
// only generated catalog identities plus decomp-verified topology facts.
func NewFirstBadgeWorldProvider(romData []byte) worldmodel.NativeGridProvider {
	_ = romData
	specs := make(map[uint16]mapSpec, len(firstBadgeTopology))
	ids := make([]uint16, 0, len(firstBadgeTopology))
	for _, spec := range firstBadgeTopology {
		info, ok := gsdata.MapByName(spec.name)
		if !ok {
			continue
		}
		id := gsdata.NativeMapID(info.Group, info.Number)
		specs[id] = spec
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return &firstBadgeWorldProvider{ids: ids, specs: specs}
}

func (p *firstBadgeWorldProvider) MapIDs() []uint16 {
	if p == nil {
		return nil
	}
	return append([]uint16(nil), p.ids...)
}

func (p *firstBadgeWorldProvider) ParseMap(mapID uint16) (worldmodel.NativeMapHeader, error) {
	if p == nil {
		return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: nil first-badge world provider")
	}
	spec, ok := p.specs[mapID]
	if !ok {
		return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: map %#04x is outside first-badge topology slice", mapID)
	}
	info, ok := gsdata.Map(mapID)
	if !ok {
		return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: generated map %#04x is missing", mapID)
	}

	header := worldmodel.NativeMapHeader{
		ID:           mapID,
		WidthBlocks:  info.WidthBlocks,
		HeightBlocks: info.HeightBlocks,
		Warps:        make([]worldmodel.NativeWarp, 0, len(spec.warps)),
		Connections:  make([]worldmodel.NativeConnection, 0, len(spec.connections)),
	}
	for _, warp := range spec.warps {
		dest, ok := gsdata.MapByName(warp.dest)
		if !ok {
			return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: %s warp destination %q missing from generated catalog", spec.name, warp.dest)
		}
		if warp.destWarp == 0 {
			return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: %s warp at (%d,%d) has zero decomp destination warp", spec.name, warp.x, warp.y)
		}
		header.Warps = append(header.Warps, worldmodel.NativeWarp{
			X:          warp.x,
			Y:          warp.y,
			DestWarpID: warp.destWarp - 1,
			DestMap:    gsdata.NativeMapID(dest.Group, dest.Number),
		})
	}
	for _, connection := range spec.connections {
		dest, ok := gsdata.MapByName(connection.dest)
		if !ok {
			return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: %s connection destination %q missing from generated catalog", spec.name, connection.dest)
		}
		header.Connections = append(header.Connections, worldmodel.NativeConnection{
			Dir:    connection.dir,
			MapID:  gsdata.NativeMapID(dest.Group, dest.Number),
			Offset: connection.offset,
		})
	}
	return header, nil
}
