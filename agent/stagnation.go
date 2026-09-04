package agent

import "fmt"

// defaultStagnationAfter is how many completed rounds may pass without
// meaningful, monotonic game progress before Run stops with StopStuck.
// It is deliberately much larger than defaultStuckAfter: the short detector
// catches an objective that literally changes nothing, while this one catches
// long loops that keep moving (for example, walking between already-known
// places) without advancing through the game.
const defaultStagnationAfter = 200

// majorProgressMark is the monotonic subset of game state used by the long
// stagnation watchdog. Position, HP, money and consumables are intentionally
// absent: they can churn forever without saying the run got farther.
//
// Maps comes from Knowledge.Visited, so traversing a genuinely new area is
// progress even when an objective enters and leaves it within one round.
// MaxLevel makes productive training count without making damage/healing a
// false reset. PartyCount makes catching a new party member count.
type majorProgressMark struct {
	Badges     int
	Events     int
	Maps       int
	PartyCount int
	MaxLevel   uint8
}

func majorProgressMarkOf(obs Observation, k *Knowledge) majorProgressMark {
	mark := majorProgressMark{
		Badges:     len(obs.Badges),
		Events:     len(obs.Events),
		PartyCount: obs.PartyCount,
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

// advancedBeyond reports real forward progress. It is intentionally
// monotonic: a blackout losing money, a party member taking damage, or any
// other regression must not reset the watchdog.
func (m majorProgressMark) advancedBeyond(prev majorProgressMark) bool {
	return m.Badges > prev.Badges ||
		m.Events > prev.Events ||
		m.Maps > prev.Maps ||
		m.PartyCount > prev.PartyCount ||
		m.MaxLevel > prev.MaxLevel
}

func (m majorProgressMark) String() string {
	return fmt.Sprintf("%d badge(s), %d event(s), %d map(s), party %d, max level %d",
		m.Badges, m.Events, m.Maps, m.PartyCount, m.MaxLevel)
}
