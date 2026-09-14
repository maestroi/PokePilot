package agent

import (
	"errors"
	"fmt"
	"os"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// Run drives observe -> plan -> execute until the run-owned deterministic
// goal is done, a prompt-only planner is done, policy stops the run, or a
// safety guard fires. Gameplay state mutation remains inside objective
// transactions; Run coordinates the portable engine state around them.
func Run(m *emu.Emu, romData []byte, p Planner, budget Budget) Result {
	if budget.MaxRounds < 0 || budget.MaxFrames <= 0 {
		return Result{
			Stop: StopError,
			Err:  errors.New("agent: Run: MaxRounds cannot be negative and MaxFrames must be positive"),
		}
	}
	select {
	case <-budget.Cancel:
		return Result{Stop: StopBudget, Rounds: 0}
	default:
	}

	runGoal, deterministicGoal, err := resolveRunGoal(p, budget.Goal)
	if err != nil {
		return Result{Stop: StopError, Err: err}
	}
	graph, err := world.BuildGraph(romData)
	if err != nil {
		return Result{Stop: StopError, Err: fmt.Errorf("agent: Run: build map graph: %w", err)}
	}
	adjacency := make(map[uint8][]uint8, len(graph.Edges))
	for from, edges := range graph.Edges {
		for _, e := range edges {
			adjacency[from] = append(adjacency[from], e.To)
		}
	}

	known := NewKnowledge(adjacency)
	intent, intentAge := "", 0
	resumedPlan := Plan{}
	if budget.ResumeFrom != "" {
		stateBytes, err := os.ReadFile(budget.ResumeFrom)
		if err != nil {
			return Result{Stop: StopError, Err: fmt.Errorf("agent: Run: resume %s: %w", budget.ResumeFrom, err)}
		}
		if err := m.LoadState(stateBytes); err != nil {
			return Result{Stop: StopError, Err: fmt.Errorf("agent: Run: resume %s: LoadState: %w", budget.ResumeFrom, err)}
		}
		mem := LoadCheckpointMemory(budget.ResumeFrom, adjacency, budget.Log)
		known = mem.Knowledge
		intent, intentAge = mem.Intent, mem.IntentAge
		resumedPlan = mem.Plan.clone()
	}

	var ring *checkpointRing
	if budget.CheckpointDir != "" {
		keep := budget.CheckpointKeep
		if keep <= 0 {
			keep = defaultCheckpointKeep
		}
		ring = &checkpointRing{dir: budget.CheckpointDir, keep: keep}
	}

	res := Result{Completed: []Objective{}, Outcomes: []ObjectiveResult{}}
	startFrame := m.FrameCount()
	tape := &dialogueTape{}
	m.AlsoSample(tape.sample)
	var history []RoundRecord
	last := Observe(m, romData)
	engine := newRunEngine(budget, resumedPlan, last, known)
	notifyPlanning(p, engine.planning.snapshot())

	knownRequirementCount := len(known.Requirements)
	observedBadges, observedEvents := len(last.Badges), len(last.Events)
	early := progressOf(last, known, 0)
	res.ProgressEarly = &early
	lastUnroutable := ""

	for round := 1; ; round++ {
		select {
		case <-budget.Cancel:
			return Result{Stop: StopBudget, Rounds: round - 1}
		default:
		}

		// Completion has precedence over watchdogs, budget edges, and another
		// planner call. A state that satisfies the deterministic goal is done
		// even when the objective that reached it also exposed a later fault.
		if deterministicGoal {
			status := evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
			res.GoalStatus = &status
			if status.Complete {
				res.Stop = StopDone
				break
			}
		}
		if roundCapReached(round, budget.MaxRounds) {
			res.Stop = StopBudget
			break
		}

		// Knowledge update policy remains explicit at the round boundary. The
		// engine carries planning/watchdog/failure state; deterministic game
		// facts remain owned by Knowledge and the adapter observation.
		noteObservation(known, last)
		if len(known.Requirements) > knownRequirementCount {
			engine.planning.request("new_requirement")
			knownRequirementCount = len(known.Requirements)
		}
		if round > 1 && len(last.Badges) > observedBadges {
			engine.planning.request("badge_changed")
		}
		if round > 1 && len(last.Events) > observedEvents {
			engine.planning.request("story_changed")
		}
		observedBadges, observedEvents = len(last.Badges), len(last.Events)
		for _, id := range tape.seenMaps() {
			known.SawMap(id)
		}
		logUnroutable(budget.Log, round, last, &lastUnroutable)

		watchdog := engine.watchdogs.roundBoundary(
			round, last, known, len(known.Completed), engine.planning.hasStrategist(p),
		)
		if watchdog.ReplanReason != "" {
			engine.planning.request(watchdog.ReplanReason)
		}
		logWatchdogDecision(budget.Log, round, watchdog, last)
		if watchdog.Stop != StopUnset {
			res.Stop = watchdog.Stop
			break
		}

		if reqs := known.Requirements; len(reqs) > 0 {
			last.Requirements = append([]Requirement{}, reqs...)
		}
		last.Failures = known.FailureList()
		now := offerWithTMHM(m, romData, last, known)
		now = engine.quarantine.filter(last, now)
		if len(now) == 0 {
			res.Stop = StopError
			res.Err = errors.New("agent: Run: nothing is possible from here")
			break
		}

		last.Intent = intent
		last.IntentAge = intentAge
		last.Round = round
		last.RoundsLeft = roundsLeft(round, budget.MaxRounds)

		obj, fromPlan, err, retries := engine.planning.choose(budget.Log, round, p, last, now)
		res.ReplyRetries += retries
		notifyPlanning(p, engine.planning.snapshot())
		if errors.Is(err, ErrDone) {
			if deterministicGoal {
				status := GoalStatus{}
				if res.GoalStatus != nil {
					status = *res.GoalStatus
				} else {
					status = evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
					res.GoalStatus = &status
				}
				res.Stop = StopError
				res.Err = incompleteGoalError(status)
				break
			}
			res.Stop = StopDone
			break
		}
		if err != nil {
			res.Stop = StopError
			res.Err = err
			break
		}

		switch {
		case obj.Intent != "" && obj.Intent != intent:
			intent, intentAge = obj.Intent, 0
		case intent != "":
			intentAge++
		}

		if ring != nil {
			if err := ring.write(m, round, obj, known, intent, intentAge, engine.planning.Plan); err != nil {
				res.Stop = StopError
				res.Err = fmt.Errorf("agent: Run: checkpoint round %d: %w", round, err)
				break
			}
		}

		before := last
		objectiveResult, execErr := executeObjectiveResult(m, romData, obj)
		last = objectiveResult.Final
		res.Rounds = round

		if execErr != nil {
			blackedOut := errors.Is(execErr, skill.ErrBlackedOut)
			retreated := errors.Is(execErr, skill.ErrTrainRetreat)
			if blackedOut {
				last.BlackedOut = true
				objectiveResult.Summary += fmt.Sprintf(" (respawned in %s, money %d -> %d)",
					last.RespawnPlace, before.Money, last.Money)
			}
			res.Outcomes = append(res.Outcomes, objectiveResult)
			outcome := objectiveResult.HistoryText()

			known.Failed(obj, execErr)
			known.notePartyCombatChange(before, last, execErr)
			history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: outcome})
			last.History = history
			last.RecentDialogue = tape.recent()
			logRound(budget.Log, round, obj, outcome, last)

			if deterministicGoal {
				status := evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
				res.GoalStatus = &status
				if status.Complete {
					markLastOutcomeRecovered(&res)
					res.Stop = StopDone
					break
				}
			}

			action := actionFor(objectiveResult.Outcome)
			switch action {
			case actionChoice, actionStop:
				markLastOutcomeTerminal(&res)
				res.Stop, res.Err = StopError, execErr
			case actionContinue:
				markLastOutcomeTerminal(&res)
				res.Stop = StopError
				res.Err = fmt.Errorf("agent: objective %s returned error with completed outcome: %w", obj, execErr)
			case actionReplan:
				engine.quarantine.record(objectiveResult)
			}
			if res.Stop != StopUnset {
				break
			}

			leadLevel := uint8(0)
			if len(last.Party) > 0 {
				leadLevel = last.Party[0].Level
			}
			failure := engine.failures.recoverable(
				obj, objectiveResult, blackedOut, retreated, engine.planning.hasStrategist(p), leadLevel,
			)
			if failure.ReplanReason != "" {
				engine.planning.request(failure.ReplanReason)
				notifyPlanning(p, engine.planning.snapshot())
			}
			if failure.Stop != StopUnset {
				markLastOutcomeTerminal(&res)
				res.Stop, res.Err = failure.Stop, execErr
				break
			}
			if failure.Recovered {
				markLastOutcomeRecovered(&res)
			}
			if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
				res.Stop = StopBudget
				break
			}
			continue
		}

		res.Outcomes = append(res.Outcomes, objectiveResult)
		res.Completed = append(res.Completed, obj)
		engine.quarantine.clear(obj)
		engine.planning.success(fromPlan)
		notifyPlanning(p, engine.planning.snapshot())
		known.Done(obj)
		known.notePartyCombatChange(before, last, nil)
		if obj.Kind == KindTalk {
			known.TalkedTo(before.Map, obj.X, obj.Y)
		}
		engine.failures.success()
		history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: objectiveResult.HistoryText()})
		last.History = history
		last.RecentDialogue = tape.recent()
		logRound(budget.Log, round, obj, objectiveResult.HistoryText(), last)

		if deterministicGoal {
			status := evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
			res.GoalStatus = &status
			if status.Complete {
				res.Stop = StopDone
				break
			}
		}

		watchdog = engine.watchdogs.successfulObjective(before, last, engine.planning.hasStrategist(p))
		if watchdog.ReplanReason != "" {
			engine.planning.request(watchdog.ReplanReason)
			notifyPlanning(p, engine.planning.snapshot())
		}
		if watchdog.Stop != StopUnset {
			res.Stop = watchdog.Stop
			break
		}
		if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
			res.Stop = StopBudget
			break
		}
	}

	res.Final = last
	res.Planning = engine.planning.snapshot()
	if deterministicGoal {
		status := evaluateRunGoal(p, runGoal, last, res.Rounds, budget.MaxRounds, intent, intentAge)
		res.GoalStatus = &status
	}
	final := progressOf(last, known, res.Rounds)
	res.ProgressFinal = &final
	if up, ok := p.(UsagePlanner); ok {
		res.PromptTokens, res.CompletionTokens = up.Usage()
	}
	return res
}

func progressOf(obs Observation, k *Knowledge, round int) Progress {
	return Progress{
		Round:   round,
		Badges:  len(obs.Badges),
		Events:  len(obs.Events),
		Maps:    len(k.Visited),
		Map:     obs.Map,
		MapName: obs.MapName,
	}
}

func noteObservation(k *Knowledge, obs Observation) {
	k.SawMap(obs.Map)
	k.SawDialogue(obs.RecentDialogue, obs.MapName, obs.X, obs.Y)
}

func sameProgress(a, b Observation) bool {
	if a.Map != b.Map || a.X != b.X || a.Y != b.Y || a.PartyCount != b.PartyCount {
		return false
	}
	if len(a.Events) != len(b.Events) {
		return false
	}
	for i := range a.Events {
		if a.Events[i] != b.Events[i] {
			return false
		}
	}
	return true
}
