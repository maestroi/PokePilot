package agent

// runGoalPolicy owns run-level completion semantics. Keeping this separate
// from the objective loop makes the precedence rule explicit: a satisfied
// deterministic goal wins over round/watchdog limits and planner exhaustion.
type runGoalPolicy struct {
	goal          Goal
	deterministic bool
	maxRounds     int
}

func newRunGoalPolicy(p Planner, budget Budget) (*runGoalPolicy, error) {
	goal, deterministic, err := resolveRunGoal(p, budget.Goal)
	if err != nil {
		return nil, err
	}
	return &runGoalPolicy{goal: goal, deterministic: deterministic, maxRounds: budget.MaxRounds}, nil
}

type runGoalDecision struct {
	Stop   Stop
	Status *GoalStatus
	Err    error
}

func (g *runGoalPolicy) evaluate(p Planner, obs Observation, round int, intent string, intentAge int) *GoalStatus {
	if g == nil || !g.deterministic {
		return nil
	}
	status := evaluateRunGoal(p, g.goal, obs, round, g.maxRounds, intent, intentAge)
	return &status
}

func (g *runGoalPolicy) startRound(p Planner, obs Observation, round int, intent string, intentAge int) runGoalDecision {
	status := g.evaluate(p, obs, round, intent, intentAge)
	if status != nil && status.Complete {
		return runGoalDecision{Stop: StopDone, Status: status}
	}
	if roundCapReached(round, g.maxRounds) {
		return runGoalDecision{Stop: StopBudget, Status: status}
	}
	return runGoalDecision{Status: status}
}

func (g *runGoalPolicy) afterObjective(p Planner, obs Observation, round int, intent string, intentAge int) runGoalDecision {
	status := g.evaluate(p, obs, round, intent, intentAge)
	if status != nil && status.Complete {
		return runGoalDecision{Stop: StopDone, Status: status}
	}
	return runGoalDecision{Status: status}
}

func (g *runGoalPolicy) plannerDone(p Planner, obs Observation, round int, intent string, intentAge int, current *GoalStatus) runGoalDecision {
	if g == nil || !g.deterministic {
		return runGoalDecision{Stop: StopDone}
	}
	status := current
	if status == nil {
		status = g.evaluate(p, obs, round, intent, intentAge)
	}
	if status != nil && status.Complete {
		return runGoalDecision{Stop: StopDone, Status: status}
	}
	if status == nil {
		status = &GoalStatus{}
	}
	return runGoalDecision{Stop: StopError, Status: status, Err: incompleteGoalError(*status)}
}

// runKnowledgePolicy owns the run-local high-water marks that translate new
// portable knowledge into strategic replanning requests. Knowledge itself
// remains the durable fact store; this policy only detects meaningful deltas.
type runKnowledgePolicy struct {
	requirements int
	badges       int
	events       int
}

func newRunKnowledgePolicy(initial Observation, known *Knowledge) *runKnowledgePolicy {
	return &runKnowledgePolicy{
		requirements: len(known.Requirements),
		badges:       len(initial.Badges),
		events:       len(initial.Events),
	}
}

func (k *runKnowledgePolicy) roundBoundary(round int, obs Observation, known *Knowledge, seenMaps []uint8) []string {
	noteObservation(known, obs)
	reasons := make([]string, 0, 3)
	if len(known.Requirements) > k.requirements {
		reasons = append(reasons, "new_requirement")
		k.requirements = len(known.Requirements)
	}
	if round > 1 && len(obs.Badges) > k.badges {
		reasons = append(reasons, "badge_changed")
	}
	if round > 1 && len(obs.Events) > k.events {
		reasons = append(reasons, "story_changed")
	}
	k.badges, k.events = len(obs.Badges), len(obs.Events)
	for _, id := range seenMaps {
		known.SawMap(id)
	}
	return reasons
}

type runRoundBoundaryDecision struct {
	Stop          Stop
	GoalStatus    *GoalStatus
	Watchdog      runWatchdogDecision
	PolicyApplied bool
}

// beginRound centralizes the ordering of all pre-planner policy. That ordering
// is semantic: completion first, then round budget, knowledge-driven replans,
// then stagnation/dead-position watchdogs. seenMaps is deliberately lazy so a
// terminal completion/budget decision does not consume transient sampler state
// that the historical Run loop never touched after deciding to stop.
func (e *runEngine) beginRound(p Planner, round int, obs Observation, known *Knowledge, seenMaps func() []uint8, intent string, intentAge int) runRoundBoundaryDecision {
	goal := e.goal.startRound(p, obs, round, intent, intentAge)
	decision := runRoundBoundaryDecision{Stop: goal.Stop, GoalStatus: goal.Status}
	if decision.Stop != StopUnset {
		return decision
	}
	decision.PolicyApplied = true

	var maps []uint8
	if seenMaps != nil {
		maps = seenMaps()
	}
	for _, reason := range e.knowledge.roundBoundary(round, obs, known, maps) {
		e.planning.request(reason)
	}
	decision.Watchdog = e.watchdogs.roundBoundary(
		round, obs, known, len(known.Completed), e.planning.hasStrategist(p),
	)
	if decision.Watchdog.ReplanReason != "" {
		e.planning.request(decision.Watchdog.ReplanReason)
	}
	decision.Stop = decision.Watchdog.Stop
	return decision
}
