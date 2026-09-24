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
		return "PLAY STYLE: ADVENTURE. Play naturally: combine required progression with nearby exploration, useful items, unseen NPC interactions, party development, and sensible recovery. Prefer unbeaten trainer battles when they are reasonably nearby: they are finite, reliable XP and also earn money for useful purchases, so prefer them over repeated wild grinding when training is useful. Prefer novel low-cost detours when they fit the current situation, but do not wander or repeat completed content indefinitely. Only choose objectives that are actually offered."
	case PlayStyleCompletionist:
		return "PLAY STYLE: COMPLETIONIST. Play like a thorough player: prefer meaningful unfinished optional content when there is no pressing safety or progression reason. Visit useful side routes and unvisited areas, challenge unbeaten optional trainers, talk to relevant unseen NPCs, collect reachable items/TMs/HMs, use worthwhile upgrades, catch new species when practical, and pursue offered evolutions. Prefer breadth and durable progress over repeated grinding or repeating completed content, but do not perform low-value interactions solely to increase test coverage. Continue story progression when it unlocks worthwhile optional content, improves safety, or advances the explicit run goal. The run goal is independent from this play style and alone determines when the run ends. Only choose objectives that are actually offered."
	case PlayStyleTeamBuilder:
		return "PLAY STYLE: TEAM BUILDER. Prioritize building and developing a full six-Pokémon party and make all six battle-ready for the run goal. A roster of 3, 4, or 5 is still incomplete while useful legal catch objectives exist: prefer filling empty slots over repeatedly polishing the same small core. Among similarly reachable catches, prefer strong complementary choices with good encounter levels, useful type/role coverage, and evolution upside rather than arbitrary filler. Keep enough capture stock to recruit the roster. Prefer unbeaten trainer battles for catch-up XP whenever practical: trainer XP is finite and reliable, and the prize money funds balls, healing stock, and other useful preparation, so use trainers before repeated wild grinding when either is available at similar cost. Once six members are present, prioritize catch-up training, moves, durable items, preparation, and recovery for the weakest members so the whole team develops instead of only the lead or top three. Progress when it unlocks materially better team options or is needed for the run goal; avoid optional detours that do not improve the team. Only choose objectives that are actually offered."
	default:
		return "PLAY STYLE: SPEEDRUN. Prefer direct required progression and efficient preparation. Avoid optional exploration, collection, training, and interaction unless they unblock progression, materially improve success odds, or are needed for recovery. When offered bounded REPEL supply/use objectives on encounter-heavy travel, prefer them when they avoid unnecessary wild-battle transitions; do not use them when the run is intentionally fighting wild encounters. An on-route unbeaten trainer can be worthwhile when its XP or prize money materially reduces later grinding or enables required preparation, but do not detour just to clear trainers. Only choose objectives that are actually offered."
	}
}
