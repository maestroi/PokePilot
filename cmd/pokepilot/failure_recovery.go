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

func farmHasBadge(obs agent.Observation, badge state.Badge) bool {
	want := badge.String()
	for _, got := range obs.Badges {
		if got == want {
			return true
		}
	}
	return false
}

func farmSeekingCascade(obs agent.Observation) bool {
	return farmHasBadge(obs, state.BadgeBoulder) && !farmHasBadge(obs, state.BadgeCascade)
}

func farmFrontierJourney(o agent.Objective) bool {
	return o.Kind == agent.KindGoTo && strings.Contains(strings.ToLower(o.Note), farmUnvisitedAdjacentNote)
}

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

// Progression criticality is semantic: farm policy never needs to know which
// concrete Red story slice a progression objective represents.
func farmProgressionCritical(o agent.Objective) bool {
	if farmFrontierJourney(o) {
		return true
	}
	switch o.Kind {
	case agent.KindGym, agent.KindProgress:
		return true
	default:
		return false
	}
}

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
			continue
		}
		if o.Kind == agent.KindCatch {
			continue
		}
		if o.Kind == agent.KindTrain && !needTrain {
			continue
		}
		if hasFrontier && o.Kind == agent.KindGoTo && !farmFrontierJourney(o) {
			continue
		}
		out = append(out, o)
	}
	if len(out) == 0 {
		return offered
	}
	return out
}

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
