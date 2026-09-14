package agent

import (
	"fmt"
	"strconv"
	"strings"
)

type GoalKind uint8

const (
	GoalNone GoalKind = iota
	GoalEliteFour
	GoalBadges
	GoalReach
	GoalLevel
	GoalItem
	GoalDex
)

type Goal struct {
	Kind   GoalKind
	Target string
	Count  int
}

type GoalStatus struct {
	Complete bool
	Summary  string
	Current  int
	Target   int
}

func ParseGoal(raw string) (Goal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Goal{}, nil
	}
	if strings.EqualFold(raw, "elite-four") || strings.EqualFold(raw, "elite four") {
		return Goal{Kind: GoalEliteFour}, nil
	}
	if strings.EqualFold(raw, "dex") || strings.EqualFold(raw, "pokedex") || strings.EqualFold(raw, "pokédex") {
		return Goal{Kind: GoalDex}, nil
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
	case "complete the obtainable pokedex", "complete the obtainable pokédex", "complete the pokedex", "complete the pokédex":
		return Goal{Kind: GoalDex}, true
	default:
		return Goal{}, false
	}
}

func PlannerGoal(raw string) (Goal, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Goal{}, false, nil
	}
	if g, ok := plannerGoalPreset(raw); ok {
		return g, true, nil
	}
	if strings.EqualFold(raw, "elite-four") || strings.EqualFold(raw, "elite four") ||
		strings.EqualFold(raw, "dex") || strings.EqualFold(raw, "pokedex") || strings.EqualFold(raw, "pokédex") {
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
		return Goal{}, false, nil
	}
}

func PlannerGoalStatus(raw string, obs Observation) (status GoalStatus, structured bool, err error) {
	g, structured, err := PlannerGoal(raw)
	if err != nil || !structured {
		return GoalStatus{}, structured, err
	}
	return EvaluateGoal(g, obs), true, nil
}

// EvaluateGoal depends only on portable planner facts. It does not inspect a
// Red event constant or map byte to decide completion.
func EvaluateGoal(g Goal, obs Observation) GoalStatus {
	switch g.Kind {
	case GoalNone:
		return GoalStatus{Summary: "no deterministic goal"}
	case GoalEliteFour:
		n := len(obs.Badges)
		// A Hall of Fame bit is the authoritative ending fact, but #39's full
		// campaign contract also requires the complete eight-badge journey. This
		// prevents a synthetic/corrupt late-game state from qualifying as a
		// fresh autonomous completion while keeping the predicate portable.
		if n >= 8 && obs.Story.Has(ProgressMainStoryComplete) {
			return GoalStatus{Complete: true, Summary: "Hall of Fame reached after the eight-badge campaign, Elite Four and Champion", Current: 8, Target: 8}
		}
		if obs.Story.Has(ProgressMainStoryComplete) {
			return GoalStatus{Summary: fmt.Sprintf("Hall of Fame reached but campaign badges are only %d/8", n), Current: n, Target: 8}
		}
		return GoalStatus{Summary: fmt.Sprintf("beat the Elite Four and Champion and reach the Hall of Fame; badges %d/8", n), Current: n, Target: 8}
	case GoalDex:
		return evaluateDexGoal(obs)
	case GoalBadges:
		n := len(obs.Badges)
		return GoalStatus{Complete: n >= g.Count, Summary: fmt.Sprintf("badges %d/%d", n, g.Count), Current: n, Target: g.Count}
	case GoalReach:
		target := semanticPlace(g.Target)
		current := obs.Location
		if current == "" {
			// Synthetic legacy fixtures may only provide MapName. Convert that
			// display value into the same semantic identity rather than reading a
			// game-specific map number.
			current = semanticLocation(obs.MapName)
		}
		complete := current == target
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

// evaluateDexGoal deliberately evaluates the ROM/save-derived catalog rather
// than a hard-coded 151 count. Targets are species still obtainable by this
// save; Unavailable entries are excluded from the denominator and can never
// keep the run alive. A zero-sized catalog is treated as unavailable evidence,
// not as a completed Dex, so an observation/build failure cannot false-positive.
func evaluateDexGoal(obs Observation) GoalStatus {
	owned := len(obs.Dex.Owned)
	remaining := len(obs.Dex.Targets)
	unavailable := len(obs.Dex.Unavailable)
	target := owned + remaining
	if target == 0 {
		return GoalStatus{Summary: "Pokédex catalog unavailable; completion cannot be evaluated"}
	}

	complete := remaining == 0
	if complete {
		return GoalStatus{
			Complete: true,
			Summary:  fmt.Sprintf("Pokédex complete: %d/%d obtainable owned; %d unavailable", owned, target, unavailable),
			Current:  owned,
			Target:   target,
		}
	}
	return GoalStatus{
		Summary: fmt.Sprintf("Pokédex owned %d/%d obtainable; %d remaining; %d unavailable", owned, target, remaining, unavailable),
		Current: owned,
		Target:  target,
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
