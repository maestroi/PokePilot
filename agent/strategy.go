package agent

import (
	"fmt"
	"sort"
	"strings"
)

const (
	strategyHypothesisCap = 6
	strategyBlockerCap    = 6
)

// StrategicMemory is planner-derived memory kept explicitly separate from
// Knowledge. Knowledge is game truth; entries here are hypotheses and plans
// that may be wrong and must never be treated as decoded state.
type StrategicMemory struct {
	Strategy   string
	Age        int
	Hypotheses []string
	Blockers   []string
	NoProgress int
	last       progressMark
}

// StrategyView is the bounded prompt-facing form of StrategicMemory.
type StrategyView struct {
	Strategy   string   `json:"strategy,omitempty"`
	Age        int      `json:"age,omitempty"`
	Hypotheses []string `json:"hypotheses,omitempty"`
	Blockers   []string `json:"blockers,omitempty"`
	NoProgress int      `json:"no_progress_rounds,omitempty"`
}

type progressMark struct {
	badges int
	events int
	maps   int
	level  int
	mapID  uint8
	set    bool
}

// SetStrategy records the planner's current higher-level approach. Repeating
// the same strategy ages it; changing it resets the age. Run code should only
// call this with text supplied by the planner.
func (m *StrategicMemory) SetStrategy(s string) {
	s = strings.TrimSpace(s)
	if s == m.Strategy {
		if s != "" {
			m.Age++
		}
		return
	}
	m.Strategy = s
	m.Age = 0
}

// AddHypothesis remembers a planner theory without promoting it to game
// truth. Newest entries are first, duplicates move to the front.
func (m *StrategicMemory) AddHypothesis(s string) {
	m.Hypotheses = pushBounded(m.Hypotheses, s, strategyHypothesisCap)
}

// AddBlocker remembers a planner-described blocker. The game-derived
// Requirements field remains the authoritative record of what was actually
// said; this is only the planner's interpretation.
func (m *StrategicMemory) AddBlocker(s string) {
	m.Blockers = pushBounded(m.Blockers, s, strategyBlockerCap)
}

func pushBounded(in []string, s string, capN int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return in
	}
	out := make([]string, 0, capN)
	out = append(out, s)
	for _, old := range in {
		if old != s && len(out) < capN {
			out = append(out, old)
		}
	}
	return out
}

// ObserveProgress updates the long-horizon no-progress counter using only
// facts already visible to the planner. A map change alone counts as local
// movement; badges/events/visited-map growth and party levels are stronger
// progress but require no special weighting here.
func (m *StrategicMemory) ObserveProgress(obs Observation, visitedMaps int) bool {
	mark := progressMark{
		badges: len(obs.Badges),
		events: len(obs.Events),
		maps:   visitedMaps,
		level:  maxPartyLevel(obs),
		mapID:  obs.Map,
		set:    true,
	}
	if !m.last.set {
		m.last = mark
		m.NoProgress = 0
		return true
	}
	progressed := mark.badges > m.last.badges || mark.events > m.last.events || mark.maps > m.last.maps || mark.level > m.last.level || mark.mapID != m.last.mapID
	if progressed {
		m.NoProgress = 0
	} else {
		m.NoProgress++
	}
	m.last = mark
	return progressed
}

func maxPartyLevel(obs Observation) int {
	level := 0
	for _, mon := range obs.Party {
		if int(mon.Level) > level {
			level = int(mon.Level)
		}
	}
	return level
}

// ReplanReason returns a deterministic reason to ask for a materially new
// strategy. It does not choose that strategy.
func (m StrategicMemory) ReplanReason(after int) string {
	if after <= 0 || m.NoProgress < after {
		return ""
	}
	if m.Strategy == "" {
		return fmt.Sprintf("no measurable progress for %d rounds; choose a strategy", m.NoProgress)
	}
	return fmt.Sprintf("strategy %q made no measurable progress for %d rounds; choose a materially different approach", m.Strategy, m.NoProgress)
}

// View returns copies so a prompt or UI cannot mutate run memory.
func (m StrategicMemory) View() StrategyView {
	return StrategyView{
		Strategy:   m.Strategy,
		Age:        m.Age,
		Hypotheses: append([]string(nil), m.Hypotheses...),
		Blockers:   append([]string(nil), m.Blockers...),
		NoProgress: m.NoProgress,
	}
}

// StableStrategicFacts returns a deterministic rendering useful in tests,
// logs, and prompt snapshots.
func (m StrategicMemory) StableStrategicFacts() []string {
	var out []string
	for _, h := range m.Hypotheses {
		out = append(out, "hypothesis: "+h)
	}
	for _, b := range m.Blockers {
		out = append(out, "blocker: "+b)
	}
	sort.Strings(out)
	return out
}
