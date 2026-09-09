package agent

import (
	"fmt"
	"strings"
)

// StrategicMemory is derived run state used to detect long-horizon stalls.
// It deliberately stores no planner-authored strategy text. Observation.Intent
// remains chooser-authored context; the persistent multi-step Plan is owned and
// advanced separately by Run.
//
// NoProgress is the strict stall counter: badges, events, party levels and
// genuinely new local maps all reset it. NoWorldProgress is the slower
// strategic backstop: level gains do NOT reset it. That distinction matters
// for grinding. Training can be real measurable progress and still spend many
// rounds without opening any new part of the game. After the same threshold,
// ReplanReason asks the planner to re-evaluate that approach; it does not force
// the planner to stop training or prescribe a route.
//
// seenMaps was previously a fixed 4-entry rolling window (ponytail: "a cycle
// longer than this still reads as progress"). MEASURED 2026-09-09 on a live
// run: 48 rounds cycling Viridian/Route 2/Route 1/Pallet Town/Reds
// House/Oak's Lab (six maps) at 0/8 badges, and replan_reasons never once
// showed stagnation or stuck — the window was smaller than the cycle, so
// every re-entry read as fresh exploration and the recovery escalation
// never fired. A map, once seen this run, must never count as progress
// again regardless of how much has happened since; the map domain is a
// single byte, so tracking every map ever seen costs nothing worth
// bounding.
type StrategicMemory struct {
	NoProgress      int
	NoWorldProgress int
	last            progressMark
	seenMaps        map[uint8]bool
}

type progressMark struct {
	badges int
	events int
	level  int
	set    bool
}

// ObserveProgress updates both long-horizon counters using only facts already
// visible to the planner. Badge, event and party-level growth are measurable
// progress. World progress is deliberately narrower: badge/event growth or a
// map never stood on before this run. Local movement counts only the first
// time a map is entered, so any cycle through already-seen maps — A->B->A->B
// or a longer loop through six maps — cannot reset either strategic counter
// forever.
func (m *StrategicMemory) ObserveProgress(obs Observation) bool {
	mark := progressMark{
		badges: len(obs.Badges),
		events: len(obs.Events),
		level:  maxPartyLevel(obs),
		set:    true,
	}
	mapProgress := !m.seenMaps[obs.Map]
	if m.seenMaps == nil {
		m.seenMaps = map[uint8]bool{}
	}
	m.seenMaps[obs.Map] = true

	if !m.last.set {
		m.last = mark
		m.NoProgress = 0
		m.NoWorldProgress = 0
		return true
	}

	worldProgressed := mark.badges > m.last.badges || mark.events > m.last.events || mapProgress
	progressed := worldProgressed || mark.level > m.last.level
	if progressed {
		m.NoProgress = 0
	} else {
		m.NoProgress++
	}
	if worldProgressed {
		m.NoWorldProgress = 0
	} else {
		m.NoWorldProgress++
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
//
// A strict no-progress stall wins because it is the stronger diagnosis. The
// world-progress branch catches the subtler case where levels keep increasing
// while badges, events and explored maps do not. Its wording explicitly leaves
// room for intentional preparation: this is a prompt to re-evaluate, not a
// deterministic decision that grinding is wrong.
func (m StrategicMemory) ReplanReason(after int, intent string) string {
	if after <= 0 {
		return ""
	}
	intent = strings.TrimSpace(intent)
	if m.NoProgress >= after {
		if intent == "" {
			return fmt.Sprintf("No measurable progress for %d rounds; choose a materially different approach.", m.NoProgress)
		}
		return fmt.Sprintf("Intent %q has made no measurable progress for %d rounds; choose a materially different approach.", intent, m.NoProgress)
	}
	if m.NoWorldProgress < after {
		return ""
	}
	if intent == "" {
		return fmt.Sprintf("No new badge, event, or map progress for %d rounds. Level gains may be useful preparation, but re-evaluate whether this approach still serves the goal.", m.NoWorldProgress)
	}
	return fmt.Sprintf("Intent %q has made no new badge, event, or map progress for %d rounds. Level gains may be useful preparation, but re-evaluate whether this approach still serves the goal.", intent, m.NoWorldProgress)
}
