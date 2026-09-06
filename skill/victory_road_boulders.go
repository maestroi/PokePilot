package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

const (
	victoryRoad1FMap uint8 = 0x6c
	victoryRoad2FMap uint8 = 0xc2
	victoryRoad3FMap uint8 = 0xc6

	// ROM-derived from pokered/constants/event_constants.asm. The coordinates
	// below come from each Victory Road script's CheckBoulderCoords table.
	eventVictoryRoad1BoulderOnSwitch  state.Event = 0x917
	eventVictoryRoad2BoulderOnSwitch1 state.Event = 0x538
	eventVictoryRoad2BoulderOnSwitch2 state.Event = 0x53f
	eventVictoryRoad3BoulderOnSwitch1 state.Event = 0x660
	eventVictoryRoad3BoulderInHole    state.Event = 0x666
)

// VictoryRoadBoulderSection names the five story-relevant movable-object goals
// across Victory Road. These are goal facts only; no section contains an input
// sequence. SolveVictoryRoadBoulderSection always derives pushes from the live
// grid, player coordinate, and boulder sprite coordinates.
type VictoryRoadBoulderSection uint8

const (
	VictoryRoad1FSwitch VictoryRoadBoulderSection = iota
	VictoryRoad2FSwitch1
	VictoryRoad3FSwitch
	VictoryRoad3FHole
	VictoryRoad2FSwitch2
)

func (s VictoryRoadBoulderSection) String() string {
	switch s {
	case VictoryRoad1FSwitch:
		return "Victory Road 1F switch"
	case VictoryRoad2FSwitch1:
		return "Victory Road 2F west switch"
	case VictoryRoad3FSwitch:
		return "Victory Road 3F switch"
	case VictoryRoad3FHole:
		return "Victory Road 3F hole"
	case VictoryRoad2FSwitch2:
		return "Victory Road 2F east switch"
	default:
		return fmt.Sprintf("Victory Road boulder section %d", uint8(s))
	}
}

// VictoryRoadBoulderSpec returns the ROM-script goal for one section. Switch
// coordinates are the literal dbmapcoord entries consumed by
// CheckBoulderCoords; the 3F hole is marked terminal because the map script
// hides that 3F sprite and exposes the corresponding 2F boulder.
func VictoryRoadBoulderSpec(section VictoryRoadBoulderSection) (BoulderPuzzleSpec, bool) {
	var spec BoulderPuzzleSpec
	switch section {
	case VictoryRoad1FSwitch:
		spec = BoulderPuzzleSpec{
			Map:              victoryRoad1FMap,
			Targets:          []world.Point{{X: 17, Y: 13}},
			CompleteEvent:    eventVictoryRoad1BoulderOnSwitch,
			HasCompleteEvent: true,
		}
	case VictoryRoad2FSwitch1:
		spec = BoulderPuzzleSpec{
			Map:              victoryRoad2FMap,
			Targets:          []world.Point{{X: 1, Y: 16}},
			CompleteEvent:    eventVictoryRoad2BoulderOnSwitch1,
			HasCompleteEvent: true,
		}
	case VictoryRoad3FSwitch:
		spec = BoulderPuzzleSpec{
			Map:              victoryRoad3FMap,
			Targets:          []world.Point{{X: 3, Y: 5}},
			CompleteEvent:    eventVictoryRoad3BoulderOnSwitch1,
			HasCompleteEvent: true,
		}
	case VictoryRoad3FHole:
		spec = BoulderPuzzleSpec{
			Map:              victoryRoad3FMap,
			Targets:          []world.Point{{X: 23, Y: 15}},
			TerminalTargets:  map[[2]int]bool{{23, 15}: true},
			CompleteEvent:    eventVictoryRoad3BoulderInHole,
			HasCompleteEvent: true,
		}
	case VictoryRoad2FSwitch2:
		spec = BoulderPuzzleSpec{
			Map:              victoryRoad2FMap,
			Targets:          []world.Point{{X: 9, Y: 16}},
			CompleteEvent:    eventVictoryRoad2BoulderOnSwitch2,
			HasCompleteEvent: true,
		}
	default:
		return BoulderPuzzleSpec{}, false
	}
	return spec, true
}

// SolveVictoryRoadBoulderSection solves one Victory Road goal from the current
// observed state. It is safe to call from a checkpoint captured halfway
// through a puzzle: no initial boulder coordinate or previous push count is
// assumed.
func SolveVictoryRoadBoulderSection(m *emu.Emu, romData []byte, policy MovePolicy, section VictoryRoadBoulderSection) (BoulderPuzzleResult, error) {
	spec, ok := VictoryRoadBoulderSpec(section)
	if !ok {
		return BoulderPuzzleResult{}, fmt.Errorf("skill: unknown %s", section)
	}
	result, err := SolveBoulderPuzzle(m, romData, policy, spec)
	if err != nil {
		return result, fmt.Errorf("skill: %s: %w", section, err)
	}
	return result, nil
}
