package agent

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	MaxPlanSteps = 10
	PlanGoalCap  = 200
	PlanStepCap  = 320
)

// ErrPlanStepUnresolved marks a plan step that failed to resolve against
// this round's offered menu (Chosen found no match) — a content mismatch:
// the model named something not currently offered, whether stale (a prior
// round's menu), invented, or a formatting near-miss Chosen couldn't
// recover. It is distinct from a structurally invalid plan (empty goal, a
// menu index used as a step, an oversized field): those indicate the
// strategist itself is malfunctioning and stay a hard stop, while an
// unresolved step still has a working single-objective chooser underneath
// it to fall back on for this round. See choose in plan.go.
var ErrPlanStepUnresolved = errors.New("agent: strategist: plan step does not resolve")

// Plan is the strategist's cached strategic leg. Goal is the longer-lived
// purpose; Steps are only the immediately executable setup actions for the
// current world state. A leg stops at the first discovery/progression boundary
// so objectives learned after that boundary can be considered from fresh
// Observation instead of being shadowed by a stale pre-boundary itinerary.
//
// Steps remain the human-readable sentences the strategist emitted for
// prompt/telemetry compatibility. StepKeys is the durable semantic identity
// captured when a fresh plan is validated. New checkpoints resolve by
// StepKeys; legacy plans without StepKeys retain the historical
// sentence-resolution fallback. Boundary is additive checkpoint metadata, so
// older checkpoints decode with the conservative false value.
type Plan struct {
	Goal        string         `json:"goal,omitempty"`
	Steps       []string       `json:"steps,omitempty"`
	StepKeys    []ObjectiveKey `json:"step_keys,omitempty"`
	Step        int            `json:"step,omitempty"`
	Round       int            `json:"round,omitempty"`
	Boundary    bool           `json:"boundary,omitempty"`
	TailDropped int            `json:"tail_dropped,omitempty"`
}

func (p Plan) Active() bool { return p.Step >= 0 && p.Step < len(p.Steps) }

func (p Plan) clone() Plan {
	p.Steps = append([]string(nil), p.Steps...)
	p.StepKeys = append([]ObjectiveKey(nil), p.StepKeys...)
	return p
}

// validateStoredPlan checks only the durable shape. A resumed step may be
// stale by design; Run resolves it against the freshly rebuilt menu and skips
// it rather than treating an old checkpoint as corrupt. StepKeys may be absent
// only for a migrated legacy checkpoint.
func validateStoredPlan(p Plan) error {
	if len(p.Goal) > PlanGoalCap {
		return fmt.Errorf("plan goal is %d bytes, over cap %d", len(p.Goal), PlanGoalCap)
	}
	if len(p.Steps) > MaxPlanSteps {
		return fmt.Errorf("plan has %d steps, over cap %d", len(p.Steps), MaxPlanSteps)
	}
	if p.Step < 0 || p.Step > len(p.Steps) {
		return fmt.Errorf("plan step %d is outside 0..%d", p.Step, len(p.Steps))
	}
	if len(p.StepKeys) != 0 && len(p.StepKeys) != len(p.Steps) {
		return fmt.Errorf("plan has %d display steps but %d semantic step keys", len(p.Steps), len(p.StepKeys))
	}
	for i, step := range p.Steps {
		if strings.TrimSpace(step) == "" {
			return fmt.Errorf("plan step %d is empty", i)
		}
		if len(step) > PlanStepCap {
			return fmt.Errorf("plan step %d is %d bytes, over cap %d", i, len(step), PlanStepCap)
		}
		if len(p.StepKeys) != 0 {
			if err := p.StepKeys[i].Objective().Validate(); err != nil {
				return fmt.Errorf("plan step %d has invalid semantic key: %w", i, err)
			}
			if len(p.StepKeys[i].Intent) > IntentCap {
				return fmt.Errorf("plan step %d intent is %d bytes, over cap %d", i, len(p.StepKeys[i].Intent), IntentCap)
			}
		}
	}
	return nil
}

