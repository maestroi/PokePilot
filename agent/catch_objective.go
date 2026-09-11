package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
)

func executeCatchObjective(m *emu.Emu, romData []byte, o Objective, result ObjectiveResult) (ObjectiveResult, error) {
	if o.Place != "" {
		dest, ok := skill.Place(string(o.Place))
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown catch habitat %q", o, o.Place)
		}
		var (
			travel skill.TravelResult
			err    error
		)
		if o.Flee {
			travel, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 20)
		} else {
			travel, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 20)
		}
		result.Travel = &travel
		if err != nil {
			return result, fmt.Errorf("agent: %s: travel to catch habitat: %w", o, err)
		}
	}

	species, ok := redSpeciesID(o.Species)
	if !ok {
		return result, fmt.Errorf("agent: %s: unknown Red species %q", o, o.Species)
	}
	caught, err := skill.Catch(m, romData, []uint8{species}, skill.StatAwareMove(romData), 5)
	if err != nil {
		return result, fmt.Errorf("agent: %s: %w", o, err)
	}
	if caught.Outcome == skill.OutcomeCaught {
		return result, nil
	}
	result.Outcome = OutcomeBlocked
	return result, fmt.Errorf("agent: %s: no %s caught (outcome %s, balls=%d, encounters=%d)",
		o, strings.ToUpper(string(o.Species)), catchOutcomeName(caught.Outcome), caught.BallsThrown, caught.Encounters)
}
