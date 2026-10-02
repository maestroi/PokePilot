package session

import (
	"fmt"
	"strconv"
	"strings"
)

// EarlyLevels is the first-room qualification batch after the one-puzzle
// milestone: boot, solve, and advance this many puzzles.
const EarlyLevels = 5

// GoalKind names a Boxxle run stop condition.
type GoalKind string

const (
	GoalFirst   GoalKind = "first"
	GoalEarly   GoalKind = "early"
	GoalLevels  GoalKind = "levels"
	GoalEndless GoalKind = "endless"
)

// Goal is how many puzzles a Boxxle run should solve before stopping.
type Goal struct {
	Kind   GoalKind
	Levels int
}

// ParseGoal accepts first (default), early, endless, or levels:N.
func ParseGoal(raw string) (Goal, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "", "first", "auto":
		return Goal{Kind: GoalFirst, Levels: 1}, nil
	case "early":
		return Goal{Kind: GoalEarly, Levels: EarlyLevels}, nil
	case "endless":
		return Goal{Kind: GoalEndless}, nil
	}
	if strings.HasPrefix(raw, "levels:") {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(raw, "levels:")))
		if err != nil || n <= 0 {
			return Goal{}, fmt.Errorf("boxxle session: levels target must be a positive integer")
		}
		return Goal{Kind: GoalLevels, Levels: n}, nil
	}
	return Goal{}, fmt.Errorf("boxxle session: unknown goal %q; want first, early, endless, or levels:N", raw)
}

func (g Goal) String() string {
	switch g.Kind {
	case GoalLevels:
		return fmt.Sprintf("levels:%d", g.Levels)
	case "":
		return string(GoalFirst)
	default:
		return string(g.Kind)
	}
}

// Endless reports whether the run has no puzzle-count stop.
func (g Goal) Endless() bool { return g.Kind == GoalEndless }

// WantedLevels is how many puzzles prove the goal. Zero means no level cap.
func (g Goal) WantedLevels() int {
	if g.Endless() {
		return 0
	}
	if g.Levels <= 0 {
		return 1
	}
	return g.Levels
}
