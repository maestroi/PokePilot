package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/worldmodel"
)

func spriteBlockersFromTopology(live game.LiveTopologyState) map[[2]int]bool {
	blocked := make(map[[2]int]bool, len(live.LiveObjects))
	for _, object := range live.LiveObjects {
		blocked[[2]int{object.X, object.Y}] = true
	}
	return blocked
}

// spriteBlockers returns the currently loaded object-event tiles. The profile
// owns sprite/object RAM layout; routing only consumes semantic positions.
func spriteBlockers(m *emu.Emu) map[[2]int]bool {
	live, err := liveTopologyState(m)
	if err != nil {
		return map[[2]int]bool{}
	}
	return spriteBlockersFromTopology(live)
}

func hiddenObjectSet(live game.LiveTopologyState) map[uint8]bool {
	hidden := make(map[uint8]bool, len(live.HiddenObjects))
	for slot, absent := range live.HiddenObjects {
		if absent && slot > 0 && slot <= 0xff {
			hidden[uint8(slot)] = true
		}
	}
	return hidden
}

func objectTileSet(live game.LiveTopologyState) map[int][2]int {
	tiles := make(map[int][2]int, len(live.ObjectPositions))
	for slot, at := range live.ObjectPositions {
		tiles[slot] = [2]int{at.X, at.Y}
	}
	return tiles
}

// stationaryObjectBlockers returns the live/fallback tiles for MovementStay
// objects. Hidden objects are absent; moved stay trainers use their semantic
// runtime position rather than their static header home.
func stationaryObjectBlockers(h worldmodel.HeaderView, tiles map[int][2]int, hidden map[uint8]bool) map[[2]int]bool {
	header := h.WorldMapHeader()
	blocked := map[[2]int]bool{}
	for i, o := range header.Objects {
		if o.Movement != worldmodel.ObjectMovementStay || hidden[uint8(i+1)] {
			continue
		}
		if t, ok := tiles[i+1]; ok {
			blocked[t] = true
			continue
		}
		blocked[[2]int{int(o.X), int(o.Y)}] = true
	}
	return blocked
}

// observedStationaryObjectBlockers returns stationary home tiles positively
// present in the current live-object snapshot. Moving objects never become
// persistent topology merely because they occupy the same coordinate.
func observedStationaryObjectBlockers(h worldmodel.HeaderView, live []game.LiveMapObject) map[[2]int]bool {
	header := h.WorldMapHeader()
	blocked := map[[2]int]bool{}
	for _, object := range live {
		if object.Slot < 1 || object.Slot > len(header.Objects) {
			continue
		}
		o := header.Objects[object.Slot-1]
		if o.Movement != worldmodel.ObjectMovementStay || object.X != int(o.X) || object.Y != int(o.Y) {
			continue
		}
		blocked[[2]int{object.X, object.Y}] = true
	}
	return blocked
}

func persistentTopologyBlockers(h worldmodel.HeaderView, hidden map[uint8]bool) map[[2]int]bool {
	header := h.WorldMapHeader()
	blocked := map[[2]int]bool{}
	for i, o := range header.Objects {
		if o.Movement != worldmodel.ObjectMovementStay || hidden[uint8(i+1)] {
			continue
		}
		blocked[[2]int{int(o.X), int(o.Y)}] = true
	}
	return blocked
}

func presentStationaryObjectBlockers(m *emu.Emu, h worldmodel.HeaderView) map[[2]int]bool {
	live, err := liveTopologyState(m)
	if err != nil {
		return persistentTopologyBlockers(h, nil)
	}
	return persistentTopologyBlockers(h, hiddenObjectSet(live))
}

func routingBlockersFromTopology(h worldmodel.HeaderView, live game.LiveTopologyState) map[[2]int]bool {
	return mergeBlockers(
		spriteBlockersFromTopology(live),
		persistentTopologyBlockers(h, hiddenObjectSet(live)),
	)
}

// routingBlockers combines stable present stay objects with the current live
// object snapshot for same-map path/component planning.
func routingBlockers(m *emu.Emu, h worldmodel.HeaderView) map[[2]int]bool {
	live, err := liveTopologyState(m)
	if err != nil {
		return persistentTopologyBlockers(h, nil)
	}
	return routingBlockersFromTopology(h, live)
}

func currentObservedStationaryObjectBlockers(m *emu.Emu, h worldmodel.HeaderView) map[[2]int]bool {
	live, err := liveTopologyState(m)
	if err != nil {
		return map[[2]int]bool{}
	}
	return observedStationaryObjectBlockers(h, live.LiveObjects)
}

// liveBlockers widens the current live object set with every present
// MovementStay object, using the profile-owned runtime position when one is
// available. This preserves off-screen trainer/item topology without reading
// concrete cartridge RAM in reusable routing.
func liveBlockers(m *emu.Emu, h worldmodel.HeaderView) map[[2]int]bool {
	live, err := liveTopologyState(m)
	if err != nil {
		return persistentTopologyBlockers(h, nil)
	}
	return mergeBlockers(
		spriteBlockersFromTopology(live),
		stationaryObjectBlockers(h, objectTileSet(live), hiddenObjectSet(live)),
	)
}

func mergeBlockers(live, fixed map[[2]int]bool) map[[2]int]bool {
	out := make(map[[2]int]bool, len(live)+len(fixed))
	for k := range live {
		out[k] = true
	}
	for k := range fixed {
		out[k] = true
	}
	return out
}
