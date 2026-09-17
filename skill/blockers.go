package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// pikaFollowerSlot is the sprite slot Pokémon Yellow's Pikachu follower
// occupies (PIKACHU_SPRITE_INDEX = NUM_SPRITESTATA_STRUCTS - 1 = 15;
// wSpritePikachuStateData1 is struct 15 in the shared sprite array). Red has
// no such slot: its slot 15 is an ordinary NPC.
const pikaFollowerSlot = 15

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
func spriteBlockers(m *emu.Emu, romData []byte) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return spritesBlocked(&mem, tablesForROM(romData))
}

// spritesBlocked maps the decoded live objects to the tiles they occupy,
// dropping the passable-follower slot the ROM names, if any. It is split from
// spriteBlockers so it can be driven from a synthesised snapshot: the
// emulator exposes no memory-write API, and a decision this narrow should be
// testable without booting to the exact state that triggers it.
func spritesBlocked(mem *state.Mem, ts romTableSet) map[[2]int]bool {
	blocked := map[[2]int]bool{}
	for _, s := range ts.wram.DecodeSprites(mem) {
		if ts.passableSpriteSlot != 0 && s.Slot == ts.passableSpriteSlot {
			continue
		}
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
func liveBlockers(m *emu.Emu, romData []byte, h rom.MapHeader) map[[2]int]bool {
	return mergeBlockers(spriteBlockers(m, romData), stationaryObjectBlockers(h))
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
