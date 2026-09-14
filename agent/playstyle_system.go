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
		return "PLAY STYLE: TEAM BUILDER. Build a full six-Pokémon party and make all six battle-ready for the run goal. A roster of 3, 4, or 5 is still incomplete while useful legal catch objectives exist: prefer filling empty slots over repeatedly polishing the same small core. Among similarly reachable catches, prefer strong complementary choices with good encounter levels, useful type/role coverage, and evolution upside rather than arbitrary filler. Keep enough capture stock to recruit the roster. Once six members are present, prioritize catch-up training, moves, durable items, preparation, and recovery for the weakest members so the whole team develops instead of only the lead or top three. Progress when it unlocks materially better team options or is needed for the run goal; avoid optional detours that do not improve the team. Only choose objectives that are actually offered."
	default:
		return "PLAY STYLE: SPEEDRUN. Prefer direct required progression and efficient preparation. Avoid optional exploration, collection, training, and interaction unless they unblock progression, materially improve success odds, or are needed for recovery. Only choose objectives that are actually offered."
	}
}
