package agent

// runEngine owns the mutable portable policy state around objective
// transactions. Gameplay mutation still belongs to the transaction/adapter;
// this type coordinates run-level planning, completion, knowledge deltas,
// watchdogs and one unified recoverable-failure policy.
type runEngine struct {
	goal      *runGoalPolicy
	knowledge *runKnowledgePolicy
	planning  *runPlanning
	watchdogs *runWatchdogPolicy
	failures  *runFailurePolicy
}

func newRunEngine(budget Budget, resumed Plan, initial Observation, known *Knowledge, goal *runGoalPolicy) *runEngine {
	return &runEngine{
		goal:      goal,
		knowledge: newRunKnowledgePolicy(initial, known),
		planning:  newRunPlanning(resumed),
		watchdogs: newRunWatchdogPolicy(budget, initial, known),
		failures:  newRunFailurePolicy(budget.MaxConsecutiveFailures),
	}
}

type runWatchdogCause uint8

const (
	runWatchdogNone runWatchdogCause = iota
	runWatchdogStagnation
	runWatchdogStagnationRecurred
	runWatchdogDeadPosition
	runWatchdogShortStuck
)

// runWatchdogDecision is a pure policy result. Run performs the requested
// side effects (logging, planner notification, stopping); the watchdog owns
// only the state transition that decides which of those should happen.
type runWatchdogDecision struct {
	Stop         Stop
	ReplanReason string
	Cause        runWatchdogCause

	MajorProgress  bool
	HighWater      majorProgressMark
	StagnantRounds int
	DeadStreak     int
}

type runWatchdogPolicy struct {
	stuckAfter      int
	stagnationAfter int

	majorProgress          majorProgressMark
	lastMajorProgressRound int
	stagnationEscalated    bool
	stuckEscalated         bool
	stuck                  int
	dead                   deadPosition
}

func newRunWatchdogPolicy(budget Budget, initial Observation, known *Knowledge) *runWatchdogPolicy {
	stuckAfter := budget.StuckAfter
	if stuckAfter <= 0 {
		stuckAfter = defaultStuckAfter
	}
	stagnationAfter := budget.StagnationAfter
	if stagnationAfter <= 0 {
		stagnationAfter = defaultStagnationAfter
	}
	return &runWatchdogPolicy{
		stuckAfter:      stuckAfter,
		stagnationAfter: stagnationAfter,
		majorProgress:   majorProgressMarkOf(initial, known),
	}
}

// roundBoundary evaluates the long stagnation watchdog and the dead-position
// watchdog in the same order Run historically did. It is deliberately ROM-
// free: callers provide only the settled semantic observation and knowledge.
func (w *runWatchdogPolicy) roundBoundary(round int, obs Observation, known *Knowledge, completed int, strategic bool) runWatchdogDecision {
	decision := runWatchdogDecision{}
	current := majorProgressMarkOf(obs, known)
	if w.majorProgress.absorb(current) {
		w.lastMajorProgressRound = round - 1
		w.stagnationEscalated = false
		decision.MajorProgress = true
	}
	decision.HighWater = w.majorProgress

	completedRounds := round - 1
	decision.StagnantRounds = completedRounds - w.lastMajorProgressRound
	if decision.StagnantRounds >= w.stagnationAfter {
		if strategic {
			if !replanOnce(&w.stagnationEscalated) {
				decision.Stop = StopStuck
				decision.Cause = runWatchdogStagnationRecurred
				return decision
			}
			decision.ReplanReason = "stagnation"
			w.lastMajorProgressRound = completedRounds
		} else {
			decision.Stop = StopStuck
			decision.Cause = runWatchdogStagnation
			return decision
		}
	}

	decision.DeadStreak = w.dead.observe(obs, completed)
	if decision.DeadStreak >= deadPositionAfter {
		decision.Stop = StopStuck
		decision.Cause = runWatchdogDeadPosition
	}
	return decision
}

// productiveSession records a bounded gameplay session that made live
// controller progress even though its requested semantic postcondition was not
// reached. Stochastic hunt exhaustion is the canonical case: the full hunt
// budget ran successfully and simply missed the requested species. Treating
// that as idle time makes the stagnation/dead-position watchdogs kill a healthy
// retry loop (#1547). This refreshes liveness without mutating semantic
// majorProgress, so reporting still distinguishes "tried productively" from
// actual badge/story/Dex progress. Explicit round/frame budgets remain the
// outer ceiling for deliberately long-running goals.
func (w *runWatchdogPolicy) productiveSession(round int) {
	if round > w.lastMajorProgressRound {
		w.lastMajorProgressRound = round
	}
	w.stagnationEscalated = false
	w.stuck = 0
	w.stuckEscalated = false
	w.dead = deadPosition{}
}

