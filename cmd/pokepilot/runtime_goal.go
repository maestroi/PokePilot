package main

import "github.com/maestroi/pokepilot/agent"

// RunGoal exposes the goal already assigned to the concrete LLM planner.
// agent.Run remains the only layer that decides whether the goal is complete.
func (s *statsPlanner) RunGoal() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.Goal
}

// ObserveRunGoal mirrors Run's authoritative status into planner context and
// live statistics. Completion publishes exactly once on the incomplete ->
// complete transition; the runtime may evaluate the final settled state more
// than once for diagnostics, but that must not fabricate duplicate UI events.
func (s *statsPlanner) ObserveRunGoal(obs agent.Observation, status agent.GoalStatus, deterministic bool) {
	if s == nil {
		return
	}
	wasComplete := s.stats.GoalComplete
	s.runGoalStatus = status
	s.runGoalDeterministic = deterministic
	s.setGoalStats(status, deterministic)
	if deterministic && status.Complete && !wasComplete {
		s.publishSnapshot(obs)
	}
}

// reportingPlanner is the farm's watch-page decorator. Forward the run-goal
// interfaces so wrapping statsPlanner cannot accidentally erase the runtime
// completion contract.
func (p reportingPlanner) RunGoal() string {
	if gp, ok := p.inner.(agent.RunGoalProvider); ok {
		return gp.RunGoal()
	}
	return ""
}

func (p reportingPlanner) ObserveRunGoal(obs agent.Observation, status agent.GoalStatus, deterministic bool) {
	if sink, ok := p.inner.(agent.RunGoalStatusObserver); ok {
		sink.ObserveRunGoal(obs, status, deterministic)
	}
}
