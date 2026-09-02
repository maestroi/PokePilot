package agent

import (
	"fmt"
	"strings"
)

// StrategicMemory is derived run state used to detect long-horizon stalls.
// It deliberately stores no planner-authored strategy text: Observation.Intent
// and IntentAge are the single planner-owned strategic memory carried by Run.
type StrategicMemory struct {
	NoProgress int
	last       progressMark
}

type progressMark struct {
	badges int
	events int
	maps   int
	level  int
	mapID  uint8
	set    bool
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
// approach once observable progress has stalled. intent is the planner-owned
// Observation.Intent carried by Run; this type never stores a second copy.
// It diagnoses the stall but does not choose the replacement approach.
func (m StrategicMemory) ReplanReason(after int, intent string) string {
	if after <= 0 || m.NoProgress < after {
		return ""
	}
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return fmt.Sprintf("No measurable progress for %d rounds; choose a materially different approach.", m.NoProgress)
	}
	return fmt.Sprintf("Intent %q has made no measurable progress for %d rounds; choose a materially different approach.", intent, m.NoProgress)
}
