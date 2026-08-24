package world

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
)

// maxMapID is the highest map id present in the Red ROM header tables.
const maxMapID = 0xF7

type EdgeKind uint8

const (
	EdgeWarp EdgeKind = iota // step into a warp tile
	EdgeConnection           // walk off the map edge
)

type Edge struct {
	Kind   EdgeKind
	From   uint8 // source map id
	To     uint8 // destination map id, already resolved (never 0xFF)
	WarpX  uint8 // EdgeWarp only: the warp tile on the source map
	WarpY  uint8
	Dir    uint8 // EdgeConnection only: 0=north 1=south 2=west 3=east
}

type Graph struct{ Edges map[uint8][]Edge }

// BuildGraph builds a MAP-level graph over every parseable map. Nodes are map
// ids; edges are warps and edge connections. Tile arrival coordinates are not
// stored: they are resolved at execution time from live RAM.
//
// A warp whose DestMap is 0xFF means "the map you came from". It is resolved
// statically to the unique map that has a warp back to the source, excluding
// any map the source already reaches via an explicit warp (that direction is
// covered by its own edge). If zero or more than one candidate remains the
// edge is dropped rather than guessed, so no edge ever has To == 0xFF.
func BuildGraph(romData []byte) (*Graph, error) {
	headers := make(map[uint8]rom.MapHeader)
	for id := uint8(0); id <= maxMapID; id++ {
		h, err := rom.ParseMap(romData, id)
		if err != nil {
			continue // invalid map id: skip, do not fail the build
		}
		headers[id] = h
	}
	if len(headers) == 0 {
		return nil, fmt.Errorf("no parseable maps in ROM of %d bytes", len(romData))
	}

	// warpTo[A] is the set of maps that have a warp (any DestWarpID) to A.
	warpTo := make(map[uint8]map[uint8]bool)
	// explicit[A] is the set of maps A reaches via a non-0xFF warp.
	explicit := make(map[uint8]map[uint8]bool)
	for id, h := range headers {
		for _, w := range h.Warps {
			if warpTo[w.DestMap] == nil {
				warpTo[w.DestMap] = make(map[uint8]bool)
			}
			warpTo[w.DestMap][id] = true
			if w.DestMap != 0xFF {
				if explicit[id] == nil {
					explicit[id] = make(map[uint8]bool)
				}
				explicit[id][w.DestMap] = true
			}
		}
	}

	g := &Graph{Edges: make(map[uint8][]Edge, len(headers))}
	for id := range headers {
		g.Edges[id] = nil // a node exists for every valid map, even edge-less ones
	}

	resolve := func(a uint8, w rom.Warp) (uint8, bool) {
		if w.DestMap != 0xFF {
			return w.DestMap, true
		}
		n, dest := 0, uint8(0)
		for b := range warpTo[a] {
			if explicit[a][b] {
				continue // a already warps explicitly to b
			}
			n++
			dest = b
		}
		if n != 1 {
			return 0, false // zero or multiple candidates: drop, do not guess
		}
		return dest, true
	}

	for id, h := range headers {
		for _, w := range h.Warps {
			to, ok := resolve(id, w)
			if !ok {
				continue
			}
			g.Edges[id] = append(g.Edges[id], Edge{
				Kind:  EdgeWarp,
				From:  id,
				To:    to,
				WarpX: w.X,
				WarpY: w.Y,
			})
		}
		for _, c := range h.Connections {
			g.Edges[id] = append(g.Edges[id], Edge{
				Kind: EdgeConnection,
				From: id,
				To:   c.MapID,
				Dir:  c.Dir,
			})
		}
	}
	return g, nil
}
