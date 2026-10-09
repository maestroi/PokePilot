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
	w := &runWatchdogPolicy{
		stuckAfter:      stuckAfter,
		stagnationAfter: stagnationAfter,
		majorProgress:   majorProgressMarkOf(initial, known),
	}
	if known != nil && known.Stagnation.Mark != (majorProgressMark{}) {
		// Resume: keep counting from the checkpoint's stagnant rounds unless
		// the resumed state already beats the remembered high-water mark.
		w.majorProgress = known.Stagnation.Mark
		if !w.majorProgress.absorb(majorProgressMarkOf(initial, known)) {
			w.lastMajorProgressRound = -known.Stagnation.Rounds
		}
	}
	return w
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
	if known != nil {
		defer func() {
			known.Stagnation = StagnationMemory{Mark: w.majorProgress, Rounds: completedRounds - w.lastMajorProgressRound}
		}()
	}
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
		failureCauseIs(result, "catch_attempt_missed") || failureCauseIs(result, "static_capture_exhausted")
	staticUnavailable := failureCauseIs(result, "static_capture_unavailable")
	routePrerequisite := failureCauseIs(result, "route_prerequisite_missing")
	routeUnavailable := failureCauseIs(result, "no_route") || failureCauseIs(result, "no_path")
	routeSearchExhausted := failureCauseIs(result, "route_replan_exhausted")
	navigationStalled := failureCauseIs(result, "navigation_stalled")
	transitionExecutionFailed := failureCauseIs(result, "transition_execution_failed")
	pushPuzzleSearchExhausted := failureCauseIs(result, "push_puzzle_search_exhausted")
	progressionPrerequisite := failureCauseIs(result, "progression_prerequisite_missing")
	trainingInefficient := failureCauseIs(result, "training_inefficient_area")
	localInteractionUnavailable := obj.Kind == KindTalk && failureCauseIs(result, "no_dialogue")
	// Economic blockages are resource/planning feedback, not mechanical
	// recovery failure. KindBuy owns the generic "outcome:blocked" shop
	// boundary; typed money/stock causes must also spare KindGym /
	// KindProgress when a nested restock (Cut-carrier balls before Vermilion
	// Gym, Saffron drinks, etc.) reports cant_afford without a Buy objective.
	economicBlocked := result.Outcome == OutcomeBlocked && (failureCauseIs(result, "cant_afford") ||
		failureCauseIs(result, "not_in_stock") ||
		failureCauseIs(result, "bag_not_risen") ||
		(obj.Kind == KindBuy && failureCauseIs(result, "outcome:blocked")))
	combatDefeat := failureCauseIs(result, failureCauseCombatDefeat)

	// These are successful bounded gameplay sessions whose requested terminal
	// condition simply was not reached. A training shortfall explicitly means
	// the lead gained a level; a hunt exhaustion (grass, fishing or Safari)
	// means the controller completed the whole stochastic hunt budget without
	// landing the requested species; a finite static exhaustion means all
	// rollback-safe RNG phases were attempted without consuming the one-time
	// encounter; a missed catch means the wanted target was met and balls were
	// thrown but the catch roll never held. None is evidence that recovery itself is broken, so
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
	// the failure budget untouched. Genuine controller failures still use the
	// bounded policy below, and watchdog/round/frame budgets remain the outer
	// guard if no prerequisite can be satisfied.
	//
	// ErrNavigationStalled is also bounded route-search evidence: GoTo emits it
	// only after proving a repeated player state or an excessive sequence of
	// successful map transitions. record() already installs same-state
	// quarantine for that journey, so charging the generic mechanical budget as
	// well can terminate a healthy run before an alternate objective moves the
	// player or advances progression (#2141).
	if routePrerequisite || routeUnavailable || routeSearchExhausted || navigationStalled || transitionExecutionFailed ||
		pushPuzzleSearchExhausted || progressionPrerequisite || trainingInefficient || localInteractionUnavailable ||
		staticUnavailable || economicBlocked {
		// A fully bounded route-search exhaustion is also a planning boundary:
		// GoTo already spent its local replan budget and the failure policy has
		// recorded a same-state quarantine for this exact objective. Charging the
		// generic mechanical-failure budget again terminates healthy runs before
		// another objective/movement can change the routing state (#1843-#1845).
		// The quarantine fails open when no alternative exists, while the normal
		// stagnation/round/frame watchdogs remain the outer bound.
		//
		// A local training-area rejection is the same class of planning
		// boundary as a missing route prerequisite: the executor deliberately
		// sent no gameplay input because this area cannot satisfy the requested
		// training rung within the bounded session. Quarantine/replanning owns
		// the response; spending the fatal mechanical-failure budget here makes
		// a healthy search for a better area terminate as "recovery exhausted".
		//
		// A graph-level no_route/no_path and a stable transition execution failure
		// are also route-planning evidence. The route attempt may have moved the
		// player before discovering the dead end; that must not turn the next
		// planner round into a second mechanical failure. #2173/#2174 measured
		// both forms in live farm runs.
		//
		// A local Talk that reaches no dialogue is likewise a stale/unavailable
		// exploration target, not a controller malfunction. Its quarantine is
		// scoped to the semantic location rather than the player's approach tile,
		// so movement caused by TalkAt does not immediately resurrect it (#2168).
		//
		// Bounded push-puzzle search exhaustion is equivalent route-planning
		// feedback, and a blocked economic purchase is resource/shop feedback
		// whether the planner asked to Buy or a nested restock under Gym/
		// Progress reported cant_afford / not_in_stock / bag_not_risen. These
		// all still use quarantine and the normal stagnation/round/frame watchdogs;
		// actual shop controller faults (menu_stuck, shop_*_stalled, etc.) are
		// deliberately excluded and continue through the mechanical budget.
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
		// A required-battle defeat starts a new, deterministic recovery
		// campaign (heal/restock/train before the retry). Refresh liveness here
		// so a boss reached near the long-stagnation threshold still gets a
		// chance to execute that campaign on the next round. Without this, the
		// watchdog can stop immediately after the blackout, before combat
		// preparation receives a single turn (#2254).
		decision := runFailureDecision{Recovered: true, ProductiveSession: true}
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
