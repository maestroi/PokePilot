package main

import (
	"strings"

	"github.com/maestroi/pokepilot/agent"
)

const farmFailureCooldownRounds = 2

const farmGymChallengeNote = "(gym challenge: this action approaches, talks to, and battles the leader)"

// farmProgressionCritical reports objectives that represent an explicit story
// frontier rather than an optional/repeatable action. These stay visible even
// when the exact objective failed recently: hiding one merely because other
// legal actions exist can make the farm wander away from a mandatory frontier
// (observed in Pewter: the run reached Brock, then the gym challenge vanished
// behind the generic two-round cooldown and the planner left without talking
// to him).
//
// A repeated technical failure is still bounded by agent.Run and still reaches
// failure telemetry. Keeping the verb visible does not turn failures into
// success; it only prevents the farm recovery wrapper from accidentally
// removing the one action that can advance the story.
func farmProgressionCritical(o agent.Objective) bool {
	switch o.Kind {
	case agent.KindGym,
		agent.KindErrand,
		agent.KindRocketHideout,
		agent.KindPokemonTower,
		agent.KindFuchsiaProgression:
		return true
	default:
		return false
	}
}

// clarifyFarmProgression makes the atomic gym verb explicit on the exact menu
// line the LLM chooses. `go to pewter gym` and `beat the gym leader here` can
// otherwise look like two halves of the same action, and small planners have
// been observed to do only the first half. KindGym already owns the approach,
// leader dialogue and battle in skill.Gym; this note exposes that existing
// contract without changing objective identity or hard-coding Brock.
func clarifyFarmProgression(offered []agent.Objective) []agent.Objective {
	out := append([]agent.Objective(nil), offered...)
	for i := range out {
		if out[i].Kind != agent.KindGym || strings.Contains(out[i].Note, farmGymChallengeNote) {
			continue
		}
		if out[i].Note == "" {
			out[i].Note = farmGymChallengeNote
		} else {
			out[i].Note += " " + farmGymChallengeNote
		}
	}
	return out
}

// farmRecoveryOffered keeps an exact objective that just failed out of the
// farm planner's menu for a short cooldown when alternatives exist. This is
// deliberately run-local and temporary: it prevents the current
// same-objective/same-error guard from killing an otherwise healthy run on an
// incidental skill edge case, without teaching permanent world geometry.
//
// Progression-critical verbs are exempt from quarantine even when alternatives
// exist. Their failures are useful telemetry and may still prove a genuine
// blocker, but silently hiding them can itself create a progression failure.
// If filtering would remove every remaining option, the full menu is returned.
func farmRecoveryOffered(obs agent.Observation, offered []agent.Objective) []agent.Objective {
	offered = clarifyFarmProgression(offered)
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
		if blocked[o.String()] && !farmProgressionCritical(o) {
			continue
		}
		out = append(out, o)
	}
	if len(out) == 0 {
		return offered
	}
	return out
}
