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

// ObserveRunGoal mirrors Run's authoritative status into the existing live
// statistics. The planner-side progress note is still prepared in Next; when
// Run detects completion before another model call, publishSnapshot makes the
// final complete=true state visible without fabricating a call.
func (s *statsPlanner) ObserveRunGoal(obs agent.Observation, status agent.GoalStatus, deterministic bool) {
	if s == nil {
		return
	}
	s.setGoalStats(status, deterministic)
	if deterministic && status.Complete {
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
