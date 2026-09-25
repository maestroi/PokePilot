package state

// eventVictoryRoad2EastSwitch is EVENT_VICTORY_ROAD_2_BOULDER_ON_SWITCH2.
// It is the final live puzzle gate in Victory Road: clearVictoryRoad returns
// only after this event is set, so it is a deterministic RAM-backed boundary
// for the cave transaction. Route 23 deliberately resets Victory Road boulder
// events when its map script loads, so callers that need a later milestone must
// combine this live event with post-cave geography rather than treating it as a
// permanently durable story flag.
const eventVictoryRoad2EastSwitch Event = 0x53f

// VictoryRoadCleared reports whether the final 2F east switch is currently
// satisfied. It is derived from current RAM and is authoritative while inside
// the cave; loading Route 23 can reset it by design.
func VictoryRoadCleared(m *Mem) bool {
	return m != nil && HasEvent(m, eventVictoryRoad2EastSwitch)
}

// Route 23 tiles north of Victory Road, measured from the live collision grid
// (the walkable component reached through the 2F exit versus the one reached
// from the 1F entrance). Both cave doors sit on row 31: the 1F entrance at
// (4,31) is south-side, while the 2F exit at (14,31) opens into an east pocket
// that runs down to row 37 beside the south side's x<=7 corridor, so no single
// row threshold separates them.
const (
	route23NorthMaxY      = 30
	route23ExitPocketMinX = 14
	route23ExitPocketMaxY = 37
)

// Route23NorthOfVictoryRoad reports whether a Route 23 coordinate is on the
// Indigo side of the cave, i.e. reachable from the 2F exit without re-entering
// Victory Road.
func Route23NorthOfVictoryRoad(x, y int) bool {
	return y <= route23NorthMaxY || (x >= route23ExitPocketMinX && y <= route23ExitPocketMaxY)
}
