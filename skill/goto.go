package skill

import (
	"errors"
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// Destination is a concrete place: a map and a standing position on it.
type Destination struct {
	Map uint8
	X, Y uint8
}

// ErrBattle is returned by GoTo when a wild battle interrupts the route. GoTo
// never fights or flees; it aborts and reports the battle.
var ErrBattle = errors.New("skill: battle interrupted the route")

// GoTo walks the player to dest, crossing maps as needed. The graph is built
// once; after every leg the current map and coordinates are re-read from RAM
// and the remaining route is re-planned, so a leg that lands the player
// somewhere unexpected is recovered from rather than assumed.
func GoTo(m *emu.Emu, romData []byte, dest Destination) error {
	g, err := world.BuildGraph(romData)
	if err != nil {
		return err
	}

	for {
		if err := abortIfBattle(m); err != nil {
			return err
		}
		cur := m.Peek8(sym.CurMap)
		x, y := playerXY(m)

		if cur == dest.Map {
			return walkWithinMap(m, romData, dest)
		}

		route, err := world.FindRoute(g, cur, dest.Map)
		if err != nil {
			return fmt.Errorf("skill: GoTo: no route from map %02x at (%d,%d) to map %02x at (%d,%d): %w",
				cur, x, y, dest.Map, dest.X, dest.Y, err)
		}
		if len(route) == 0 {
			return walkWithinMap(m, romData, dest)
		}

		e := route[0]
		if err := Traverse(m, romData, e); err != nil {
			return fmt.Errorf("skill: GoTo: %w", err)
		}
	}
}

// places is the single source of truth for the names Place accepts.
var places = map[string]Destination{
	"reds bedroom":            {Map: 0x26, X: 3, Y: 6},
	"reds house":              {Map: 0x25, X: 3, Y: 2},
	"pallet town":             {Map: 0x00, X: 5, Y: 6},
	"viridian city":           {Map: 0x01, X: 23, Y: 26},
	"viridian pokemon center": {Map: 0x29, X: 3, Y: 2},
}

// Place maps a friendly name to a Destination.
func Place(name string) (Destination, bool) {
	d, ok := places[name]
	return d, ok
}

// PlaceNames returns every name Place accepts, sorted, so a caller can offer
// one objective per place without duplicating the list.
func PlaceNames() []string {
	names := make([]string, 0, len(places))
	for name := range places {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// abortIfBattle returns an error wrapping ErrBattle when a battle is active,
// carrying the current map and coordinates.
func abortIfBattle(m *emu.Emu) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) != nil {
		x, y := playerXY(m)
		return fmt.Errorf("skill: GoTo: battle on map %02x at (%d,%d): %w",
			m.Peek8(sym.CurMap), x, y, ErrBattle)
	}
	return nil
}

// walkWithinMap walks the player from their current position to dest on the
// current map, retrying around dynamic obstacles (tiles the static collision
// grid does not know about) up to maxRetries times.
func walkWithinMap(m *emu.Emu, romData []byte, dest Destination) error {
	cur := m.Peek8(sym.CurMap)
	sx, sy := playerXY(m)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return fmt.Errorf("skill: GoTo: parse map %02x at (%d,%d): %w", cur, sx, sy, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return fmt.Errorf("skill: GoTo: build map %02x at (%d,%d): %w", cur, sx, sy, err)
	}

	blocked := map[[2]int]bool{}
	const maxRetries = 3
	for attempt := 0; ; attempt++ {
		x, y := playerXY(m)
		steps, err := world.FindPath(grid, int(x), int(y), int(dest.X), int(dest.Y), blocked)
		if err != nil {
			return fmt.Errorf("skill: GoTo: no path on map %02x from (%d,%d) to (%d,%d): %w",
				cur, x, y, dest.X, dest.Y, err)
		}
		if err := WalkPath(m, steps); err != nil {
			if errors.Is(err, ErrBattleInterrupted) {
				x, y := playerXY(m)
				return fmt.Errorf("skill: GoTo: battle on map %02x at (%d,%d): %w", cur, x, y, ErrBattle)
			}
			var eb *ErrBlocked
			if errors.As(err, &eb) {
				if attempt >= maxRetries {
					return fmt.Errorf("skill: GoTo: blocked on map %02x at (%d,%d) after %d retries: %w",
						cur, eb.At.X, eb.At.Y, maxRetries, err)
				}
				blocked[[2]int{int(eb.At.X) + eb.Step.DX, int(eb.At.Y) + eb.Step.DY}] = true
				continue
			}
			return fmt.Errorf("skill: GoTo: walk on map %02x at (%d,%d): %w", cur, x, y, err)
		}
		return nil
	}
}