// successfulObjective evaluates the short stuck watchdog after a successful
// transaction. A material semantic change resets both its counter and its
// one-shot strategic escalation.
func (w *runWatchdogPolicy) successfulObjective(before, after Observation, strategic bool) runWatchdogDecision {
	decision := runWatchdogDecision{}
	if sameProgress(before, after) {
		w.stuck++
	} else {
		w.stuck = 0
		w.stuckEscalated = false
	}
	if w.stuck < w.stuckAfter {
		return decision
	}
	if strategic {
		if !replanOnce(&w.stuckEscalated) {
			decision.Stop = StopStuck
			decision.Cause = runWatchdogShortStuck
			return decision
		}
		decision.ReplanReason = "stuck"
		w.stuck = 0
		return decision
	}
	decision.Stop = StopStuck
	decision.Cause = runWatchdogShortStuck
	return decision
}

// runFailureDecision is the portable consequence of one recoverable
// objective failure. Frame-budget checks intentionally remain engine guards:
// this policy is concerned only with retry/replan/terminal failure state.
type runFailureDecision struct {
	Stop              Stop
	ReplanReason      string
	Recovered         bool
	ProductiveSession bool
}

// runFailurePolicy owns all recoverable-failure state: same-world quarantine,
// repeat detection, strategic escalation and blackout/training retry budgets.
// Every path keys off fingerprintRecoverableFailure, so these mechanisms cannot
// disagree about whether two failures are the same semantic event.
type runFailurePolicy struct {
	maxConsecutive       int
	consecutive          int
	lastFailKey          string
	retreatStreak        int
	lastRetreatLevel     uint8
	escalated            map[string]bool
	quarantine           map[string]failureQuarantineEntry
	pendingPrerequisites []Prerequisite
}

func newRunFailurePolicy(maxConsecutive int) *runFailurePolicy {
	if maxConsecutive <= 0 {
		maxConsecutive = defaultMaxConsecutiveFailures
	}
	return &runFailurePolicy{
		maxConsecutive: maxConsecutive,
		escalated:      map[string]bool{},
		quarantine:     map[string]failureQuarantineEntry{},
	}
}

