package agent

import (
	"fmt"
	"io"
	"strings"
)

func logRound(w io.Writer, round int, o Objective, outcome string, after Observation) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "round %d: %s -> %s, map %02x at (%d,%d)\n", round, o, outcome, after.Map, after.X, after.Y)
}

func logUnroutable(w io.Writer, round int, obs Observation, prev *string) {
	if w == nil || len(obs.Unroutable) == 0 {
		return
	}
	line := strings.Join(obs.Unroutable, ", ")
	if line == *prev {
		return
	}
	*prev = line
	fmt.Fprintf(w, "round %d: unroutable from map %02x at (%d,%d): %s\n",
		round, obs.Map, obs.X, obs.Y, line)
}

func logWatchdogDecision(w io.Writer, round int, decision runWatchdogDecision, obs Observation) {
	if w == nil {
		return
	}
	if decision.MajorProgress && round > 1 {
		fmt.Fprintf(w, "round %d: major progress -> %s\n", round-1, decision.HighWater)
	}
	if decision.Stop != StopStuck {
		return
	}
	switch decision.Cause {
	case runWatchdogStagnationRecurred:
		fmt.Fprintf(w, "stagnation watchdog recurred after strategic replan: %d rounds without major progress; high-water mark: %s\n",
			decision.StagnantRounds, decision.HighWater)
	case runWatchdogStagnation:
		fmt.Fprintf(w, "stagnation watchdog: %d rounds without major progress; high-water mark: %s\n",
			decision.StagnantRounds, decision.HighWater)
	case runWatchdogDeadPosition:
		fmt.Fprintf(w, "dead-position watchdog: %d rounds at map %#04x (%d,%d) with no completed objective\n",
			decision.DeadStreak, obs.Map, obs.X, obs.Y)
	}
}
