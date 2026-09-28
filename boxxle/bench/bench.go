// Package bench is the ROM-free benchmark harness for Boxxle (Sokoban)
// planning. It drives a planner — any Proposer, from the greedy policy to a
// model plan via the boxxle/decision backend — against the checked-in
// solver fixtures, counts the metrics the #2095 epic asks for, and compares
// the result against the deterministic solver reference.
//
// The harness owns no game semantics beyond what boxxle.Board already
// exposes: legality, deadlock detection, and path lengths all come from the
// board. The solver (boxxle/solver) is the oracle; the harness measures how
// far a planner's plans get from it.
package bench

import (
	"fmt"
	"sort"
	"time"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/solver"
)

// PlannerCall records one planner invocation: how long it took, what it cost
// in tokens (zero for local/deterministic planners), and which backend
// answered. Err is non-nil when the planner failed to produce a proposal.
type PlannerCall struct {
	Latency          time.Duration
	PromptTokens     int
	CompletionTokens int
	Backend          string
	Model            string
	Err              error
}

// Proposer proposes one crate push for a board state. It is the seam where a
// model planner (boxxle/decision) plugs in: the proposer returns a crate and
// a direction, and the harness checks the proposal against the board.
type Proposer func(state boxxle.State) (boxxle.Pos, boxxle.Direction, PlannerCall, error)

// Run is the outcome of driving one puzzle with one proposer.
type Run struct {
	Puzzle         string
	Solved         bool
	Pushes         int
	WalkingMoves   int
	InvalidPushes  int
	Deadlocks      int
	Replans        int
	Restarts       int
	CallFailures   int
	FallbackPushes int
	UsedFallback   bool
	Calls          []PlannerCall
	Duration       time.Duration
}

// FallbackConfig controls when the harness hands a stuck run over to the
// deterministic solver. All thresholds are inclusive: reaching any of the
// non-zero limits triggers the hand-off. A zero value disables the fallback.
type FallbackConfig struct {
	Enabled      bool
	MaxInvalid   int // hand off after this many invalid proposed pushes
	MaxDeadlocks int // hand off after this many deadlocking proposed pushes
	MaxActions   int // hand off after this many total (applied + rejected) actions
}

// ShouldHandOff reports whether the fallback should take over the run.
func (f FallbackConfig) ShouldHandOff(r *Run) bool {
	if !f.Enabled {
		return false
	}
	if f.MaxInvalid > 0 && r.InvalidPushes >= f.MaxInvalid {
		return true
	}
	if f.MaxDeadlocks > 0 && r.Deadlocks >= f.MaxDeadlocks {
		return true
	}
	if f.MaxActions > 0 {
		actions := r.Pushes + r.InvalidPushes + r.Deadlocks + r.CallFailures
		if actions >= f.MaxActions {
			return true
		}
	}
	return false
}

// maxActions is the safety cap on a single Drive loop. A proposer that never
// solves and never trips a fallback threshold would otherwise spin forever;
// the cap turns that into an unsolved run.
const maxActions = 10000

// Drive runs one puzzle to completion (or to the safety cap) under the given
// proposer and fallback policy. It is the measurement unit of the harness:
// every counter in Run is produced here.
//
// The loop is: check solved, check fallback hand-off, ask the proposer for a
// push, validate it against the board (rejecting illegal and deadlocking
// pushes without applying them), and apply it with the player walk counted
// from the board's path.
func Drive(puzzle solver.Puzzle, propose Proposer, fb FallbackConfig) Run {
	run := Run{Puzzle: puzzle.Name}
	start := time.Now()
	state := puzzle.State

	for actions := 0; actions < maxActions; actions++ {
		if state.Solved {
			run.Solved = true
			break
		}
		if fb.ShouldHandOff(&run) {
			state = run.finishWithFallback(state)
			break
		}

		crate, dir, call, err := propose(state)
		run.Calls = append(run.Calls, call)
		if err != nil {
			run.CallFailures++
			continue
		}

		board, err := boxxle.NewBoard(state)
		if err != nil {
			run.CallFailures++
			continue
		}
		lp, ok := board.FindPush(crate, dir)
		if !ok {
			run.InvalidPushes++
			continue
		}
		if board.PushDeadlock(lp) {
			run.Deadlocks++
			continue
		}

		if path, ok := board.PathTo(*state.Player, lp.PlayerFrom); ok {
			run.WalkingMoves += len(path)
		}
		next, err := solver.Apply(state, solver.Push{
			CrateFrom:  lp.Crate,
			CrateTo:    lp.CrateTo,
			Dir:        lp.Dir,
			PlayerFrom: lp.PlayerFrom,
		})
		if err != nil {
			run.CallFailures++
			continue
		}
		state = next
		run.Pushes++
	}

	run.Duration = time.Since(start)
	if len(run.Calls) > 1 {
		run.Replans = len(run.Calls) - 1
	}
	if state.Solved {
		run.Solved = true
	}
	return run
}