// validateStrategicPlan canonicalizes a fresh model plan against the menu the
// strategist actually saw. The model still selects by the exact displayed
// sentence, but once that sentence resolves the runtime captures ObjectiveKey
// and no durable state depends on the wording afterward.
func validateStrategicPlan(p Plan, offered []Objective, round int) (Plan, error) {
	p.Goal = strings.TrimSpace(p.Goal)
	if p.Goal == "" {
		return Plan{}, errors.New("agent: strategist: plan goal is empty")
	}
	if len(p.Goal) > PlanGoalCap {
		return Plan{}, fmt.Errorf("agent: strategist: plan goal is %d bytes, over cap %d", len(p.Goal), PlanGoalCap)
	}
	if len(p.Steps) == 0 {
		return Plan{}, errors.New("agent: strategist: plan has no steps")
	}
	if len(p.Steps) > MaxPlanSteps {
		return Plan{}, fmt.Errorf("agent: strategist: plan has %d steps, over cap %d", len(p.Steps), MaxPlanSteps)
	}
	canonical := make([]string, 0, len(p.Steps))
	keys := make([]ObjectiveKey, 0, len(p.Steps))
	boundary := false
	tailDropped := p.TailDropped
	for i, raw := range p.Steps {
		step := strings.TrimSpace(raw)
		if step == "" {
			return Plan{}, fmt.Errorf("agent: strategist: plan step %d is empty", i+1)
		}
		if len(step) > PlanStepCap {
			return Plan{}, fmt.Errorf("agent: strategist: plan step %d is %d bytes, over cap %d", i+1, len(step), PlanStepCap)
		}
		if _, err := strconv.Atoi(step); err == nil {
			return Plan{}, fmt.Errorf("agent: strategist: plan step %d is menu index %q; steps must be objective sentences", i+1, step)
		}
		obj, err := Chosen(offered, step)
		if err != nil {
			return Plan{}, fmt.Errorf("agent: strategist: plan step %d does not resolve: %w: %w", i+1, ErrPlanStepUnresolved, err)
		}
		canonical = append(canonical, obj.String())
		keys = append(keys, obj.Key())
		// A strategic leg must not pre-commit through a world-state boundary.
		// New maps and progression transactions can expose objectives that were
		// impossible to offer when this plan was authored. Keep the long-range
		// purpose in Goal, but deterministically discard any speculative tail.
		if objectiveEndsStrategicLeg(obj) {
			boundary = true
			tailDropped += len(p.Steps) - len(canonical)
			break
		}
	}
	return Plan{Goal: p.Goal, Steps: canonical, StepKeys: keys, Step: 0, Round: round, Boundary: boundary, TailDropped: tailDropped}, nil
}

// StrategicPlanner is optional. Scripted and simple test planners retain the
// historical one-step loop; real LLM planners implement this interface and
// let Run add the zero-call plan tier without changing Planner itself.
type StrategicPlanner interface {
	Strategize(obs Observation, offered []Objective, reason string) (Plan, error)
}

// StrategicFeedbackPlanner is the strategist equivalent of FeedbackPlanner:
// rejected plan-shaped replies can be re-asked only when the request changes.
type StrategicFeedbackPlanner interface {
	StrategizeRetry(obs Observation, offered []Objective, reason string, r Retry) (Plan, error)
}

