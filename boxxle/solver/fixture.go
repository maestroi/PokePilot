package solver

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/boxxle"
)

// fixtures embeds the checked-in, ROM-free Sokoban board fixtures so the
// solver, tests, and the boxxlebench CLI all reason over the exact same set.
//
//go:embed testdata
var fixtures embed.FS

// Fixture format (standard Sokoban text / LDS):
//
//	#  wall
//	@  player
//	+  player on a goal
//	$  crate
//	*  crate on a goal
//	.  goal
//	(space)  floor
//
// The board is the whole grid: there is no implicit border, so fixtures
// control the exact wall layout.
const fixtureFormat = "standard Sokoban text (LDS): # wall, @ player, + player-on-goal, $ crate, * crate-on-goal, . goal, space floor"

// LoadPuzzle parses one fixture in the standard Sokoban text format into a
// typed board state. It requires exactly one player and at least one floor
// cell, and rejects unknown characters so a typo in a fixture fails loudly
// instead of silently becoming floor.
func LoadPuzzle(name string, data []byte) (Puzzle, error) {
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return Puzzle{}, fmt.Errorf("boxxle solver: fixture %s is empty", name)
	}
	width := 0
	for _, row := range lines {
		if len(row) > width {
			width = len(row)
		}
	}
	if width == 0 {
		return Puzzle{}, fmt.Errorf("boxxle solver: fixture %s has no cells", name)
	}

	var walls, goals, crates []boxxle.Pos
	var player *boxxle.Pos
	for y, row := range lines {
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case '#':
				walls = append(walls, boxxle.Pos{X: x, Y: y})
			case '@':
				if player != nil {
					return Puzzle{}, fmt.Errorf("boxxle solver: fixture %s has multiple players", name)
				}
				player = &boxxle.Pos{X: x, Y: y}
			case '+':
				if player != nil {
					return Puzzle{}, fmt.Errorf("boxxle solver: fixture %s has multiple players", name)
				}
				player = &boxxle.Pos{X: x, Y: y}
				goals = append(goals, boxxle.Pos{X: x, Y: y})
			case '$':
				crates = append(crates, boxxle.Pos{X: x, Y: y})
			case '*':
				crates = append(crates, boxxle.Pos{X: x, Y: y})
				goals = append(goals, boxxle.Pos{X: x, Y: y})
			case '.':
				goals = append(goals, boxxle.Pos{X: x, Y: y})
			case ' ':
				// floor
			default:
				return Puzzle{}, fmt.Errorf("boxxle solver: fixture %s has unknown character %q at (%d,%d)", name, row[x], x, y)
			}
		}
	}
	if player == nil {
		return Puzzle{}, fmt.Errorf("boxxle solver: fixture %s has no player", name)
	}

	state := boxxle.State{
		Screen: boxxle.ScreenPuzzle,
		Width:  width,
		Height: len(lines),
		Walls:  walls,
		Goals:  goals,
		Crates: crates,
		Player: player,
		Solved: len(crates) > 0 && allOnGoals(crates, goals),
	}
	return Puzzle{Name: name, State: state}, nil
}

// Fixtures loads every checked-in fixture in deterministic (sorted) order.
func Fixtures() ([]Puzzle, error) {
	entries, err := fs.ReadDir(fixtures, "testdata")
	if err != nil {
		return nil, fmt.Errorf("boxxle solver: read fixtures: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	var out []Puzzle
	for _, name := range names {
		data, err := fixtures.ReadFile("testdata/" + name)
		if err != nil {
			return nil, fmt.Errorf("boxxle solver: read fixture %s: %w", name, err)
		}
		p, err := LoadPuzzle(strings.TrimSuffix(name, ".txt"), data)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func allOnGoals(crates, goals []boxxle.Pos) bool {
	set := make(map[boxxle.Pos]bool, len(goals))
	for _, g := range goals {
		set[g] = true
	}
	for _, c := range crates {
		if !set[c] {
			return false
		}
	}
	return true
}