// finishWithFallback hands the run over to the deterministic solver and
// applies its solution. Fallback pushes are counted separately so telemetry
// can distinguish model work from oracle work. It returns the final state so
// the caller can see the solved board.
func (r *Run) finishWithFallback(state boxxle.State) boxxle.State {
	sol, ok, err := solver.Solve(state)
	if err != nil || !ok {
		return state
	}
	r.UsedFallback = true
	for _, p := range sol.Pushes {
		next, err := solver.Apply(state, p)
		if err != nil {
			return state
		}
		state = next
		r.FallbackPushes++
		r.Pushes++
		r.WalkingMoves += p.Walk
	}
	return state
}

// ReferenceRun solves a puzzle with the deterministic solver and reports it
// as a run. It is the baseline every model run is compared against: minimum
// pushes, the walk count of that solution, zero planner calls.
func ReferenceRun(puzzle solver.Puzzle) Run {
	run := Run{Puzzle: puzzle.Name}
	sol, ok, err := solver.Solve(puzzle.State)
	if err != nil || !ok {
		return run
	}
	state := puzzle.State
	for _, p := range sol.Pushes {
		next, err := solver.Apply(state, p)
		if err != nil {
			return Run{Puzzle: puzzle.Name}
		}
		state = next
	}
	run.Solved = state.Solved
	run.Pushes = sol.PushCount()
	run.WalkingMoves = sol.WalkCount()
	return run
}