func strategizeWithRetries(log io.Writer, round int, p StrategicPlanner, obs Observation, offered []Objective, reason string) (Plan, error, int) {
	plan, err := p.Strategize(obs, offered, reason)
	if err == nil {
		plan, err = validateStrategicPlan(plan, offered, round)
	}
	fp, canRetry := p.(StrategicFeedbackPlanner)
	retries := 0
	for err != nil && !errors.Is(err, ErrDone) && canRetry && retries < MaxReplyRetries-1 {
		r, retryable := classifyRetry(err)
		if !retryable {
			if log != nil {
				fmt.Fprintf(log, "round %d: strategist reply rejected and not retried: %v\n", round, err)
			}
			break
		}
		retries++
		if IsLengthTruncation(err) {
			// Unlike the cheap chooser, strategic planning starts at 8192
			// tokens. Make the third ask genuinely larger than the second.
			r.MaxTokensFactor = 1 << retries
			if r.MaxTokensFactor < 2 {
				r.MaxTokensFactor = 2
			}
		}
		if log != nil {
			fmt.Fprintf(log, "round %d: strategist reply rejected (ask %d of %d): %v; re-ask differs by %s\n",
				round, retries+1, MaxReplyRetries, err, r.describe())
		}
		plan, err = fp.StrategizeRetry(obs, offered, reason, r)
		if err == nil {
			plan, err = validateStrategicPlan(plan, offered, round)
		}
	}
	return plan, err, retries
}

// resolvePlanStep advances past stale steps and returns the first objective
// that still resolves against the current menu. New plans resolve by semantic
// key, so changing presentation wording cannot invalidate a checkpoint. Plans
// loaded from v4 checkpoint memory have no StepKeys and use the old sentence
// resolver until they are replaced by the next strategic plan.
func resolvePlanStep(plan *Plan, offered []Objective) (Objective, int, bool) {
	if plan == nil {
		return Objective{}, 0, false
	}
	skipped := 0
	semantic := len(plan.StepKeys) == len(plan.Steps) && len(plan.StepKeys) != 0
	for plan.Step < len(plan.Steps) {
		if semantic {
			if obj, ok := resolveObjectiveKey(offered, plan.StepKeys[plan.Step]); ok {
				return obj, skipped, true
			}
		} else if obj, err := Chosen(offered, plan.Steps[plan.Step]); err == nil {
			return obj, skipped, true
		}
		plan.Step++
		skipped++
	}
	return Objective{}, skipped, false
}

// objectiveEndsStrategicLeg identifies actions after which the objective menu
// can materially change. Progress/gym/starter transactions commit durable
// story state. Travel is a boundary only when it enters an unvisited adjacent
// map; ordinary movement among known locations may still be useful setup
// inside one cached leg.
func objectiveEndsStrategicLeg(o Objective) bool {
	switch o.Kind {
	case KindProgress, KindGym, KindStarter:
		return true
	case KindGoTo:
		return strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map")
	default:
		return false
	}
}

// strategicLegContinuation is the zero/cheap-call bridge after a cached leg
// crosses a discovery boundary. It deliberately handles only unambiguous
// forward progress:
//   - exactly one currently offered progression transaction is safe to run
//     without another model call;
//   - exactly one unvisited adjacent destination can continue the same leg.
//
// Fight/flee variants for that one destination are returned together so the
// cheap chooser can decide encounter policy without waking the strategist.
// Any real branch is left to the strategist.
func strategicLegContinuation(offered []Objective) (candidates []Objective, reason string) {
	progress := make([]Objective, 0, 2)
	seenProgress := map[string]bool{}
	for _, o := range offered {
		if o.Kind != KindProgress {
			continue
		}
		key := o.Key().ID()
		if !seenProgress[key] {
			seenProgress[key] = true
			progress = append(progress, o)
		}
	}
	if len(progress) == 1 {
		return progress, "single_progression"
	}

	frontierByPlace := map[PlaceID][]Objective{}
	for _, o := range offered {
		if o.Kind != KindGoTo || !strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map") {
			continue
		}
		frontierByPlace[o.Place] = append(frontierByPlace[o.Place], o)
	}
	if len(frontierByPlace) != 1 {
		return nil, ""
	}
	for _, variants := range frontierByPlace {
		return variants, "single_frontier"
	}
	return nil, ""
}

