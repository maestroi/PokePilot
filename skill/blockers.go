package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/worldmodel"
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
// hidden object (a defeated Giovanni, an already-collected ball, a rival who
// walked off and was hidden) is absent, so it blocks nothing: its RAM tile is
// wherever it last stood, which can be a doorway (Oak's Lab rival at (4,10)
// before the exit mat, run-22ahrk9pflcilu3jxq9xt37x6); a MovementWalk object is not included, since its live position is exactly
// what spriteBlockers reports when in range and this layer has no better
// answer for it out of range.
//
// tiles is the per-slot RAM position (state.DecodeObjectTiles). A stay trainer
// that walked out to intercept the player stays where the fight left it until
// the map reloads, so its RAM tile, not its header home, is the obstacle —
// even off-screen. A slot without a RAM tile falls back to the header home.
// hidden is state.HiddenObjectIDs, keyed by the same 1-based object ID.
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

// observedStationaryObjectBlockers returns only stationary object home tiles
// that are present in the current sprite snapshot. Unlike liveBlockers, this
// is used to change map component topology, so a hidden item or defeated
// trainer must not split the map after it has disappeared. Moving sprites are
// excluded because their positions are observations for one walking plan, not
// stable geometry for later map legs.
func observedStationaryObjectBlockers(h worldmodel.HeaderView, live []state.SpriteState) map[[2]int]bool {
	header := h.WorldMapHeader()
	blocked := map[[2]int]bool{}
	for _, sprite := range live {
		if sprite.Slot < 1 || sprite.Slot > len(header.Objects) {
			continue
		}
		o := header.Objects[sprite.Slot-1]
		if o.Movement != worldmodel.ObjectMovementStay || sprite.X != int(o.X) || sprite.Y != int(o.Y) {
			continue
		}
		blocked[[2]int{sprite.X, sprite.Y}] = true
	}
	return blocked
}

// presentStationaryObjectBlockers returns MovementStay home tiles that are
// still present on the map: not listed in the missable/hidden object set.
// Unlike observedStationaryObjectBlockers (sprite-RAM only, so off-screen
// objects vanish) and stationaryObjectBlockers (every ROM stay object,
// including already-collected balls), this is the topology set GoTo should
// overlay: undefeated trainers and remaining items split components even when
// they are outside the current sprite window (Silph Co 5F Card Key corridor).
func presentStationaryObjectBlockers(m *emu.Emu, h worldmodel.HeaderView) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return persistentTopologyBlockers(h, state.HiddenObjectIDs(&mem))
}

// persistentTopologyBlockers is the stable subset that may be remembered in
// the route graph across navigation legs. Moving objects are deliberately
// excluded even when their live sprite currently occupies a corridor tile:
// their position belongs to immediate path avoidance, not map geometry.
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

// routingBlockers is the preferred same-map topology/path set: every still-
// present stationary object plus the live sprite snapshot. Off-screen
// trainers stay in the set so component routing can leave and re-enter
// instead of planning a walk through their tile.
func routingBlockers(m *emu.Emu, h worldmodel.HeaderView) map[[2]int]bool {
	return mergeBlockers(spriteBlockers(m), presentStationaryObjectBlockers(m, h))
}

func currentObservedStationaryObjectBlockers(m *emu.Emu, h worldmodel.HeaderView) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return observedStationaryObjectBlockers(h, state.DecodeSprites(&mem))
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
// oscillating between two routes neither snapshot alone ruled out. The same
// oscillation happens when a stay trainer has left its home tile to intercept
// the player (Viridian Gym, run-1biaubd9xooqm): off-screen it vanished from
// spriteBlockers while its empty home tile was blocked instead.
func liveBlockers(m *emu.Emu, h worldmodel.HeaderView) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return mergeBlockers(spriteBlockers(m), stationaryObjectBlockers(h, state.DecodeObjectTiles(&mem), state.HiddenObjectIDs(&mem)))
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
