package farm

import "strings"

const (
	DefaultEliteFourGoal = "elite-four"
	DefaultDexGoal       = "dex"
)

// DefaultGoalForPlayStyle returns the wire-level terminal goal implied by an
// explicitly selected play style. Empty/unknown styles intentionally return
// empty so legacy specs keep their historical behavior instead of silently
// acquiring a new goal.
func DefaultGoalForPlayStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "speedrun", "adventure", "team_builder", "team-builder", "teambuilder":
		return DefaultEliteFourGoal
	case "completionist":
		return DefaultDexGoal
	default:
		return ""
	}
}

// ApplyPlayStyleDefaultGoal resolves a missing LLM goal from the explicitly
// selected play style. An explicit goal always wins. Scripted runs never gain
// an LLM goal from a play style extension.
func ApplyPlayStyleDefaultGoal(spec *Spec) {
	if spec == nil || !strings.EqualFold(strings.TrimSpace(spec.Planner), "llm") || strings.TrimSpace(spec.Goal) != "" {
		return
	}
	if goal := DefaultGoalForPlayStyle(PlayStyleForSpec(*spec)); goal != "" {
		spec.Goal = goal
	}
}
