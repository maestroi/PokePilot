package profile

import (
	"fmt"

	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// NativeMapProvider exposes the verified wide-id Gold/Silver topology slice.
// The historical MapProvider contract remains uint8 and cannot represent the
// Gen-II (map group, map number) namespace without collisions.
func (*Profile) NativeMapProvider(romData []byte) worldmodel.NativeGridProvider {
	return gsrom.NewFirstBadgeWorldProvider(romData)
}

// MapAdjacency exposes the static wide-id map adjacency for the verified
// Gold/Silver topology slice. It builds the native graph (uint16 map ids) and
// projects it to the portable adjacency, so generic routing never narrows the
// Gen-II (group, number) namespace to the historical uint8 world graph.
func (p *Profile) MapAdjacency(romData []byte) (map[uint16][]uint16, error) {
	graph, err := world.BuildNativeGraph(p.NativeMapProvider(romData))
	if err != nil {
		return nil, fmt.Errorf("gs profile: build native map graph: %w", err)
	}
	adjacency := make(map[uint16][]uint16, len(graph.Edges))
	for from, edges := range graph.Edges {
		for _, e := range edges {
			adjacency[from] = append(adjacency[from], e.To)
		}
	}
	return adjacency, nil
}
