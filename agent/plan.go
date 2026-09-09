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

// Plan is the strategist's bounded, multi-round commitment. Steps are exact
// Objective.String() sentences, never menu indexes: Offer is rebuilt every
// round and indexes are not stable across observations.
type Plan struct {
	Goal  string   `json:"goal,omitempty"`
	Steps []string `json:"steps,omitempty"`
	Step  int      `json:"step,omitempty"`
	Round int      `json:"round,omitempty"`
}

func (p Plan) Active() bool { return p.Step >= 0 && p.Step < len(p.Steps) }

func (p Plan) clone() Plan {
	p.Steps = append([]string(nil), p.Steps...)
	return p
}

// validateStoredPlan checks only the durable shape. A resumed step may be
// stale by design; Run resolves it against the freshly rebuilt menu and skips
// it rather than treating an old checkpoint as corrupt.
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
	for i, step := range p.Steps {
		if strings.TrimSpace(step) == "" {
			return fmt.Errorf("plan step %d is empty", i)
		}
		if len(step) > PlanStepCap {
			return fmt.Errorf("plan step %d is %d bytes, over cap %d", i, len(step), PlanStepCap)
		}
	}
	return nil
}

// validateStrategicPlan canonicalizes a fresh model plan against the menu the
// strategist actually saw. This is intentionally exact: the model may select
// only deterministic objectives already exposed by Offer, and the runtime
// never fuzzy-matches or invents a missing action.
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
	}
	return Plan{Goal: p.Goal, Steps: canonical, Step: 0, Round: round}, nil
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

// resolvePlanStep advances past stale steps and returns the first sentence
// that still resolves against the current menu. Resolution failure is a skip,
// not a failure: a prerequisite may have disappeared because the step is
// already satisfied.
func resolvePlanStep(plan *Plan, offered []Objective) (Objective, int, bool) {
	if plan == nil {
		return Objective{}, 0, false
	}
	skipped := 0
	for plan.Step < len(plan.Steps) {
		obj, err := Chosen(offered, plan.Steps[plan.Step])
		if err == nil {
			return obj, skipped, true
		}
		plan.Step++
		skipped++
	}
	return Objective{}, skipped, false
}

// PlanningStats is run-owned telemetry for the three planning tiers. Counts
// are logical planner operations; endpoint-level retry/failover counts remain
// in LLMStats/LLMHealth where they already live.
type PlanningStats struct {
	Plan             Plan           `json:"plan,omitempty"`
	StrategicCalls   int            `json:"strategic_calls,omitempty"`
	FastCalls        int            `json:"fast_calls,omitempty"`
	PlanExecutions   int            `json:"plan_executions,omitempty"`
	StepsSkipped     int            `json:"steps_skipped,omitempty"`
	LastReplanReason string         `json:"last_replan_reason,omitempty"`
	ReplanReasons    map[string]int `json:"replan_reasons,omitempty"`
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
				if !errors.Is(err, ErrPlanStepUnresolved) {
					return Objective{}, false, err, retries
				}
				// The strategist itself is fine — every re-ask above quoted
				// the actual offered menu back at it — but it kept naming
				// something not on it (stale, invented, or a formatting
				// near-miss Chosen couldn't recover) until the retry budget
				// ran out. That is a round-scoped failure, not a run one:
				// fall back to the single-objective chooser, which asks a
				// far more constrained question (pick one menu index) that
				// this failure mode does not reach. The strategist gets
				// another chance next round; r.pending is untouched.
				if log != nil {
					fmt.Fprintf(log, "round %d: strategist exhausted retries (%v); falling back to the single-objective planner for this round\n", round, err)
				}
				r.Stats.FastCalls++
				obj, ferr, fretries := planWithRetries(log, round, p, obs, offered)
				r.sync()
				return obj, false, ferr, retries + fretries
			}
			r.install(plan, reason)
			obj, skipped, ok := resolvePlanStep(&r.Plan, offered)
			r.Stats.StepsSkipped += skipped
			r.sync()
			if !ok {
				return Objective{}, false, errors.New("agent: strategist returned a plan with no resolvable step"), retries
			}
			r.Stats.PlanExecutions++
			r.sync()
			return obj, true, nil, retries
		}

		if r.Plan.Active() {
			obj, skipped, ok := resolvePlanStep(&r.Plan, offered)
			r.Stats.StepsSkipped += skipped
			r.sync()
			if ok {
				r.Stats.PlanExecutions++
				r.sync()
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

func recoverableFailureReplan(seen map[string]bool, obj Objective, result ObjectiveResult, blackedOut, retreated bool, consecutive, maxConsecutive int) (reason, key string, terminal bool) {
	key = recoverableFailureKey(obj, result)
	if seen[key] || consecutive > maxConsecutive {
		return "", key, true
	}
	reason = "objective_failed"
	if blackedOut {
		reason = "blackout"
	} else if retreated {
		reason = "train_retreat"
	}
	return reason, key, false
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

func (r *runPlanning) success(fromPlan bool) {
	if fromPlan && r.Plan.Step < len(r.Plan.Steps) {
		r.Plan.Step++
	}
	r.sync()
}