// PlanningStats is run-owned telemetry for the three planning tiers. Counts
// are logical planner operations; endpoint-level retry/failover counts remain
// in LLMStats/LLMHealth where they already live.
type PlanningStats struct {
	Plan                Plan           `json:"plan,omitempty"`
	StrategicCalls      int            `json:"strategic_calls,omitempty"`
	FastCalls           int            `json:"fast_calls,omitempty"`
	PlanExecutions      int            `json:"plan_executions,omitempty"`
	LegAutoExecutions   int            `json:"leg_auto_executions,omitempty"`
	LegFastExecutions   int            `json:"leg_fast_executions,omitempty"`
	LegBoundaries       int            `json:"leg_boundaries,omitempty"`
	LegTailStepsDropped int            `json:"leg_tail_steps_dropped,omitempty"`
	StepsSkipped        int            `json:"steps_skipped,omitempty"`
	LastLegDecision     string         `json:"last_leg_decision,omitempty"`
	LastReplanReason    string         `json:"last_replan_reason,omitempty"`
	ReplanReasons       map[string]int `json:"replan_reasons,omitempty"`
}

func (s PlanningStats) clone() PlanningStats {
	s.Plan = s.Plan.clone()
	if s.ReplanReasons != nil {
		cp := make(map[string]int, len(s.ReplanReasons))
		for k, v := range s.ReplanReasons {
			cp[k] = v
		}
		s.ReplanReasons = cp
	}
	return s
}

// PlanningObserver lets decorators publish run-owned plan state without
// turning telemetry into another planner or introducing a dependency on farm.
type PlanningObserver interface {
	ObservePlanning(PlanningStats)
}

func notifyPlanning(p Planner, stats PlanningStats) {
	if o, ok := p.(PlanningObserver); ok {
		o.ObservePlanning(stats.clone())
	}
}

const maxPlannerTransportAttempts = 2

type runPlanning struct {
	Plan        Plan
	Stats       PlanningStats
	pending     string
	strategized bool
}

func newRunPlanning(plan Plan) *runPlanning {
	r := &runPlanning{Plan: plan.clone(), strategized: plan.Goal != "" || len(plan.Steps) != 0}
	r.Stats.ReplanReasons = map[string]int{}
	r.sync()
	return r
}

func (r *runPlanning) hasStrategist(p Planner) bool {
	_, ok := p.(StrategicPlanner)
	return ok
}

func (r *runPlanning) request(reason string) {
	if reason == "" {
		return
	}
	if r.pending == "" || (r.pending == "objective_failed" && reason != "objective_failed") {
		r.pending = reason
	}
}

func (r *runPlanning) sync() {
	r.Stats.Plan = r.Plan.clone()
}

func (r *runPlanning) snapshot() PlanningStats {
	r.sync()
	return r.Stats.clone()
}

func (r *runPlanning) install(plan Plan, reason string) {
	r.Plan = plan.clone()
	r.strategized = true
	r.pending = ""
	r.Stats.StrategicCalls++
	r.Stats.LegTailStepsDropped += plan.TailDropped
	r.Stats.LastReplanReason = reason
	if r.Stats.ReplanReasons == nil {
		r.Stats.ReplanReasons = map[string]int{}
	}
	r.Stats.ReplanReasons[reason]++
	r.sync()
}

