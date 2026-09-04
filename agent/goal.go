package agent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/red/state"
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
//	elite-four
//	badges:8
//	reach:cerulean city
//	level:25
//	item:potion
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

// plannerGoalPreset recognizes the finite human-readable goals exposed by the
// operator UI. They remain natural language in the LLM prompt, but get the
// same observable stop predicate as their structured equivalent. This is a
// deliberately small compatibility table, not a natural-language parser: an
// arbitrary sentence still stays prompt-only until a real goal resolver is
// added.
func plannerGoalPreset(raw string) (Goal, bool) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.TrimSpace(strings.TrimSuffix(normalized, "."))
	switch normalized {
	case "earn the boulder badge", "earn 1 badge", "earn one badge":
		return Goal{Kind: GoalBadges, Count: 1}, true
	case "earn 2 badges":
		return Goal{Kind: GoalBadges, Count: 2}, true
	case "earn 3 badges":
		return Goal{Kind: GoalBadges, Count: 3}, true
	case "earn 4 badges":
		return Goal{Kind: GoalBadges, Count: 4}, true
	case "earn 5 badges":
		return Goal{Kind: GoalBadges, Count: 5}, true
	case "earn 6 badges":
		return Goal{Kind: GoalBadges, Count: 6}, true
	case "earn 7 badges":
		return Goal{Kind: GoalBadges, Count: 7}, true
	case "earn 8 badges", "earn all 8 badges":
		return Goal{Kind: GoalBadges, Count: 8}, true
	case "beat the elite four and champion", "beat the elite four and the champion":
		return Goal{Kind: GoalEliteFour}, true
	default:
		return Goal{}, false
	}
}

// PlannerGoal parses an LLMPlanner.Goal when it uses structured syntax or one
// of the finite human-readable presets above. Presets keep the task sentence
// natural for the model while still gaining an observable stop condition.
// Arbitrary prose remains prompt-only and byte-for-byte unchanged.
//
// The bool reports whether raw opted into a deterministic goal. Structured
// validation errors are returned instead of silently treating malformed
// structured syntax as prose.
func PlannerGoal(raw string) (Goal, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Goal{}, false, nil
	}
	if g, ok := plannerGoalPreset(raw); ok {
		return g, true, nil
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

// PlannerGoalStatus evaluates a planner goal when it opted into deterministic
// syntax or a known preset. Other free-text goals remain prompt-only.
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
		// state.Event.String(), not the raw decomp label: Observe fills
		// Observation.Events with the former ("BeatChampionRival"), so a
		// hardcoded "EVENT_BEAT_CHAMPION_RIVAL" could never match. offer.go
		// reads event names the same way.
		for _, event := range obs.Events {
			if strings.EqualFold(event, state.EventBeatChampionRival.String()) {
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