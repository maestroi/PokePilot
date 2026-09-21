package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
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
	blocked := map[[2]int]bool{}
	for _, o := range h.Objects {
		if o.Movement == rom.MovementStay {
			blocked[[2]int{int(o.X), int(o.Y)}] = true
		}
	}
	return blocked
}

// observedStationaryObjectBlockers returns only stationary object home tiles
// that are present in the current sprite snapshot. Unlike liveBlockers, this
// is used to change map component topology, so a hidden item or defeated
// trainer must not split the map after it has disappeared. Moving sprites are
// excluded because their positions are observations for one walking plan, not
// stable geometry for later map legs.
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

// presentStationaryObjectBlockers returns every Stay-home tile that still
// exists on the current map. Hidden/missable objects are omitted; defeated
// trainers that remain as solid sprites are kept. Unlike the observed-only
// snapshot, this does not depend on sprite RAM proximity — a corridor NPC
// several rooms away still occupies its tile.
func presentStationaryObjectBlockers(m *emu.Emu, h rom.MapHeader) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	hidden := state.HiddenObjectIDs(&mem)
	blocked := map[[2]int]bool{}
	for i, o := range h.Objects {
		if o.Movement != rom.MovementStay || hidden[uint8(i+1)] {
			continue
		}
		blocked[[2]int{int(o.X), int(o.Y)}] = true
	}
	return blocked
}

// liveBlockers is spriteBlockers widened with h's stationary objects, for
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
	return mergeBlockers(spriteBlockers(m), stationaryObjectBlockers(h))
}

// softPreferStationaryBlockers is the destination-aware form of liveBlockers.
// It starts from the live sprite snapshot and adds each Stay-home tile only
// when the caller's plan still admits a path with that tile occupied.
//
// Full Stay-home widening can invent a dead end: MEASURED on Silph Co 5F
// (run-2wvvgm279oaj1uhpm9pfh4ofm), marking every Stay object occupied removes
// the only stair-landing → Card Key approach because a Rocket's home tile sits
// on that corridor. Soft preference keeps every Stay home that is merely a
// detour, and leaves corridor-critical trainers for live collision + battle
// resolution instead of teaching walkAround to "learn" them as walls.
func softPreferStationaryBlockers(m *emu.Emu, h rom.MapHeader, plan func(blocked map[[2]int]bool) ([]world.Step, error)) map[[2]int]bool {
	blocked := spriteBlockers(m)
	for tile := range stationaryObjectBlockers(h) {
		if blocked[tile] {
			continue
		}
		trial := mergeBlockers(blocked, map[[2]int]bool{tile: true})
		if _, err := plan(trial); err == nil {
			blocked[tile] = true
		}
	}
	return blocked
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

// stayTrainerHome reports whether (x,y) is the ROM home tile of an undefeated
// Stay trainer object on h. Item balls and ordinary NPCs return false.
func stayTrainerHome(m *emu.Emu, romData []byte, h rom.MapHeader, x, y int) bool {
	for _, o := range h.Objects {
		if o.Movement != rom.MovementStay || int(o.X) != x || int(o.Y) != y || o.TrainerClass == 0 {
			continue
		}
		target, err := trainerTargetAt(romData, h, o.X, o.Y)
		if err != nil {
			return true
		}
		return !target.flag.set(m)
	}
	return false
}