// choose implements the tier order: strategist when a replan is required,
// then a zero-call plan step, then the legacy cheap chooser — either
// because no strategist is available, or because the strategist exhausted
// its retries this round without producing a resolvable plan. The latter
// is a round-scoped degrade, not a run failure: a strategist that keeps
// offering a sentence that isn't on this round's menu (a stale plan step,
// or a hallucinated one) still has a working single-objective chooser
// underneath it, and that chooser asks a far more constrained question
// (pick one menu index) that the same failure mode does not reach. The
// strategist gets another chance next round via r.pending, which this
// leaves untouched. A strategist-produced plan is guaranteed to have at
// least one currently resolvable step by validateStrategicPlan.
func (r *runPlanning) choose(log io.Writer, round int, p Planner, obs Observation, offered []Objective) (Objective, bool, error, int) {
	sp, strategic := p.(StrategicPlanner)
	for {
		// A completed boundary leg keeps its strategic Goal alive while the
		// newly revealed state is unambiguous. This is the latency-saving tier:
		// continue through a single progression/frontier without another
		// strategist call. If the only ambiguity is fight-vs-flee for one
		// frontier destination, ask only the cheap chooser on that tiny menu.
		if strategic && r.pending == "" && !r.Plan.Active() && r.Plan.Boundary {
			if continuation, reason := strategicLegContinuation(offered); len(continuation) > 0 {
				r.Stats.LastLegDecision = reason
				if len(continuation) == 1 {
					r.Stats.LegAutoExecutions++
					r.sync()
					if log != nil {
						fmt.Fprintf(log, "round %d: strategic leg auto-continue goal=%q reason=%s -> %s\n", round, r.Plan.Goal, reason, continuation[0])
					}
					return continuation[0], false, nil, 0
				}
				r.Stats.FastCalls++
				r.Stats.LegFastExecutions++
				if log != nil {
					fmt.Fprintf(log, "round %d: strategic leg cheap continuation goal=%q reason=%s candidates=%d\n", round, r.Plan.Goal, reason, len(continuation))
				}
				obj, err, retries := planWithRetries(log, round, p, obs, continuation)
				r.sync()
				return obj, false, err, retries
			}
		}

		if strategic && (r.pending != "" || !r.Plan.Active()) {
			reason := r.pending
			if reason == "" {
				if r.strategized {
					reason = "plan_exhausted"
				} else {
					reason = "initial"
				}
			}
			plan, err, retries := strategizeWithRetries(log, round, sp, obs, offered, reason)
			if err != nil {
				if !errors.Is(err, ErrPlanStepUnresolved) && !IsLengthTruncation(err) {
					return Objective{}, false, err, retries
				}
				// Two round-scoped failures degrade instead of killing the
				// run: the strategist named something not on this round's
				// menu (ErrPlanStepUnresolved — stale, invented, or a
				// near-miss Chosen couldn't recover), or it kept getting cut
				// off mid-reply (IsLengthTruncation — a reasoning-heavy
				// escalation, e.g. a recovery replan, ate the whole
				// completion budget before emitting the closing JSON; doubling
				// the budget on retry did not catch up). MEASURED on
				// run-1pkm1en1hog5a0: blackout recovery escalated reasoning
				// effort, every retry hit finish_reason "length", and because
				// only ErrPlanStepUnresolved fell back, the run re-entered the
				// same expensive strategist call every round forever instead
				// of degrading. Either way the single-objective chooser asks
				// a far cheaper, more constrained question (pick one menu
				// index, no thinking) that this failure mode does not reach.
				// The strategist gets another chance next round; r.pending is
				// untouched.
				if log != nil {
					fmt.Fprintf(log, "round %d: strategist exhausted retries (%v); falling back to the single-objective planner for this round\n", round, err)
				}
				r.Stats.FastCalls++
				obj, ferr, fretries := planWithRetries(log, round, p, obs, offered)
				r.sync()
				return obj, false, ferr, retries + fretries
			}
			r.install(plan, reason)
			if log != nil {
				fmt.Fprintf(log, "round %d: strategic leg installed reason=%s goal=%q steps=%d boundary=%t dropped_tail_steps=%d\n",
					round, reason, r.Plan.Goal, len(r.Plan.Steps), r.Plan.Boundary, r.Plan.TailDropped)
			}
			obj, skipped, ok := resolvePlanStep(&r.Plan, offered)
			r.Stats.StepsSkipped += skipped
			r.sync()
			if !ok {
				return Objective{}, false, errors.New("agent: strategist returned a plan with no resolvable step"), retries
			}
			r.Stats.PlanExecutions++
			r.Stats.LastLegDecision = "strategist_step"
			r.sync()
			if log != nil {
				fmt.Fprintf(log, "round %d: strategic leg step %d/%d goal=%q -> %s\n",
					round, r.Plan.Step+1, len(r.Plan.Steps), r.Plan.Goal, obj)
			}
			return obj, true, nil, retries
		}

		if r.Plan.Active() {
			obj, skipped, ok := resolvePlanStep(&r.Plan, offered)
			r.Stats.StepsSkipped += skipped
			r.sync()
			if ok {
				r.Stats.PlanExecutions++
				r.Stats.LastLegDecision = "cached_step"
				r.sync()
				if log != nil {
					fmt.Fprintf(log, "round %d: strategic leg cached step %d/%d goal=%q -> %s\n",
						round, r.Plan.Step+1, len(r.Plan.Steps), r.Plan.Goal, obj)
				}
				return obj, true, nil, 0
			}
			if strategic {
				r.request("plan_exhausted")
				continue
			}
		}

		r.Stats.FastCalls++
		obj, err, retries := planWithRetries(log, round, p, obs, offered)
		r.sync()
		return obj, false, err, retries
	}
}

