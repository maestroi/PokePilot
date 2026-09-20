package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// spriteBlockers snapshots the sprite RAM and returns the tiles the live map
// objects currently occupy. Keys are [2]int{X, Y}, the same key order
// walkAround and world.FindPath use. The snapshot is a fresh observation of
// where sprites ARE right now; it is not merged into or cached against
// anything.
//
// Sprite RAM only tracks objects near the player, mirroring the hardware's
// own OAM limit: an object several tiles away is not "empty," it is simply
// not decoded yet. A plan from far away sees no blocker there and walks
// straight for it; liveBlockers is the fix for planning across that
// distance, this is the raw, distance-limited observation for callers that
// are already close (grinding, fishing, an adjacent interaction).
func spriteBlockers(m *emu.Emu) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	blocked := map[[2]int]bool{}
	for _, s := range state.DecodeSprites(&mem) {
		blocked[[2]int{s.X, s.Y}] = true
	}
	return blocked
}

// stationaryObjectBlockers returns the home tiles of every MovementStay
// object on h — NPC or ground item alike. Pickup's own doc comment already
// establishes that an item ball is solid: it "walks to the tile adjacent to
// an item ball, faces it and takes it," never walks onto one. A toggled-
// hidden object (a defeated Giovanni, an already-collected ball) costs
// nothing worse than a route that avoids a tile that turned out to be open;
// a MovementWalk object is not included, since its live position is exactly
// what spriteBlockers reports when in range and this layer has no better
// answer for it out of range.
func stationaryObjectBlockers(h rom.MapHeader) map[[2]int]bool {
	return presentStationaryObjectBlockers(h, nil)
}

// presentStationaryObjectBlockers returns MovementStay home tiles that are
// still present on the map: every stay object whose 1-based object id is not
// in hidden. Unlike observedStationaryObjectBlockers, this is map-wide — an
// off-screen trainer or item ball still occupies its home tile — matching
// HiddenObjectIDs' map-wide missable list. Use it for local planning and for
// component topology so a distant NPC that cuts a floor is visible to the
// router before the sprite buffer loads it.
func presentStationaryObjectBlockers(h rom.MapHeader, hidden map[uint8]bool) map[[2]int]bool {
	blocked := map[[2]int]bool{}
	for i, o := range h.Objects {
		if o.Movement != rom.MovementStay || hidden[uint8(i+1)] {
			continue
		}
		blocked[[2]int{int(o.X), int(o.Y)}] = true
	}
	return blocked
}

// observedStationaryObjectBlockers returns only stationary object home tiles
// that are present in the current sprite snapshot. Unlike present stationary
// blockers, this drops off-screen stay objects, so it must not be the sole
// input to map-component topology on floors where a distant NPC is the cut.
// Moving sprites are excluded because their positions are observations for
// one walking plan, not stable geometry for later map legs.
func observedStationaryObjectBlockers(h rom.MapHeader, live []state.SpriteState) map[[2]int]bool {
	blocked := map[[2]int]bool{}
	for _, sprite := range live {
		if sprite.Slot < 1 || sprite.Slot > len(h.Objects) {
			continue
		}
		o := h.Objects[sprite.Slot-1]
		if o.Movement != rom.MovementStay || sprite.X != int(o.X) || sprite.Y != int(o.Y) {
			continue
		}
		blocked[[2]int{sprite.X, sprite.Y}] = true
	}
	return blocked
}

func currentObservedStationaryObjectBlockers(m *emu.Emu, h rom.MapHeader) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return observedStationaryObjectBlockers(h, state.DecodeSprites(&mem))
}

func currentPresentStationaryObjectBlockers(m *emu.Emu, h rom.MapHeader) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return presentStationaryObjectBlockers(h, state.HiddenObjectIDs(&mem))
}

// liveBlockers is spriteBlockers widened with present stationary objects, for
// callers that plan a route across a distance the player has not yet
// crossed: MEASURED on Pokemon Tower 5F (run-3anwzvms26fjy32alh211qa4fn and
// run-27a3sz93t4z9t3vgoljp7oofcc), a Channeler at (17,7) invisible to
// spriteBlockers from (11,9) let the planner repeatedly choose a route
// straight through his tile, discover the live collision once close, detour
// all the way back through Pokemon Tower 5F's purified-zone clearing (whose
// auto-heal box retriggers on every fresh entry), and repeat — tripping
// Travel's same-box loop guard on a walk that was never actually stuck, just
// oscillating between two routes neither snapshot alone ruled out.
func liveBlockers(m *emu.Emu, h rom.MapHeader) map[[2]int]bool {
	return mergeBlockers(spriteBlockers(m), currentPresentStationaryObjectBlockers(m, h))
}

// mergeBlockers returns the union of live and fixed blockers as a new map
// that owns its entries, so neither input is mutated or aliased by the
// result.
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