// Metrics is the per-puzzle row of a report.
type Metrics struct {
	Puzzle           string  `json:"puzzle"`
	Solved           bool    `json:"solved"`
	Pushes           int     `json:"pushes"`
	WalkingMoves     int     `json:"walking_moves"`
	ModelCalls       int     `json:"model_calls"`
	Replans          int     `json:"replans"`
	InvalidPushes    int     `json:"invalid_pushes"`
	Deadlocks        int     `json:"deadlocks"`
	Restarts         int     `json:"restarts"`
	FallbackPushes   int     `json:"fallback_pushes"`
	UsedFallback     bool    `json:"used_fallback"`
	LatencyP50MS     float64 `json:"latency_p50_ms"`
	LatencyP95MS     float64 `json:"latency_p95_ms"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	InferenceTimeMS  float64 `json:"inference_time_ms"`
	ReferencePushes  int     `json:"reference_pushes"`
	SolutionRatio    float64 `json:"solution_ratio"` // pushes / reference pushes; 0 without a reference
}

// Distribution summarizes a per-puzzle metric across a report.
type Distribution struct {
	Count  int     `json:"count"`
	Min    int     `json:"min"`
	Max    int     `json:"max"`
	Median float64 `json:"median"`
}

// Report is the aggregate benchmark output for one policy over a set of
// puzzles.
type Report struct {
	Policy                string       `json:"policy"`
	Puzzles               int          `json:"puzzles"`
	Solved                int          `json:"solved"`
	SolveRate             float64      `json:"solve_rate"`
	SolvedWithoutFallback int          `json:"solved_without_fallback"`
	WithoutFallbackRate   float64      `json:"without_fallback_rate"`
	Pushes                Distribution `json:"pushes"`
	WalkingMoves          Distribution `json:"walking_moves"`
	ModelCalls            Distribution `json:"model_calls"`
	Replans               Distribution `json:"replans"`
	InvalidPushes         Distribution `json:"invalid_pushes"`
	Deadlocks             Distribution `json:"deadlocks"`
	Restarts              Distribution `json:"restarts"`
	LatencyP50MS          float64      `json:"latency_p50_ms"`
	LatencyP95MS          float64      `json:"latency_p95_ms"`
	AvgSolutionRatio      float64      `json:"avg_solution_ratio"`
	Rows                  []Metrics    `json:"rows"`
}

// Aggregate folds per-puzzle runs into a report. reference maps puzzle name
// to the solver's minimum push count; puzzles absent from it (unsolvable)
// get a zero SolutionRatio.
func Aggregate(policy string, runs []Run, reference map[string]int) Report {
	rep := Report{Policy: policy, Puzzles: len(runs), Rows: make([]Metrics, 0, len(runs))}
	var latencies []float64

	pushes := make([]int, 0, len(runs))
	walks := make([]int, 0, len(runs))
	calls := make([]int, 0, len(runs))
	replans := make([]int, 0, len(runs))
	invalid := make([]int, 0, len(runs))
	deadlocks := make([]int, 0, len(runs))
	restarts := make([]int, 0, len(runs))
	ratioSum := 0.0
	ratioN := 0

	for _, r := range runs {
		m := Metrics{
			Puzzle:          r.Puzzle,
			Solved:          r.Solved,
			Pushes:          r.Pushes,
			WalkingMoves:    r.WalkingMoves,
			ModelCalls:      len(r.Calls),
			Replans:         r.Replans,
			InvalidPushes:   r.InvalidPushes,
			Deadlocks:       r.Deadlocks,
			Restarts:        r.Restarts,
			FallbackPushes:  r.FallbackPushes,
			UsedFallback:    r.UsedFallback,
			ReferencePushes: reference[r.Puzzle],
			InferenceTimeMS: float64(totalLatency(r.Calls).Microseconds()) / 1000,
		}
		for _, c := range r.Calls {
			latencies = append(latencies, float64(c.Latency.Microseconds())/1000)
			m.PromptTokens += c.PromptTokens
			m.CompletionTokens += c.CompletionTokens
		}
		if len(r.Calls) > 0 {
			m.LatencyP50MS = percentile(latenciesFor(r.Calls), 0.50)
			m.LatencyP95MS = percentile(latenciesFor(r.Calls), 0.95)
		}
		if r.Solved && reference[r.Puzzle] > 0 {
			m.SolutionRatio = float64(r.Pushes) / float64(reference[r.Puzzle])
			ratioSum += m.SolutionRatio
			ratioN++
		}
		if r.Solved {
			rep.Solved++
		}
		if r.Solved && !r.UsedFallback {
			rep.SolvedWithoutFallback++
		}
		rep.Rows = append(rep.Rows, m)
		pushes = append(pushes, r.Pushes)
		walks = append(walks, r.WalkingMoves)
		calls = append(calls, len(r.Calls))
		replans = append(replans, r.Replans)
		invalid = append(invalid, r.InvalidPushes)
		deadlocks = append(deadlocks, r.Deadlocks)
		restarts = append(restarts, r.Restarts)
	}

	if rep.Puzzles > 0 {
		rep.SolveRate = float64(rep.Solved) / float64(rep.Puzzles)
		rep.WithoutFallbackRate = float64(rep.SolvedWithoutFallback) / float64(rep.Puzzles)
	}
	if ratioN > 0 {
		rep.AvgSolutionRatio = ratioSum / float64(ratioN)
	}
	rep.Pushes = distribution(pushes)
	rep.WalkingMoves = distribution(walks)
	rep.ModelCalls = distribution(calls)
	rep.Replans = distribution(replans)
	rep.InvalidPushes = distribution(invalid)
	rep.Deadlocks = distribution(deadlocks)
	rep.Restarts = distribution(restarts)
	if len(latencies) > 0 {
		rep.LatencyP50MS = percentile(latencies, 0.50)
		rep.LatencyP95MS = percentile(latencies, 0.95)
	}
	return rep
}

// CompareText renders a human-readable comparison of a model report against
// the solver reference report.
func CompareText(model, reference Report) string {
	out := &stringBuilder{}
	out.line("boxxle benchmark: %s vs solver reference", model.Policy)
	out.line("  puzzles:            %d", model.Puzzles)
	out.line("  solve rate:         %.1f%% (reference %.1f%%)",
		100*model.SolveRate, 100*reference.SolveRate)
	out.line("  solved w/o fallback: %d/%d (%.1f%%)",
		model.SolvedWithoutFallback, model.Puzzles, 100*model.WithoutFallbackRate)
	out.line("  avg solution ratio: %.2f (1.0 = optimal)", model.AvgSolutionRatio)
	out.line("  pushes:             median %.1f (min %d, max %d)",
		model.Pushes.Median, model.Pushes.Min, model.Pushes.Max)
	out.line("  walking moves:      median %.1f (min %d, max %d)",
		model.WalkingMoves.Median, model.WalkingMoves.Min, model.WalkingMoves.Max)
	out.line("  model calls:        median %.1f (min %d, max %d)",
		model.ModelCalls.Median, model.ModelCalls.Min, model.ModelCalls.Max)
	out.line("  replans:            median %.1f (min %d, max %d)",
		model.Replans.Median, model.Replans.Min, model.Replans.Max)
	out.line("  invalid pushes:     median %.1f (min %d, max %d)",
		model.InvalidPushes.Median, model.InvalidPushes.Min, model.InvalidPushes.Max)
	out.line("  deadlocks caused:   median %.1f (min %d, max %d)",
		model.Deadlocks.Median, model.Deadlocks.Min, model.Deadlocks.Max)
	out.line("  latency:            p50 %.1f ms, p95 %.1f ms",
		model.LatencyP50MS, model.LatencyP95MS)
	return out.string()
}

func totalLatency(calls []PlannerCall) time.Duration {
	var total time.Duration
	for _, c := range calls {
		total += c.Latency
	}
	return total
}

func latenciesFor(calls []PlannerCall) []float64 {
	out := make([]float64, 0, len(calls))
	for _, c := range calls {
		out = append(out, float64(c.Latency.Microseconds())/1000)
	}
	return out
}

// percentile returns the p-quantile (0..1) of a slice using linear
// interpolation on the sorted values.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(pos)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func distribution(values []int) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	return Distribution{
		Count:  len(sorted),
		Min:    sorted[0],
		Max:    sorted[len(sorted)-1],
		Median: percentile(toFloats(sorted), 0.50),
	}
}

func toFloats(values []int) []float64 {
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = float64(v)
	}
	return out
}

// stringBuilder is a tiny helper for CompareText.
type stringBuilder struct {
	s string
}

func (b *stringBuilder) line(format string, args ...interface{}) {
	b.s += fmt.Sprintf(format, args...) + "\n"
}

func (b *stringBuilder) string() string {
	return b.s
}
