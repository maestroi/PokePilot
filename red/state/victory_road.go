package state

// eventVictoryRoad2EastSwitch is EVENT_VICTORY_ROAD_2_BOULDER_ON_SWITCH2.
// It is the final live puzzle gate in Victory Road: clearVictoryRoad returns
// only after this event is set, so it is a deterministic RAM-backed boundary
// between the cave transaction and the final Indigo lobby recovery transaction.
const eventVictoryRoad2EastSwitch Event = 0x53f

// VictoryRoadCleared reports whether the final 2F east switch has been
// satisfied. It is derived from current RAM and survives objective retries;
// no run-local checkpoint flag is involved.
func VictoryRoadCleared(m *Mem) bool {
	return m != nil && HasEvent(m, eventVictoryRoad2EastSwitch)
}
