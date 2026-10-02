package rom

import (
	"fmt"
	"sort"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/worldmodel"
)

// romWorldProvider is the full Gold/Silver map graph parsed from a ROM.
type romWorldProvider struct {
	rom    []byte
	tables Tables
	ids    []uint16
	grids  worldmodel.NativeGridProvider
}

// NewWorldProvider parses every catalog map from romData. Both Johto and
// Kanto are included. Destinations that are not in the catalog are left on
// the header so a missing dest is visible; BuildNativeGraph drops edges
// whose dest is not enumerated.
func NewWorldProvider(romData []byte) (worldmodel.NativeGridProvider, error) {
	if len(romData) == 0 {
		return nil, fmt.Errorf("gs/rom: empty ROM")
	}
	tables, err := LocateTables(romData)
	if err != nil {
		return nil, err
	}
	ids := make([]uint16, 0, len(gsdata.Maps()))
	for _, info := range gsdata.Maps() {
		ids = append(ids, gsdata.NativeMapID(info.Group, info.Number))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return &romWorldProvider{
		rom:    romData,
		tables: tables,
		ids:    ids,
		grids:  NewFirstBadgeWorldProvider(romData),
	}, nil
}

func (p *romWorldProvider) MapIDs() []uint16 {
	if p == nil {
		return nil
	}
	return append([]uint16(nil), p.ids...)
}

func (p *romWorldProvider) ParseMap(mapID uint16) (worldmodel.NativeMapHeader, error) {
	if p == nil {
		return worldmodel.NativeMapHeader{}, fmt.Errorf("gs/rom: nil world provider")
	}
	h, err := parseMapAt(p.rom, p.tables, mapID)
	if err != nil {
		return worldmodel.NativeMapHeader{}, err
	}
	return h.NativeHeader(), nil
}

func (p *romWorldProvider) Grid(mapID uint16, blocks []byte, mode worldmodel.TraversalMode) (worldmodel.NativeGridSpec, error) {
	if p == nil || p.grids == nil {
		return worldmodel.NativeGridSpec{}, fmt.Errorf("gs/rom: nil world provider")
	}
	return p.grids.Grid(mapID, blocks, mode)
}
