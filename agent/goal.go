package agent

import (
	"fmt"
	"strconv"
	"strings"
)

// GoalKind identifies a run-level success condition. Objectives are actions;
// goals are predicates over observations and are never marked complete by an
// LLM reply.
type GoalKind uint8

const (
	GoalNone GoalKind = iota
	GoalEliteFour
	GoalBadges
	GoalReach
	GoalLevel
	GoalItem
)

// Goal is a typed, observable success condition for a run.
type Goal struct {
	Kind   GoalKind
	Target string
	Count  int
}

// GoalStatus is the deterministic evaluation of a Goal against one
// observation. Summary is suitable for planner prompts and operator UIs.
type GoalStatus struct {
	Complete bool
	Summary  string
	Current  int
	Target   int
}

// ParseGoal accepts the stable structured spelling for run goals.
//
//   elite-four
//   badges:8
//   reach:cerulean city
//   level:25
//   item:potion
//
// Empty input means no deterministic goal. Arbitrary prose is deliberately
// rejected here: free-text LLMPlanner.Goal values remain valid prompt text,
// but they are not deterministic success predicates.
func ParseGoal(raw string) (Goal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Goal{}, nil
	}
	if strings.EqualFold(raw, "elite-four") || strings.EqualFold(raw, "elite four") {
		return Goal{Kind: GoalEliteFour}, nil
	}
	kind, arg, ok := strings.Cut(raw, ":")
	if !ok || strings.TrimSpace(arg) == "" {
		return Goal{}, fmt.Errorf("agent: invalid goal %q", raw)
	}
	arg = strings.TrimSpace(arg)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "badges", "badge-count":
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > 8 {
			return Goal{}, fmt.Errorf("agent: invalid badge goal %q: want 1..8", arg)
		}
		return Goal{Kind: GoalBadges, Count: n}, nil
	case "reach", "place":
		return Goal{Kind: GoalReach, Target: strings.ToLower(arg)}, nil
	case "level":
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > 100 {
			return Goal{}, fmt.Errorf("agent: invalid level goal %q: want 1..100", arg)
		}
		return Goal{Kind: GoalLevel, Count: n}, nil
	case "item":
		return Goal{Kind: GoalItem, Target: strings.ToLower(arg)}, nil
	default:
		return Goal{}, fmt.Errorf("agent: unknown goal kind %q", kind)
	}
}

// PlannerGoal parses an LLMPlanner.Goal only when it uses one of the
// structured spellings above. This is the compatibility seam between the
// existing free-text Goal prompt and deterministic completion: prose such as
// "Earn the Boulder Badge." remains prompt-only and byte-for-byte unchanged,
// while "badges:1" or "elite-four" gains an observable stop condition.
//
// The bool reports whether raw opted into the structured syntax. If it did,
// validation errors are returned instead of silently treating a malformed
// structured goal as prose.
func PlannerGoal(raw string) (Goal, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Goal{}, false, nil
	}
	if strings.EqualFold(raw, "elite-four") || strings.EqualFold(raw, "elite four") {
		g, err := ParseGoal(raw)
		return g, true, err
	}
	kind, _, ok := strings.Cut(raw, ":")
	if !ok {
		return Goal{}, false, nil
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "badges", "badge-count", "reach", "place", "level", "item":
		g, err := ParseGoal(raw)
		return g, true, err
	default:
		// Colons are legal prose. Only the documented prefixes opt in.
		return Goal{}, false, nil
	}
}

// PlannerGoalStatus evaluates a planner goal when it opted into structured
// syntax. Free-text goals return structured=false and remain prompt-only.
func PlannerGoalStatus(raw string, obs Observation) (status GoalStatus, structured bool, err error) {
	g, structured, err := PlannerGoal(raw)
	if err != nil || !structured {
		return GoalStatus{}, structured, err
	}
	return EvaluateGoal(g, obs), true, nil
}

// EvaluateGoal evaluates a run-level goal from facts already present in an
// Observation. It never interprets planner text and never mutates state.
func EvaluateGoal(g Goal, obs Observation) GoalStatus {
	switch g.Kind {
	case GoalNone:
		return GoalStatus{Summary: "no deterministic goal"}
	case GoalEliteFour:
		for _, event := range obs.Events {
			if strings.EqualFold(event, "EVENT_BEAT_CHAMPION_RIVAL") {
				return GoalStatus{Complete: true, Summary: "Elite Four and Champion defeated", Current: 1, Target: 1}
			}
		}
		return GoalStatus{Summary: fmt.Sprintf("beat the Elite Four and Champion; badges %d/8", len(obs.Badges)), Current: len(obs.Badges), Target: 8}
	case GoalBadges:
		n := len(obs.Badges)
		return GoalStatus{Complete: n >= g.Count, Summary: fmt.Sprintf("badges %d/%d", n, g.Count), Current: n, Target: g.Count}
	case GoalReach:
		complete := strings.EqualFold(strings.TrimSpace(obs.MapName), strings.TrimSpace(g.Target))
		return GoalStatus{Complete: complete, Summary: fmt.Sprintf("reach %s; currently %s", g.Target, obs.MapName), Current: boolInt(complete), Target: 1}
	case GoalLevel:
		level := 0
		for _, mon := range obs.Party {
			if int(mon.Level) > level {
				level = int(mon.Level)
			}
		}
		return GoalStatus{Complete: level >= g.Count, Summary: fmt.Sprintf("party max level %d/%d", level, g.Count), Current: level, Target: g.Count}
	case GoalItem:
		for _, item := range obs.Bag {
			if strings.EqualFold(item.Name, g.Target) && item.Quantity > 0 {
				return GoalStatus{Complete: true, Summary: fmt.Sprintf("have %s x%d", item.Name, item.Quantity), Current: item.Quantity, Target: 1}
			}
		}
		return GoalStatus{Summary: fmt.Sprintf("acquire %s", g.Target), Target: 1}
	default:
		return GoalStatus{Summary: "unknown goal"}
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
