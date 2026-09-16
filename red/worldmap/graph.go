package worldmap

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// maxMapID is the highest map id present in Pokémon Red's map header tables.
// The Red parser rejects unused slots inside this range.
const maxMapID uint8 = 0xF7

// BuildGraph parses Pokémon Red's native map/elevator data and translates it
// into the game-agnostic topology consumed by world.BuildGraph.
func BuildGraph(romData []byte) (*world.Graph, error) {
	maps := make(map[uint8]world.MapTopology)
	for n := 0; n <= int(maxMapID); n++ {
		id := uint8(n)
		h, err := rom.ParseMap(romData, id)
		if err != nil {
			continue
		}
		m := world.MapTopology{
			ID:     id,
			Width:  int(h.WidthBlocks) * 2,
			Height: int(h.HeightBlocks) * 2,
		}
		for _, w := range h.Warps {
			m.Warps = append(m.Warps, world.Warp{
				X: w.X, Y: w.Y, DestWarpID: w.DestWarpID, DestMap: w.DestMap,
			})
		}
		for _, c := range h.Connections {
			m.Connections = append(m.Connections, world.Connection{Dir: c.Dir, MapID: c.MapID, Offset: c.Offset})
		}
		if grid, err := Build(romData, h); err == nil {
			m.Grid = grid
		}
		if elevator, ok := rom.LookupElevator(id); ok {
			for _, floor := range elevator.Floors {
				m.Elevator = append(m.Elevator, world.ElevatorFloor{MapID: floor.MapID, DestWarpID: floor.DestWarpID})
			}
		}
		maps[id] = m
	}
	if len(maps) == 0 {
		return nil, fmt.Errorf("no parseable maps in ROM of %d bytes", len(romData))
	}
	return world.BuildGraph(maps)
}
