package main

import (
	"strings"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/red/state"
)

const farmFailureCooldownRounds = 2

const (
	farmGymChallengeNote      = "(gym challenge: this action approaches, talks to, and battles the leader)"
	farmUnvisitedAdjacentNote = "unvisited adjacent map"
)

// farmHasBadge is the farm wrapper's small live-observation helper. Badge
// names are already planner-visible strings, so steering must use the same
// observed facts rather than infer progress from intent/history.
func farmHasBadge(obs agent.Observation, badge state.Badge) bool {
	want := badge.String()
	for _, got := range obs.Badges {
		if got == want {
			return true
		}
	}
	return false
}

// farmSeekingCascade is deliberately narrow: the live farm regression we are
// fixing is the bridge from the first badge to the second. Before Boulder the
// existing Brock/training policy owns readiness; after Cascade later story
// slices have their own explicit progression verbs. Keeping this bounded
// avoids turning a general LLM run into a hard-coded walkthrough.
func farmSeekingCascade(obs agent.Observation) bool {
	return farmHasBadge(obs, state.BadgeBoulder) && !farmHasBadge(obs, state.BadgeCascade)
}

func farmFrontierJourney(o agent.Objective) bool {
	return o.Kind == agent.KindGoTo && strings.Contains(strings.ToLower(o.Note), farmUnvisitedAdjacentNote)
}

// farmNeedsTrainingRecovery says the run has factual combat evidence that a
// stronger party is needed before retrying a mandatory fight. Optional
// training is suppressed during the Boulder->Cascade bridge, but this evidence
// keeps the recovery rung available after a trainer/gym loss.
func farmNeedsTrainingRecovery(obs agent.Observation) bool {
	for _, f := range obs.Failures {
		text := strings.ToLower(f.Objective + " " + f.Last)
		if strings.Contains(text, "trainer loss") ||
			strings.Contains(text, "lost to the gym leader") ||
			strings.Contains(text, "retry is due") {
			return true
		}
	}
	return false
}

// farmProgressionCritical reports objectives that represent an explicit story
// frontier rather than an optional/repeatable action. These stay visible even
// when the exact objective failed recently: hiding one merely because other
// legal actions exist can make the farm wander away from a mandatory frontier
// (observed in Pewter: the run reached Brock, then the gym challenge vanished
// behind the generic two-round cooldown and the planner left without talking
// to him).
//
// A directly adjacent unvisited map is also a frontier in farm mode. If that
// leg itself is flaky, keeping it visible lets telemetry prove a real blocker
// instead of the recovery wrapper silently steering away from the only novel
// exit.
func farmProgressionCritical(o agent.Objective) bool {
	if farmFrontierJourney(o) {
		return true
	}
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

// farmPreferActiveGym prevents another "arrive at the leader, then wander
// away" loop. The first challenge is intentionally attempted without judging
// level. If it loses, agent's existing gym-loss recovery removes the challenge
// until one Train rung succeeds, so forcing the offered challenge here does not
// create an endless under-levelled retry loop.
func farmPreferActiveGym(offered []agent.Objective) []agent.Objective {
	hasGym := false
	for _, o := range offered {
		if o.Kind == agent.KindGym {
			hasGym = true
			break
		}
	}
	if !hasGym {
		return offered
	}

	out := make([]agent.Objective, 0, len(offered))
	for _, o := range offered {
		switch o.Kind {
		case agent.KindGym, agent.KindHeal, agent.KindUseItem:
			out = append(out, o)
		}
	}
	if len(out) == 0 {
		return offered
	}
	return out
}

// farmPreferFirstBadgeFrontier addresses the live post-Brock drift without
// scripting Mt. Moon coordinates or a fixed route. Between Boulder and
// Cascade, optional catching/grinding must not outrank reaching new map
// frontiers. When Offer exposes an unvisited adjacent map, old-map GoTo choices
// are temporarily removed as well, so reaching Pewter/Route 3/Route 4 cannot
// immediately bounce all the way back to Viridian Forest.
//
// One exception is factual combat recovery: a recorded mandatory trainer/gym
// loss keeps Train available until the materially stronger party can retry.
// Recovery verbs remain available throughout. The cleared Pewter Gym interior
// is also removed from this stage; the screenshot that motivated this change
// showed it picked eleven times after the Boulder Badge was already present.
func farmPreferFirstBadgeFrontier(obs agent.Observation, offered []agent.Objective) []agent.Objective {
	if !farmSeekingCascade(obs) {
		return offered
	}

	hasFrontier := false
	for _, o := range offered {
		if farmFrontierJourney(o) {
			hasFrontier = true
			break
		}
	}
	needTrain := farmNeedsTrainingRecovery(obs)

	out := make([]agent.Objective, 0, len(offered))
	for _, o := range offered {
		if o.Kind == agent.KindGoTo && strings.EqualFold(o.Place, "pewter gym") {
			continue // Boulder is live; returning to the cleared dead-end is not progression.
		}
		if o.Kind == agent.KindCatch {
			continue // optional collection can resume after the next badge.
		}
		if o.Kind == agent.KindTrain && !needTrain {
			continue // grind only when a real mandatory combat loss demonstrated the need.
		}
		if hasFrontier && o.Kind == agent.KindGoTo && !farmFrontierJourney(o) {
			continue // hold forward momentum while a novel adjacent map is reachable.
		}
		out = append(out, o)
	}
	if len(out) == 0 {
		return offered
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
	offered = farmPreferActiveGym(offered)
	offered = farmPreferFirstBadgeFrontier(obs, offered)
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
