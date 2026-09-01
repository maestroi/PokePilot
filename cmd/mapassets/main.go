// Command mapassets exports ROM-backed semantic map geometry for pokeui.
// It never writes or serves ROM bytes: the generated JSON is only the
// walkability grid plus warp/connection metadata used by the live watch map.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

type mapAsset struct {
	ID          uint8       `json:"id"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Cells       string      `json:"cells"`
	Warps       []warpAsset `json:"warps,omitempty"`
	Connections []string    `json:"connections,omitempty"`
}

type warpAsset struct {
	X    uint8 `json:"x"`
	Y    uint8 `json:"y"`
	Dest uint8 `json:"dest"`
}

func directionName(dir uint8) string {
	switch dir {
	case 0:
		return "north"
	case 1:
		return "south"
	case 2:
		return "west"
	case 3:
		return "east"
	default:
		return fmt.Sprintf("dir-%d", dir)
	}
}

func buildAsset(romData []byte, id uint8) (mapAsset, error) {
	h, err := redrom.ParseMap(romData, id)
	if err != nil {
		return mapAsset{}, err
	}
	g, err := world.Build(romData, h)
	if err != nil {
		return mapAsset{}, err
	}

	cells := make([]byte, g.Width*g.Height)
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			ch := byte('#')
			if g.Walkable(x, y) {
				ch = '.'
			}
			cells[y*g.Width+x] = ch
		}
	}

	a := mapAsset{ID: id, Width: g.Width, Height: g.Height, Cells: string(cells)}
	for _, w := range h.Warps {
		if int(w.X) < g.Width && int(w.Y) < g.Height {
			cells[int(w.Y)*g.Width+int(w.X)] = 'W'
		}
		a.Warps = append(a.Warps, warpAsset{X: w.X, Y: w.Y, Dest: w.DestMap})
	}
	a.Cells = string(cells)
	for _, c := range h.Connections {
		a.Connections = append(a.Connections, directionName(c.Dir))
	}
	return a, nil
}

func main() {
	out := flag.String("o", "cmd/pokeui/ui/maps", "output directory")
	flag.Parse()

	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		fmt.Fprintln(os.Stderr, "mapassets: POKEMON_RED_ROM is required")
		os.Exit(2)
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mapassets: read ROM: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mapassets: mkdir: %v\n", err)
		os.Exit(1)
	}

	written := 0
	for n := 0; n < 256; n++ {
		id := uint8(n)
		if state.MapName(id) == "" {
			continue
		}
		a, err := buildAsset(romData, id)
		if err != nil {
			// The decomp names a handful of unused slots whose ROM headers are
			// intentionally not useful maps. Only emit maps the existing parser
			// and world builder can describe.
			continue
		}
		data, err := json.Marshal(a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mapassets: map %02x: encode: %v\n", id, err)
			os.Exit(1)
		}
		data = append(data, '\n')
		path := filepath.Join(*out, fmt.Sprintf("%02x.json", id))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "mapassets: map %02x: write: %v\n", id, err)
			os.Exit(1)
		}
		written++
	}
	fmt.Printf("wrote %d semantic map assets to %s\n", written, *out)
}
