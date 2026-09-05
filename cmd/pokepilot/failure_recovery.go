package main

import (
	"strings"

	"github.com/maestroi/pokepilot/agent"
)

const farmFailureCooldownRounds = 2

// farmRecoveryOffered keeps an exact objective that just failed out of the
// farm planner's menu for a short cooldown when alternatives exist. This is
// deliberately run-local and temporary: it prevents the current
// same-objective/same-error guard from killing an otherwise healthy run on an
// incidental skill edge case, without teaching permanent world geometry.
//
// If filtering would remove every option, the full menu is returned. A
// mandatory progression action therefore remains retryable; repeated failure
// of that frontier can still end as failed/stuck and be classified as a
// progression blocker by the failure telemetry.
func farmRecoveryOffered(obs agent.Observation, offered []agent.Objective) []agent.Objective {
	if len(offered) <= 1 || len(obs.History) == 0 {
		return offered
	}
	blocked := map[string]bool{}
	seen := 0
	for i := len(obs.History) - 1; i >= 0 && seen < farmFailureCooldownRounds; i-- {
		r := obs.History[i]
		seen++
		if strings.HasPrefix(r.Outcome, "failed: ") {
			blocked[r.Objective] = true
		}
	}
	if len(blocked) == 0 {
		return offered
	}
	out := make([]agent.Objective, 0, len(offered))
	for _, o := range offered {
		if !blocked[o.String()] {
			out = append(out, o)
		}
	}
	if len(out) == 0 {
		return offered
	}
	return out
}
