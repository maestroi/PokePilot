package agent

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
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

	goalPolicy, err := newRunGoalPolicy(p, budget)
	if err != nil {
		return Result{Stop: StopError, Err: err}
	}
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return Result{Stop: StopError, Err: fmt.Errorf("agent: Run: detect game profile: %w", err)}
	}
	graph, err := world.BuildGraph(romData)
	if err != nil {
		return Result{Stop: StopError, Err: fmt.Errorf("agent: Run: build map graph: %w", err)}
	}
	nativeAdjacency := make(map[uint8][]uint8, len(graph.Edges))
	for from, edges := range graph.Edges {
		for _, e := range edges {
			nativeAdjacency[from] = append(nativeAdjacency[from], e.To)
		}
	}
	topology := knowledgeTopologyFor(profile.ID(), nativeAdjacency)

	known := NewKnowledge(topology)
	coverage := newCoverageTracker()
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
		mem := LoadCheckpointMemory(budget.ResumeFrom, topology, budget.Log)
		known = mem.Knowledge
		coverage = loadCoverageFile(budget.ResumeFrom, budget.Log)
		intent, intentAge = mem.Intent, mem.IntentAge
		resumedPlan = mem.Plan.clone()
	}
	// Failure evidence is scoped to the binary that is actually running. A
	// resumed checkpoint may carry tallies from an older build; re-apply the
	// active build after loading checkpoint memory so fixed objectives are not
	// discouraged by stale pre-fix history.
	known.Build = budget.Build

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
	runStarted := time.Now()
	tape := &dialogueTape{}
	m.AlsoSample(tape.sample)
	var history []RoundRecord
	last, err := observeRunInitial(m, romData)
	if err != nil {
		return Result{Stop: StopError, Err: err}
	}
	res.StartFrame = startFrame
	res.Initial = last
	coverage.seed(last)
	engine := newRunEngine(budget, resumedPlan, last, known, goalPolicy)
	notifyPlanning(p, engine.planning.snapshot())

	early := progressOf(last, known, coverage, 0)
	res.ProgressEarly = &early
	lastUnroutable := ""

