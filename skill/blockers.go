package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// spriteBlockers snapshots the sprite RAM and returns the tiles the live map
// objects currently occupy. Keys are [2]int{X, Y}, the same key order
// walkAround and world.FindPath use. The snapshot is a fresh observation of
// where sprites ARE right now; it is not merged into or cached against
// anything.
func spriteBlockers(m *emu.Emu) map[[2]int]bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	blocked := map[[2]int]bool{}
	for _, s := range state.DecodeSprites(&mem) {
		blocked[[2]int{s.X, s.Y}] = true
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