// recoverable applies the bounded retry policy using only the adapter's
// normalized failure record and semantic result state. It never inspects a
// concrete game/controller error identity.
func (f *runFailurePolicy) recoverable(obj Objective, result ObjectiveResult, strategic bool, leadLevel uint8) runFailureDecision {
	fingerprint := fingerprintRecoverableFailure(obj, result)
	failureKey := fingerprint.Key
	blackedOut := failureIsBlackout(result)
	retryableBlackout := blackedOut
	retreated := failureCauseIs(result, "train_retreat")
	trainProgress := failureCauseIs(result, "train_progress_shortfall")
	huntMiss := failureCauseIs(result, "catch_hunt_exhausted") || failureCauseIs(result, "fishing_hunt_exhausted") ||
		failureCauseIs(result, "catch_attempt_missed")
	routePrerequisite := failureCauseIs(result, "route_prerequisite_missing")
	progressionPrerequisite := failureCauseIs(result, "progression_prerequisite_missing")
	trainingInefficient := failureCauseIs(result, "training_inefficient_area")
	combatDefeat := failureCauseIs(result, failureCauseCombatDefeat)

	// These are successful bounded gameplay sessions whose requested terminal
	// condition simply was not reached. A training shortfall explicitly means
	// the lead gained a level; a hunt exhaustion (grass, fishing or Safari)
	// means the controller completed the whole stochastic hunt budget without
	// landing the requested species; a missed catch means the wanted target
	// was met and balls were thrown but the catch roll never held. Neither is evidence that recovery itself is broken, so
	// neither may consume the fatal consecutive-failure budget or look like idle
	// time to the liveness watchdogs. Same-state quarantine still suppresses the
	// exact objective when alternatives exist; explicit round/frame budgets
	// remain the ceiling for deliberately persistent stochastic goals.
	if trainProgress || huntMiss {
		f.consecutive = 0
		f.lastFailKey = ""
		f.retreatStreak, f.lastRetreatLevel = 0, 0
		decision := runFailureDecision{Recovered: true, ProductiveSession: true}
		if strategic {
			decision.ReplanReason = "objective_failed"
		}
		return decision
	}

	// A typed route prerequisite is an expected planning boundary, not evidence
	// that failure recovery itself is broken. record() has already quarantined
	// the blocked objective (and its plain/flee sibling) and retained the missing
	// capability for deterministic prerequisiteRecovery on the next round.
	// Spending the fatal consecutive-failure/escalation budget here can stop a
	// healthy run after a few distinct route gates before any of those recovery
	// paths execute (#1555, #1557). Keep the ordinary replan signal, but leave
	// the failure budget untouched. Genuine repeated navigation/controller
	// failures still use the bounded policy below, and watchdog/round/frame
	// budgets remain the outer guard if no prerequisite can be satisfied.
	if routePrerequisite || progressionPrerequisite || trainingInefficient {
		// A local training-area rejection is the same class of planning
		// boundary as a missing route prerequisite: the executor deliberately
		// sent no gameplay input because this area cannot satisfy the requested
		// training rung within the bounded session. Quarantine/replanning owns
		// the response; spending the fatal mechanical-failure budget here makes
		// a healthy search for a better area terminate as "recovery exhausted".
		decision := runFailureDecision{Recovered: true}
		if strategic {
			decision.ReplanReason = "objective_failed"
		}
		return decision
	}

	// A structured required-battle defeat is also an expected gameplay outcome,
	// not a controller/recovery malfunction. Knowledge records a combat-loss
	// gate for the exact objective, so an unchanged party cannot immediately
	// rematch it; training, incidental level gain, or another material party
	// change must release that gate first. Counting the defeat against the
	// generic mechanical failure budget therefore kills the run before its
	// dedicated combat recovery can do its job (#1695). Clear the mechanical
	// streak and replan. The normal stagnation/round/frame watchdogs remain the
	// outer guard if the run cannot find a way to improve combat readiness.
	if combatDefeat {
		f.consecutive = 0
		f.lastFailKey = ""
		f.retreatStreak, f.lastRetreatLevel = 0, 0
		decision := runFailureDecision{Recovered: true}
		if strategic {
			decision.ReplanReason = "blackout"
		}
		return decision
	}

	if strategic {
		f.consecutive++
		// Ordinary logistics blackouts (wild encounters, poison, or unstructured
		// travel losses) remain under the bounded consecutive-failure ceiling.
		// Structured required-battle defeats returned above because their combat
		// recovery gate already owns retry suppression and readiness progress.
		if (!retryableBlackout && f.escalated[failureKey]) || f.consecutive > f.maxConsecutive {
			return runFailureDecision{Stop: StopFailed}
		}
		if !retryableBlackout {
			f.escalated[failureKey] = true
		}
		f.lastFailKey = failureKey
		reason := "objective_failed"
		if blackedOut {
			reason = "blackout"
		} else if retreated {
			reason = "train_retreat"
		}
		return runFailureDecision{ReplanReason: reason, Recovered: true}
	}

	if blackedOut || retreated {
		if retreated && leadLevel != 0 && leadLevel == f.lastRetreatLevel {
			f.retreatStreak++
		} else if retreated {
			f.retreatStreak = 1
			f.lastRetreatLevel = leadLevel
		} else {
			f.retreatStreak, f.lastRetreatLevel = 0, 0
		}
		f.lastFailKey = ""
		if retreated && f.retreatStreak >= f.maxConsecutive {
			return runFailureDecision{Stop: StopFailed}
		}
		return runFailureDecision{Recovered: true}
	}

	f.retreatStreak, f.lastRetreatLevel = 0, 0
	f.consecutive++
	if (f.lastFailKey != "" && failureKey == f.lastFailKey) || f.consecutive >= f.maxConsecutive {
		return runFailureDecision{Stop: StopFailed}
	}
	f.lastFailKey = failureKey
	return runFailureDecision{Recovered: true}
}

// success clears streak-local failure state. escalated intentionally survives:
// its keys include semantic world-state evidence, so harmless success cannot
// reset the strategic recovery budget for the exact same failure state.
func (f *runFailurePolicy) success() {
	f.consecutive = 0
	f.lastFailKey = ""
	f.retreatStreak, f.lastRetreatLevel = 0, 0
	// Route-capability recovery is tied to the failed journey and is cleared by
	// any successful objective, matching historical behavior. Progression facts
	// and direct field-capability requirements survive the prerequisite objective
	// itself so the next round can re-observe, prune what just became true, and
	// repair another item from the same structured requirement set.
	kept := f.pendingPrerequisites[:0]
	for _, prerequisite := range f.pendingPrerequisites {
		if prerequisite.Progress != "" || prerequisite.FieldCapability != "" {
			kept = append(kept, prerequisite)
		}
	}
	f.pendingPrerequisites = kept
}
