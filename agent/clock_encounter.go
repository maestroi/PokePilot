package agent

import "github.com/maestroi/pokepilot/game"

// ClockEncounter is one clock-gated species on the current map. Open is true
// when it can be met at the observed time of day. Until is the wait leg when
// it cannot. Species that are not clock-gated are omitted, and an adapter
// with no clock leaves the slice empty.
type ClockEncounter struct {
	Species  string `json:"species"`
	Open     bool   `json:"open"`
	Until    string `json:"until,omitempty"`
	MinLevel uint8  `json:"min_level,omitempty"`
	MaxLevel uint8  `json:"max_level,omitempty"`
}

// ClockDecision is the prompt-facing list of clock legs. The full open/closed
// set stays on Observation.ClockEncounters.
type ClockDecision struct {
	Waits []ClockWait `json:",omitempty"`
}

// ClockWait is one species that becomes reachable only after the clock reaches Until.
type ClockWait struct {
	Species string
	Until   string
}

func projectClockEncounters(now game.TimeOfDay, slots []game.TimedSlot) []ClockEncounter {
	plans := game.PlanSpeciesClocks(now, slots)
	if len(plans) == 0 {
		return nil
	}
	out := make([]ClockEncounter, 0, len(plans))
	for _, plan := range plans {
		item := ClockEncounter{
			Species:  string(plan.Species),
			Open:     plan.Open,
			MinLevel: plan.MinLevel,
			MaxLevel: plan.MaxLevel,
		}
		if plan.Wait != nil {
			item.Until = string(plan.Wait.Until)
		}
		out = append(out, item)
	}
	return out
}

func clockWaits(encounters []ClockEncounter) []ClockWait {
	var waits []ClockWait
	for _, enc := range encounters {
		if enc.Open || enc.Until == "" {
			continue
		}
		waits = append(waits, ClockWait{Species: enc.Species, Until: enc.Until})
	}
	return waits
}