runLoop:
	for round := 1; ; round++ {
		select {
		case <-budget.Cancel:
			res.Stop = StopBudget
			res.Rounds = round - 1
			if ring != nil {
				if err := ring.writeBoundary(m, round, "cancel", known, coverage, intent, intentAge, engine.planning.Plan); err != nil {
					res.Stop = StopError
					res.Err = fmt.Errorf("agent: Run: cancel checkpoint round %d: %w", round, err)
				}
			}
			break runLoop
		default:
		}

		boundary := engine.beginRound(p, round, last, known, tape.seenMaps, intent, intentAge)
		if boundary.GoalStatus != nil {
			res.GoalStatus = boundary.GoalStatus
		}
		if boundary.PolicyApplied {
			logUnroutable(budget.Log, round, last, &lastUnroutable)
			logWatchdogDecision(budget.Log, round, boundary.Watchdog, last)
		}
		if boundary.Stop != StopUnset {
			res.Stop = boundary.Stop
			break
		}

		// Requirements has two sources with different lifetimes: durable facts
		// learned from dialogue and ephemeral provider block evidence. Rebuild the
		// slice every round so a material state change naturally removes provider
		// blockers instead of turning them into stale durable knowledge.
		last.Requirements = append([]Requirement(nil), known.Requirements...)
		last.Failures = known.FailureList()
		offer := offerWithTMHMEvidence(m, romData, last, known)
		last.Requirements = append(last.Requirements, providerBlockRequirements(offer.Blocked)...)
		now := engine.failures.filter(last, offer.Candidates)
		if len(now) == 0 {
			res.Stop = StopError
			res.Err = errors.New("agent: Run: nothing is possible from here")
			break
		}

		last.Intent = intent
		last.IntentAge = intentAge
		last.Round = round
		last.RoundsLeft = roundsLeft(round, budget.MaxRounds)

		var (
			obj      Objective
			fromPlan bool
			err      error
			retries  int
		)
		if prep, ok := combatPreparationObjective(last, now, known); ok {
			obj = prep
			engine.planning.request("combat_preparation")
			if budget.Log != nil {
				state := combatPreparationFor(known, last)
				fmt.Fprintf(budget.Log, "round %d: deterministic combat preparation readiness=%d/%d losses=%d -> %s\n",
					round, state.Current, state.Target, state.Losses, obj)
			}
		} else if recovery, capabilities, ok := engine.failures.prerequisiteRecovery(last, now); ok {
			obj = recovery
			engine.planning.request("prerequisite_recovery")
			if budget.Log != nil {
				fmt.Fprintf(budget.Log, "round %d: deterministic prerequisite recovery for %v -> %s\n", round, capabilities, obj)
			}
		} else {
			obj, fromPlan, err, retries = engine.planning.choose(budget.Log, round, p, last, now)
		}
		res.ReplyRetries += retries
		notifyPlanning(p, engine.planning.snapshot())
		if errors.Is(err, ErrDone) {
			done := engine.goal.plannerDone(p, last, round, intent, intentAge, res.GoalStatus)
			if done.Status != nil {
				res.GoalStatus = done.Status
			}
			res.Stop, res.Err = done.Stop, done.Err
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
			if err := ring.write(m, round, obj, known, coverage, intent, intentAge, engine.planning.Plan); err != nil {
				res.Stop = StopError
				res.Err = fmt.Errorf("agent: Run: checkpoint round %d: %w", round, err)
				break
			}
		}

		if budget.OnObjective != nil {
			budget.OnObjective(ObjectiveActivity{
				Stage: "started", Objective: obj.String(), Frame: m.FrameCount(), Round: round,
			})
		}
		before := last
		objectiveResult, execErr := executeObjectiveResultWithRoutePriority(m, romData, obj, routePriorityForPlanner(p))
		settledTiming := ObjectiveTiming{Frame: m.FrameCount(), Round: round, WallElapsed: time.Since(runStarted)}
		if budget.OnObjective != nil {
			stage := "completed"
			errText := ""
			if execErr != nil {
				stage = "failed"
				errText = execErr.Error()
			}
			budget.OnObjective(ObjectiveActivity{
				Stage: stage, Objective: obj.String(), Outcome: objectiveResult.HistoryText(), Error: errText,
				Frame: settledTiming.Frame, Round: round,
			})
		}
		last = objectiveResult.Final
		coverage.seed(last)
		res.Rounds = round

		if execErr != nil {
			blackedOut := failureIsBlackout(objectiveResult)
			if blackedOut {
				last.BlackedOut = true
				objectiveResult.Summary += fmt.Sprintf(" (respawned in %s, money %d -> %d)",
					last.RespawnPlace, before.Money, last.Money)
			}
			res.Outcomes = append(res.Outcomes, objectiveResult)
			res.OutcomeTimings = append(res.OutcomeTimings, settledTiming)
			outcome := objectiveResult.HistoryText()

			known.FailedResult(objectiveResult, execErr)
			if errors.Is(execErr, gameruntime.ErrMachineUnusable) {
				// The stalled step may still hold the emulator lock. Record the
				// failure beside the checkpoint already written for this round
				// and stop. Do not save, step, or ask the planner to continue.
				known.noteMachineUnusable(obj, execErr)
				if ring != nil {
					if err := ring.rewriteKnowledge(known, intent, intentAge, engine.planning.Plan); err != nil {
						execErr = fmt.Errorf("%w (checkpoint knowledge: %v)", execErr, err)
					}
				}
				history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: outcome})
				last.History = history
				last.RecentDialogue = tape.recent()
				logRound(budget.Log, round, obj, outcome, last)
				markLastOutcomeTerminal(&res)
				res.Stop, res.Err = StopError, execErr
				break
			}
			known.notePartyCombatResult(before, last, objectiveResult)
			history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: outcome})
			last.History = history
			last.RecentDialogue = tape.recent()
			logRound(budget.Log, round, obj, outcome, last)

			goal := engine.goal.afterObjective(p, last, round, intent, intentAge)
			if goal.Status != nil {
				res.GoalStatus = goal.Status
			}
			if goal.Stop == StopDone {
				markLastOutcomeRecovered(&res)
				res.Stop = StopDone
				break
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
				// Typed failure decisions are advisory inside the deterministic
				// safety envelope. Only outcomes already classified as recoverable
				// can reach this hook, and only the conservative pause/impossible
				// choices may tighten policy into a stop.
				if decider, ok := p.(FailureDecisionPlanner); ok {
					decision, decisionErr := decider.DecideFailure(objectiveResult)
					if decisionErr == nil {
						stop, mappingErr := FailureDecisionStops(decision.Choice)
						if mappingErr == nil && stop {
							markLastOutcomeTerminal(&res)
							res.Stop = StopError
							sentinel := ErrDecisionImpossible
							if decision.Choice == "pause" {
								sentinel = ErrDecisionPause
							}
							res.Err = fmt.Errorf("%w: disposition=%s confidence=%.3f after %s: %v",
								sentinel, decision.Choice, decision.Confidence, obj, execErr)
							break
						}
					}
				}
				engine.failures.record(objectiveResult)
			}
			if res.Stop != StopUnset {
				break
			}

			leadLevel := uint8(0)
			if len(last.Party) > 0 {
				leadLevel = last.Party[0].Level
			}
			failure := engine.failures.recoverable(
				obj, objectiveResult, engine.planning.hasStrategist(p), leadLevel,
			)
			if failure.ReplanReason != "" {
				engine.planning.request(failure.ReplanReason)
				notifyPlanning(p, engine.planning.snapshot())
			}
			if failure.ProductiveSession {
				engine.watchdogs.productiveSession(round)
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
		res.OutcomeTimings = append(res.OutcomeTimings, settledTiming)
		res.Completed = append(res.Completed, obj)
		engine.failures.clear(obj)
		legBoundary, droppedTail := engine.planning.success(fromPlan, obj)
		if legBoundary && budget.Log != nil {
			fmt.Fprintf(budget.Log, "round %d: strategic leg boundary reached goal=%q objective=%s dropped_tail_steps=%d\n",
				round, engine.planning.Plan.Goal, obj, droppedTail)
		}
		notifyPlanning(p, engine.planning.snapshot())
		known.Done(obj)
		known.notePartyCombatResult(before, last, objectiveResult)
		if obj.Kind == KindTalk {
			known.TalkedAt(observationLocation(before, known), obj.X, obj.Y)
		}
		coverage.noteSuccess(obj, before, last)
		engine.failures.success()
		history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: objectiveResult.HistoryText()})
		last.History = history
		last.RecentDialogue = tape.recent()
		logRound(budget.Log, round, obj, objectiveResult.HistoryText(), last)

		goal := engine.goal.afterObjective(p, last, round, intent, intentAge)
		if goal.Status != nil {
			res.GoalStatus = goal.Status
		}
		if goal.Stop == StopDone {
			res.Stop = StopDone
			break
		}

		watchdog := engine.watchdogs.successfulObjective(before, last, engine.planning.hasStrategist(p))
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
	res.FinalFrame = m.FrameCount()
	res.Planning = engine.planning.snapshot()
	if status := engine.goal.evaluate(p, last, res.Rounds, intent, intentAge); status != nil {
		res.GoalStatus = status
	}
	final := progressOf(last, known, coverage, res.Rounds)
	res.ProgressFinal = &final
	if up, ok := p.(UsagePlanner); ok {
		res.PromptTokens, res.CompletionTokens = up.Usage()
	}
	return res
}

func observeRunInitial(m *emu.Emu, romData []byte) (Observation, error) {
	obs, err := ObserveChecked(m, romData)
	if err != nil {
		return Observation{}, fmt.Errorf("agent: Run: initial observation: %w", err)
	}
	return obs, nil
}

func progressOf(obs Observation, k *Knowledge, coverage *coverageTracker, round int) Progress {
	return Progress{
		Round:    round,
		Badges:   len(obs.Badges),
		Events:   len(obs.Events),
		Maps:     len(k.Visited),
		Map:      obs.Map,
		MapName:  obs.MapName,
		Coverage: coverage.snapshot(obs, k),
	}
}

func noteObservation(k *Knowledge, obs Observation) {
	k.SawLocation(observationLocation(obs, k))
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
