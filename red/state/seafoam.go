package state

// SeafoamDropStage names the four floors where the two current-controlling
// boulders must be sunk into holes. The stage values follow the physical
// descent from 1F through B3F.
type SeafoamDropStage uint8

const (
	SeafoamDrop1F SeafoamDropStage = iota
	SeafoamDropB1F
	SeafoamDropB2F
	SeafoamDropB3F
)

const (
	eventSeafoam1Boulder1DownHole Event = 0x50e
	eventSeafoam1Boulder2DownHole Event = 0x50f
	eventSeafoam2Boulder1DownHole Event = 0x9c0
	eventSeafoam2Boulder2DownHole Event = 0x9c1
	eventSeafoam3Boulder1DownHole Event = 0x9c8
	eventSeafoam3Boulder2DownHole Event = 0x9c9
	eventSeafoam4Boulder1DownHole Event = 0x9d0
	eventSeafoam4Boulder2DownHole Event = 0x9d1
)

// SeafoamDropEvents returns the durable event bits set by the map script when
// the first/second boulder for a stage reaches its corresponding hole.
func SeafoamDropEvents(stage SeafoamDropStage) (Event, Event, bool) {
	switch stage {
	case SeafoamDrop1F:
		return eventSeafoam1Boulder1DownHole, eventSeafoam1Boulder2DownHole, true
	case SeafoamDropB1F:
		return eventSeafoam2Boulder1DownHole, eventSeafoam2Boulder2DownHole, true
	case SeafoamDropB2F:
		return eventSeafoam3Boulder1DownHole, eventSeafoam3Boulder2DownHole, true
	case SeafoamDropB3F:
		return eventSeafoam4Boulder1DownHole, eventSeafoam4Boulder2DownHole, true
	default:
		return 0, 0, false
	}
}

// SeafoamDropComplete reports whether one of the two durable hole events for
// stage is set. index is zero-based.
func SeafoamDropComplete(m *Mem, stage SeafoamDropStage, index int) bool {
	if m == nil || index < 0 || index > 1 {
		return false
	}
	first, second, ok := SeafoamDropEvents(stage)
	if !ok {
		return false
	}
	if index == 0 {
		return HasEvent(m, first)
	}
	return HasEvent(m, second)
}

// SeafoamStageComplete reports whether both boulders for stage have completed
// their hole transitions.
func SeafoamStageComplete(m *Mem, stage SeafoamDropStage) bool {
	return SeafoamDropComplete(m, stage, 0) && SeafoamDropComplete(m, stage, 1)
}

// SeafoamCurrentsStopped is the ROM-backed postcondition for the final Seafoam
// current puzzle. IsSurfingAllowed checks these same B3F->B4F hole events
// before allowing Surf in the lowest level.
func SeafoamCurrentsStopped(m *Mem) bool {
	return SeafoamStageComplete(m, SeafoamDropB3F)
}
