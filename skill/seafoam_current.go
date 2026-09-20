package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	seafoamB1FMap uint8 = 0x9f
	seafoamB2FMap uint8 = 0xa0
	seafoamB3FMap uint8 = 0xa1
	seafoamB4FMap uint8 = 0xa2
	seafoam1FMap  uint8 = 0xc0
)

type seafoamDropStageSpec struct {
	Stage state.SeafoamDropStage
	Map   uint8
	Holes [2]world.Point
	Slots [2]int
}

var seafoamDropStages = [...]seafoamDropStageSpec{
	{
		Stage: state.SeafoamDrop1F,
		Map:   seafoam1FMap,
		Holes: [2]world.Point{{X: 17, Y: 6}, {X: 24, Y: 6}},
		Slots: [2]int{1, 2},
	},
	{
		Stage: state.SeafoamDropB1F,
		Map:   seafoamB1FMap,
		Holes: [2]world.Point{{X: 18, Y: 6}, {X: 23, Y: 6}},
		Slots: [2]int{1, 2},
	},
	{
		Stage: state.SeafoamDropB2F,
		Map:   seafoamB2FMap,
		Holes: [2]world.Point{{X: 19, Y: 6}, {X: 22, Y: 6}},
		Slots: [2]int{1, 2},
	},
	{
		Stage: state.SeafoamDropB3F,
		Map:   seafoamB3FMap,
		Holes: [2]world.Point{{X: 3, Y: 16}, {X: 6, Y: 16}},
		Slots: [2]int{1, 2},
	},
}

func seafoamDropStageForMap(mapID uint8) (seafoamDropStageSpec, bool) {
	for _, spec := range seafoamDropStages {
		if spec.Map == mapID {
			return spec, true
		}
	}
	return seafoamDropStageSpec{}, false
}

func seafoamBoulderDropSpec(stage seafoamDropStageSpec, index int) (BoulderPuzzleSpec, error) {
	if index < 0 || index >= len(stage.Holes) {
		return BoulderPuzzleSpec{}, fmt.Errorf("skill: Seafoam boulder index %d is out of range", index)
	}
	firstEvent, secondEvent, ok := state.SeafoamDropEvents(stage.Stage)
	if !ok {
		return BoulderPuzzleSpec{}, fmt.Errorf("skill: Seafoam stage %d has no event mapping", stage.Stage)
	}
	event := firstEvent
	if index == 1 {
		event = secondEvent
	}
	hole := stage.Holes[index]
	return BoulderPuzzleSpec{
		Map:              stage.Map,
		Targets:          []world.Point{hole},
		MovableIDs:       map[int]bool{stage.Slots[index]: true},
		TerminalTargets:  map[[2]int]bool{{hole.X, hole.Y}: true},
		CompleteEvent:    event,
		HasCompleteEvent: true,
	}, nil
}

// solveSeafoamDropsOnCurrentFloor completes only the two boulder drops owned by
// the currently loaded Seafoam floor. It intentionally does not choose stairs
// or cross floors: the caller owns inter-floor travel and invokes this again
// after reaching the next live map.
//
// Each hole is solved independently and re-observed from RAM. This mirrors the
// ROM scripts, which hide one source object, show its counterpart below, and
// set one durable event after every successful terminal push.
func solveSeafoamDropsOnCurrentFloor(m *emu.Emu, romData []byte, policy MovePolicy) (BoulderPuzzleResult, error) {
	mapID := m.Peek8(sym.CurMap)
	stage, ok := seafoamDropStageForMap(mapID)
	if !ok {
		return BoulderPuzzleResult{}, fmt.Errorf("skill: Seafoam current puzzle requested on non-drop map %#02x", mapID)
	}

	var total BoulderPuzzleResult
	for index := 0; index < 2; index++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.SeafoamDropComplete(&mem, stage.Stage, index) {
			continue
		}

		spec, err := seafoamBoulderDropSpec(stage, index)
		if err != nil {
			return total, err
		}
		result, err := SolveBoulderPuzzle(m, romData, policy, spec)
		total.Pushes += result.Pushes
		total.Replans += result.Replans
		total.Explored += result.Explored
		if err != nil {
			return total, fmt.Errorf("skill: Seafoam floor %#02x boulder %d: %w", mapID, index+1, err)
		}

		state.Snapshot(m, &mem)
		if !state.SeafoamDropComplete(&mem, stage.Stage, index) {
			return total, fmt.Errorf("skill: Seafoam floor %#02x boulder %d reached its hole without setting the drop event", mapID, index+1)
		}
	}
	return total, nil
}

// seafoamCurrentsStopped reads the same final pair of events used by Red's
// IsSurfingAllowed check on B4F.
func seafoamCurrentsStopped(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.SeafoamCurrentsStopped(&mem)
}
