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
