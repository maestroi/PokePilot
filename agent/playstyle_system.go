package agent

import "strings"

// PlayStyleSystemNote turns an explicitly selected gameplay profile into a
// compact planner instruction. Empty stays empty so legacy specs keep their
// historical prompt byte-for-byte; product surfaces that explicitly choose
// Speedrun still get the Speedrun instruction.
//
// The note only changes prioritization. Deterministic code remains the source
// of truth for legality and the model may still choose only offered objectives.
func PlayStyleSystemNote(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}

	switch NormalizePlayStyle(name) {
	case PlayStyleAdventure:
		return "PLAY STYLE: ADVENTURE. Play naturally: combine required progression with nearby exploration, useful items, unseen NPC interactions, party development, and sensible recovery. Prefer novel low-cost detours when they fit the current situation, but do not wander or repeat completed content indefinitely. Only choose objectives that are actually offered."
	case PlayStyleCompletionist:
		return "PLAY STYLE: COMPLETIONIST. Maximize durable completion progress before advancing when it is legal and reasonably reachable. Treat every offered catch for a species not yet owned in the Pokédex as high-priority collection progress regardless of battle usefulness or party fullness; boxed catches still count, and Seen is not the same as Owned. Keep capture stock healthy and prefer Mart or ball-resupply objectives when balls are low and obtainable Pokédex targets remain. Explore unvisited areas, talk to unseen NPCs, collect reachable items, and challenge unbeaten trainers before main progression when the detour is reasonable. Prefer new coverage over repeated grinding or repeatedly failing content. The explicit run goal still determines when the run ends. Only choose objectives that are actually offered."
	case PlayStyleTeamBuilder:
		return "PLAY STYLE: TEAM BUILDER. Prioritize building and developing a capable party: useful catches, catch-up training, resources, moves and items, preparation, and recovery. Progress when it unlocks better team options or current development is sufficient; avoid optional detours that do not help the party. Only choose objectives that are actually offered."
	default:
		return "PLAY STYLE: SPEEDRUN. Prefer direct required progression and efficient preparation. Avoid optional exploration, collection, training, and interaction unless they unblock progression, materially improve success odds, or are needed for recovery. Only choose objectives that are actually offered."
	}
}