// chooseWithTransportRecovery retries one complete planning operation when all
// endpoint-level routing for that operation failed with ErrTransport. No game
// input has happened yet, so re-asking against the same Observation/menu is
// safe and does not consume a gameplay round. This is deliberately separate
// from reply retries: changing temperature/feedback cannot repair transport,
// while a fresh primary->fallback attempt can survive a transient backend
// timeout (#1842). The bound keeps a persistent outage terminal.
func (r *runPlanning) chooseWithTransportRecovery(log io.Writer, round int, p Planner, obs Observation, offered []Objective) (Objective, bool, error, int) {
	totalReplyRetries := 0
	for attempt := 1; attempt <= maxPlannerTransportAttempts; attempt++ {
		obj, fromPlan, err, replyRetries := r.choose(log, round, p, obs, offered)
		totalReplyRetries += replyRetries
		if err == nil || !errors.Is(err, ErrTransport) || attempt == maxPlannerTransportAttempts {
			return obj, fromPlan, err, totalReplyRetries
		}
		if log != nil {
			fmt.Fprintf(log, "round %d: planner transport failed after endpoint failover (attempt %d of %d): %v; retrying unchanged planning state\n",
				round, attempt, maxPlannerTransportAttempts, err)
		}
	}
	return Objective{}, false, fmt.Errorf("%w: planning transport retry invariant", ErrTransport), totalReplyRetries
}

// replanOnce converts a watchdog edge into one strategic replan opportunity.
// The second edge before observable progress is terminal.
func replanOnce(escalated *bool) bool {
	if escalated == nil || *escalated {
		return false
	}
	*escalated = true
	return true
}

func (r *runPlanning) success(fromPlan bool, obj Objective) (boundary bool, dropped int) {
	if fromPlan && r.Plan.Step < len(r.Plan.Steps) {
		r.Plan.Step++
	}
	// Boundary continuations are part of the cached leg even though they are
	// not literal stored steps. Count their state transitions too; otherwise
	// telemetry would under-report the exact zero/cheap-call path this layer
	// exists to make visible.
	if objectiveEndsStrategicLeg(obj) && (fromPlan || r.Plan.Boundary) {
		boundary = true
		r.Plan.Boundary = true
		if fromPlan && r.Plan.Step < len(r.Plan.Steps) {
			dropped = len(r.Plan.Steps) - r.Plan.Step
			r.Plan.Steps = append([]string(nil), r.Plan.Steps[:r.Plan.Step]...)
			if len(r.Plan.StepKeys) >= r.Plan.Step {
				r.Plan.StepKeys = append([]ObjectiveKey(nil), r.Plan.StepKeys[:r.Plan.Step]...)
			}
			r.Plan.TailDropped += dropped
		}
		r.Stats.LegBoundaries++
		r.Stats.LegTailStepsDropped += dropped
		r.Stats.LastLegDecision = "boundary_reached"
	}
	r.sync()
	return boundary, dropped
}
