package agent

// runEngine owns the mutable policy state that used to live as a cluster of
// counters and booleans inside Run. Gameplay mutation still belongs to the
// objective transaction/adapter; this type only coordinates portable run
// policy around those transactions.
type runEngine struct {
	planning   *runPlanning
	watchdogs  *runWatchdogPolicy
	failures   *runFailurePolicy
	quarantine failureQuarantine
}

func newRunEngine(budget Budget, resumed Plan, initial Observation, known *Knowledge) *runEngine {
	return &runEngine{
		planning:   newRunPlanning(resumed),
		watchdogs:  newRunWatchdogPolicy(budget, initial, known),
		failures:   newRunFailurePolicy(budget.MaxConsecutiveFailures),
		quarantine: newFailureQuarantine(),
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
	Stop         Stop
	ReplanReason string
	Recovered    bool
}

type runFailurePolicy struct {
	maxConsecutive   int
	consecutive      int
	lastFailKey      string
	retreatStreak    int
	lastRetreatLevel uint8
	escalated        map[string]bool
}

func newRunFailurePolicy(maxConsecutive int) *runFailurePolicy {
	if maxConsecutive <= 0 {
		maxConsecutive = defaultMaxConsecutiveFailures
	}
	return &runFailurePolicy{
		maxConsecutive: maxConsecutive,
		escalated:      map[string]bool{},
	}
}

// recoverable applies the historical Run retry policy without touching an
// emulator. The caller has already established that actionFor(result.Outcome)
// is actionReplan; this method decides whether that replan/retry budget is
// still available.
func (f *runFailurePolicy) recoverable(obj Objective, result ObjectiveResult, blackedOut, retreated, strategic bool, leadLevel uint8) runFailureDecision {
	failureKey := recoverableFailureKey(obj, result)
	if strategic {
		f.consecutive++
		reason, key, terminal := recoverableFailureReplan(
			f.escalated, obj, result, blackedOut, retreated, f.consecutive, f.maxConsecutive,
		)
		if terminal {
			return runFailureDecision{Stop: StopFailed}
		}
		f.escalated[key] = true
		f.lastFailKey = failureKey
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
}
