package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
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

	seafoamB4FBlockedSurfX = 7
	seafoamB4FBlockedSurfY = 11
	seafoamTravelBattles   = 120
)

type seafoamDropStageSpec struct {
	Stage state.SeafoamDropStage
	Map   uint8
	Entry world.Point
	Holes [2]world.Point
	Slots [2]int
}

var seafoamDropStages = [...]seafoamDropStageSpec{
	{
		Stage: state.SeafoamDrop1F,
		Map:   seafoam1FMap,
		Entry: world.Point{X: 5, Y: 17},
		Holes: [2]world.Point{{X: 17, Y: 6}, {X: 24, Y: 6}},
		Slots: [2]int{1, 2},
	},
	{
		Stage: state.SeafoamDropB1F,
		Map:   seafoamB1FMap,
		Entry: world.Point{X: 7, Y: 5},
		Holes: [2]world.Point{{X: 18, Y: 6}, {X: 23, Y: 6}},
		Slots: [2]int{1, 2},
	},
	{
		Stage: state.SeafoamDropB2F,
		Map:   seafoamB2FMap,
		Entry: world.Point{X: 5, Y: 3},
		Holes: [2]world.Point{{X: 19, Y: 6}, {X: 22, Y: 6}},
		Slots: [2]int{1, 2},
	},
	{
		Stage: state.SeafoamDropB3F,
		Map:   seafoamB3FMap,
		Entry: world.Point{X: 5, Y: 12},
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

func seafoamSurfAllowedFrom(mem *state.Mem, mapID uint8, x, y int) bool {
	if mapID != seafoamB4FMap || state.SeafoamCurrentsStopped(mem) {
		return true
	}
	return x != seafoamB4FBlockedSurfX || y != seafoamB4FBlockedSurfY
}

func currentFieldPathRules(m *emu.Emu, h rom.MapHeader) fieldPathRules {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return fieldPathRules{
		SurfAllowedFrom: func(x, y int) bool {
			return seafoamSurfAllowedFrom(&mem, h.ID, x, y)
		},
		ForcedLanding: func(x, y int) (world.Point, bool) {
			return forcedLandingForMap(h.ID, x, y)
		},
		MoveAllowed: func(x, y int, input world.Step) bool {
			return cyclingRoadMoveAllowed(&mem, h.ID, input)
		},
	}
}

// seafoamCurrentBlocksDestination proves that the active B4F current is the
// only reason an otherwise legal local field path cannot reach dest. This
// keeps puzzle preparation destination-aware: a B4F destination that does not
// need Surf from the blocked stairs never causes unrelated boulders to move.
func seafoamCurrentBlocksDestination(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, blocked map[[2]int]bool) (bool, error) {
	if h.ID != seafoamB4FMap || dest.Map != seafoamB4FMap {
		return false, nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.SeafoamCurrentsStopped(&mem) {
		return false, nil
	}

	if _, err := currentFieldPathPlan(m, romData, h, dest, blocked); err == nil {
		return false, nil
	} else if !errors.Is(err, world.ErrNoPath) {
		return false, err
	}
	if _, err := currentFieldPathPlanWithRules(m, romData, h, dest, blocked, fieldPathRules{}); err == nil {
		return true, nil
	} else if !errors.Is(err, world.ErrNoPath) {
		return false, err
	}
	return false, nil
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

// prepareSeafoamCurrents completes the durable drop chain in ROM order. Each
// stage is reached through ordinary Travel so stairs/warps, encounters, and
// resumed checkpoints use the same navigation contracts as the rest of the
// game. The drop solver itself re-observes RAM after every push.
func prepareSeafoamCurrents(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: Seafoam current preparation requires a battle policy")
	}
	if seafoamCurrentsStopped(m) {
		return nil
	}
	if err := RepairFieldCapabilities(m, romData, policy, []FieldMove{FieldStrength, FieldSurf}); err != nil {
		return fmt.Errorf("skill: Seafoam current preparation: prepare Strength + Surf: %w", err)
	}

	for _, stage := range seafoamDropStages {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.SeafoamStageComplete(&mem, stage.Stage) {
			continue
		}
		if m.Peek8(sym.CurMap) != stage.Map {
			dest := Destination{Map: stage.Map, X: uint8(stage.Entry.X), Y: uint8(stage.Entry.Y)}
			if _, err := TravelFlee(m, romData, dest, policy, seafoamTravelBattles); err != nil {
				return fmt.Errorf("skill: Seafoam current preparation: reach stage %d on map %#02x: %w", stage.Stage, stage.Map, err)
			}
		}
		if _, err := solveSeafoamDropsOnCurrentFloor(m, romData, policy); err != nil {
			return fmt.Errorf("skill: Seafoam current preparation: stage %d: %w", stage.Stage, err)
		}
		state.Snapshot(m, &mem)
		if !state.SeafoamStageComplete(&mem, stage.Stage) {
			return fmt.Errorf("skill: Seafoam current preparation: stage %d completed without both durable drop events", stage.Stage)
		}
	}

	if !seafoamCurrentsStopped(m) {
		return fmt.Errorf("skill: Seafoam current preparation finished without stopping B4F current")
	}
	return nil
}

// seafoamCurrentsStopped reads the same final pair of events used by Red's
// IsSurfingAllowed check on B4F.
func seafoamCurrentsStopped(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.SeafoamCurrentsStopped(&mem)
}
