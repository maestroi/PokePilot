package agent

import "fmt"

// defaultStagnationAfter is how many completed rounds may pass without
// meaningful, monotonic game progress before Run stops with StopStuck.
// It is deliberately much larger than defaultStuckAfter: the short detector
// catches an objective that literally changes nothing, while this one catches
// long loops that keep moving (for example, walking between already-known
// places) without advancing through the game.
const defaultStagnationAfter = 200

// roundCapReached says whether round is beyond an explicitly configured cap.
// Zero is the normal goal-driven mode and therefore never reaches a round cap.
func roundCapReached(round, maxRounds int) bool {
	return maxRounds > 0 && round > maxRounds
}

// roundsLeft is planner telemetry for an optional cap. Zero is deliberately
// reserved for "uncapped"; positive values include the current round.
func roundsLeft(round, maxRounds int) int {
	if maxRounds <= 0 {
		return 0
	}
	return maxRounds - round + 1
}

// majorProgressMark is the monotonic subset of game state used by the long
// stagnation watchdog. Position, HP, money and consumables are intentionally
// absent: they can churn forever without saying the run got farther.
//
// Maps comes from Knowledge.Visited, so traversing a genuinely new area is
// progress even when an objective enters and leaves it within one round.
// CombatReadiness makes productive training count without making damage/healing
// a false reset. It uses the same strongest-three weighted score as combat
// recovery, so leveling a useful secondary party member is visible even when
// the party's maximum level does not change (#1850). PartyCount makes catching
// a new party member count. DexOwned also counts successful collection when the
// party is already full and the newly caught species is sent to storage, which
// is essential progress for Dex and Completionist runs.
type majorProgressMark struct {
	Badges          int
	Events          int
	Maps            int
	PartyCount      int
	DexOwned        int
	MaxLevel        uint8
	CombatReadiness int
}

func majorProgressMarkOf(obs Observation, k *Knowledge) majorProgressMark {
	mark := majorProgressMark{
		Badges:          len(obs.Badges),
		Events:          len(obs.Events),
		PartyCount:      obs.PartyCount,
		DexOwned:        len(obs.PokedexOwned),
		CombatReadiness: partyCombatReadiness(obs),
	}
	if k != nil {
		mark.Maps = len(k.Visited)
	}
	for _, mon := range obs.Party {
		if mon.Level > mark.MaxLevel {
			mark.MaxLevel = mon.Level
		}
	}
	return mark
}

// absorb folds next into the historical high-water mark and reports whether
// any dimension moved forward. Keeping maxima matters when one dimension can
// regress: discovering a map while the party is temporarily weaker must not
// lower the remembered level and let merely recovering that old level count
// as fresh progress later.
func (m *majorProgressMark) absorb(next majorProgressMark) bool {
	advanced := false
	if next.Badges > m.Badges {
		m.Badges = next.Badges
		advanced = true
	}
	if next.Events > m.Events {
		m.Events = next.Events
		advanced = true
	}
	if next.Maps > m.Maps {
		m.Maps = next.Maps
		advanced = true
	}
	if next.PartyCount > m.PartyCount {
		m.PartyCount = next.PartyCount
		advanced = true
	}
	if next.DexOwned > m.DexOwned {
		m.DexOwned = next.DexOwned
		advanced = true
	}
	if next.MaxLevel > m.MaxLevel {
		m.MaxLevel = next.MaxLevel
		advanced = true
	}
	if next.CombatReadiness > m.CombatReadiness {
		m.CombatReadiness = next.CombatReadiness
		advanced = true
	}
	return advanced
}

func (m majorProgressMark) String() string {
	return fmt.Sprintf("%d badge(s), %d event(s), %d map(s), party %d, dex owned %d, max level %d, combat readiness %d",
		m.Badges, m.Events, m.Maps, m.PartyCount, m.DexOwned, m.MaxLevel, m.CombatReadiness)
}
