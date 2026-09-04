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
	if next.MaxLevel > m.MaxLevel {
		m.MaxLevel = next.MaxLevel
		advanced = true
	}
	return advanced
}

func (m majorProgressMark) String() string {
	return fmt.Sprintf("%d badge(s), %d event(s), %d map(s), party %d, max level %d",
		m.Badges, m.Events, m.Maps, m.PartyCount, m.MaxLevel)
}
