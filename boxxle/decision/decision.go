// Package decision lets a bounded typed-decision backend choose only from
// crate pushes the deterministic Boxxle policy already proved legal. The model
// sees a compact symbolic board and a declared set of legal, non-deadlocking
// pushes; it returns one declared push id. Deterministic code owns legality,
// staleness, and execution: the model never emits a raw D-pad sequence, and a
// malformed, impossible, or stale answer is rejected before the executor runs.
package decision

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/policy"
)

// KindPush names the Boxxle push decision for run identity and telemetry.
const KindPush = "boxxle_push"

// Plan is the typed Sokoban decision contract: the single semantic push the
// planner chose, plus an optional short sequence of follow-up pushes the
// deterministic executor may use for lookahead. The model only ever emits the
// first push; Follow is populated by deterministic code, never by the backend.
type Plan struct {
	Push   boxxle.LegalPush   `json:"push"`
	Follow []boxxle.LegalPush `json:"follow,omitempty"`
}

// Selection is one policy-bounded push decision. Plan.Push is the push that
// should execute. Deterministic is always the policy fallback. DecisionErr is
// non-nil when the typed backend was unusable and fallback won; Fallback is
// true in that case. Shadow records the backend choice but always executes
// Deterministic.
type Selection struct {
	Plan          Plan
	Deterministic Plan
	Request       agent.DecisionRequest
	Response      agent.DecisionResponse
	DecisionErr   error
	Fallback      bool
	Shadow        bool
	Agreed        *bool
}

// ErrNoForwardPush is returned as Selection.DecisionErr when every legal push
// would restore a crate layout this puzzle has already occupied. The model is
// not asked; the caller should hand the position to the solver.
var ErrNoForwardPush = errors.New("boxxle decision: every legal push restores a seen layout")

// Selector consults one typed backend after deterministic legality/scoring.
// The backend is selected through the shared agent.DecisionEngine interface, so
// the provider and model are configured by the runner, never hard-coded here.
type Selector struct {
	Engine        agent.DecisionEngine
	MinConfidence float64
	Shadow        bool
	// MaxChoices limits how many legal pushes are declared. Zero keeps the
	// complete candidate set for backends without a choice limit.
	MaxChoices int
	// Omit drops pushes the caller has measured as stalls, such as a push
	// that returns the crates to a layout already reached on this puzzle.
	// The model is asked to solve the room from the pushes that remain.
	Omit func(boxxle.LegalPush) bool
}

// pushState is the compact symbolic board representation fed to the planner.
// It carries only what deterministic code already measured: the board as ASCII
// rows, the player, crates, and goals. No raw RAM, no tile ids.
type pushState struct {
	Screen boxxle.Screen `json:"screen"`
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Board  []string      `json:"board"`
	Player boxxle.Pos    `json:"player"`
	Crates []boxxle.Pos  `json:"crates"`
	Goals  []boxxle.Pos  `json:"goals"`
	Solved bool          `json:"solved"`
}

// Choose selects the next crate push for a decoded board. The deterministic
// policy supplies the fallback; the typed backend may replace it with another
// legal push. The returned Selection is always safe to execute: Plan.Push is a
// legal, non-deadlocking push on the given state, whether it came from the
// backend or the fallback.
func (s Selector) Choose(ctx context.Context, state boxxle.State) (Selection, error) {
	det, err := policy.Choose(state)
	if err != nil {
		return Selection{}, err
	}
	deterministic := Plan{Push: det.Candidate.Push}
	candidates, err := policy.Candidates(state)
	if err != nil {
		return Selection{}, err
	}
	candidates = omitPushes(candidates, s.Omit)
	if len(candidates) == 0 {
		return Selection{
			Plan:          deterministic,
			Deterministic: deterministic,
			Fallback:      true,
			DecisionErr:   ErrNoForwardPush,
			Shadow:        s.Shadow,
		}, nil
	}
	req, byID, err := pushRequest(state, topCandidates(candidates, s.MaxChoices))
	if err != nil {
		return Selection{}, err
	}
	out := Selection{
		Plan:          deterministic,
		Deterministic: deterministic,
		Request:       req,
		Shadow:        s.Shadow,
	}
	if s.Engine == nil {
		out.Fallback = true
		out.DecisionErr = agent.ErrDecisionDisabled
		return out, nil
	}

	resp, decisionErr := agent.DecideChecked(ctx, s.Engine, req)
	out.Response = resp
	if decisionErr == nil && !choiceConfident(resp, s.MinConfidence) {
		decisionErr = fmt.Errorf("%w: %.3f < %.3f", agent.ErrDecisionLowConfidence, resp.Confidence, s.MinConfidence)
	}
	if decisionErr != nil {
		out.Fallback = true
		out.DecisionErr = decisionErr
		return out, nil
	}

	chosen, ok := byID[resp.Choice]
	if !ok {
		// DecideChecked already validates choice membership; keep this local
		// guard so a future request-builder refactor can never escape policy.
		out.Fallback = true
		out.DecisionErr = fmt.Errorf("%w: unknown push choice %q", agent.ErrInvalidDecision, resp.Choice)
		return out, nil
	}
	if err := validatePush(state, chosen); err != nil {
		out.Fallback = true
		out.DecisionErr = err
		return out, nil
	}

	same := samePush(chosen, deterministic.Push)
	out.Agreed = &same
	if s.Shadow {
		return out, nil
	}
	out.Plan = Plan{Push: chosen}
	return out, nil
}

// pushRequest builds the model-facing decision: one declared choice per legal,
// non-deadlocking push, plus the compact symbolic board. It returns the request
// and a map from choice id back to the push it names.
func pushRequest(state boxxle.State, candidates []policy.Candidate) (agent.DecisionRequest, map[string]boxxle.LegalPush, error) {
	if state.Player == nil {
		return agent.DecisionRequest{}, nil, fmt.Errorf("%w: board has no player", agent.ErrInvalidDecision)
	}
	choices := make([]agent.DecisionChoice, 0, len(candidates))
	byID := make(map[string]boxxle.LegalPush, len(candidates))
	for _, c := range candidates {
		id := pushID(c.Push)
		choices = append(choices, agent.DecisionChoice{
			ID:    id,
			Label: fmt.Sprintf("push crate at (%d,%d) %s", c.Push.Crate.X, c.Push.Crate.Y, c.Push.Dir),
			Description: fmt.Sprintf(
				"crate (%d,%d) -> (%d,%d); %s; %d step(s) for the player to reach the crate",
				c.Push.Crate.X, c.Push.Crate.Y, c.Push.CrateTo.X, c.Push.CrateTo.Y,
				goalText(c), c.PlayerDist,
			),
		})
		byID[id] = c.Push
	}
	rawState, err := agent.DecisionState(pushState{
		Screen: state.Screen,
		Width:  state.Width,
		Height: state.Height,
		Board:  boxxle.RenderBoard(state),
		Player: *state.Player,
		Crates: state.Crates,
		Goals:  state.Goals,
		Solved: state.Solved,
	})
	if err != nil {
		return agent.DecisionRequest{}, nil, err
	}
	return agent.DecisionRequest{
		Kind:     KindPush,
		Question: "Which push solves the next step of this Sokoban puzzle?",
		State:    rawState,
		Choices:  choices,
		Instructions: "Solve the room: every crate must sit on a goal. You can push a crate one cell and you cannot pull. " +
			"The board in state is the puzzle (# wall, . floor, @ player, $ crate, * crate on a goal, + empty goal). " +
			"Each choice is a legal push that keeps the puzzle solvable and does not return the crates to a layout already reached. " +
			"Choose exactly one. A crate moving closer to the nearest goal can still be a dead end; pick the push the remaining crates need.",
	}, byID, nil
}

// choiceConfident reports whether the backend's answer should play.
//
// MinConfidence is an absolute floor, sized for a handful of choices. A
// Sokoban step often has many legal pushes, so a model that has decided
// still lands well under 0.65: on the live room it picked one push at 0.51
// and the floor threw that away. A unique leader with at least twice the
// probability of every alternative is that decision, and it plays. A near
// tie under the floor still falls back.
func choiceConfident(resp agent.DecisionResponse, min float64) bool {
	if min <= 0 || resp.Confidence >= min {
		return true
	}
	second := 0.0
	for id, probability := range resp.Probabilities {
		if id == resp.Choice {
			continue
		}
		if probability > second {
			second = probability
		}
	}
	return second > 0 && resp.Confidence > 2*second
}

// omitPushes removes candidates the caller measured as stalls. A nil omit
// keeps the complete set.
func omitPushes(candidates []policy.Candidate, omit func(boxxle.LegalPush) bool) []policy.Candidate {
	if omit == nil {
		return candidates
	}
	kept := make([]policy.Candidate, 0, len(candidates))
	for _, c := range candidates {
		if omit(c.Push) {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}

// validatePush re-checks a model-chosen push against the board it was offered
// on. The choices are built from the same state, so this is a defensive guard
// against a stale or hand-edited answer: the crate must still be where the model
// says, the target cell must still be free, and the push must not deadlock.
func validatePush(state boxxle.State, push boxxle.LegalPush) error {
	board, err := boxxle.NewBoard(state)
	if err != nil {
		return fmt.Errorf("%w: %v", agent.ErrInvalidDecision, err)
	}
	if !board.IsCrate(push.Crate) {
		return fmt.Errorf("%w: crate no longer at %v", agent.ErrInvalidDecision, push.Crate)
	}
	if board.IsWall(push.CrateTo) || board.IsCrate(push.CrateTo) {
		return fmt.Errorf("%w: target %v is not free", agent.ErrInvalidDecision, push.CrateTo)
	}
	if board.PushDeadlock(push) {
		return fmt.Errorf("%w: push %v would deadlock", agent.ErrInvalidDecision, push)
	}
	return nil
}

// pushID is the stable, unambiguous choice id for one legal push: the crate
// position plus the direction. Two distinct legal pushes never share an id.
func pushID(push boxxle.LegalPush) string {
	return fmt.Sprintf("push:%d,%d:%s", push.Crate.X, push.Crate.Y, push.Dir)
}

func samePush(a, b boxxle.LegalPush) bool {
	return a.Crate == b.Crate && a.Dir == b.Dir && a.PlayerFrom == b.PlayerFrom && a.CrateTo == b.CrateTo
}

// topCandidates returns the first n candidates, preserving deterministic order.
// n <= 0 keeps the complete set.
func topCandidates(candidates []policy.Candidate, n int) []policy.Candidate {
	if n <= 0 || n >= len(candidates) {
		return candidates
	}
	return candidates[:n]
}

// goalText reports how a push changes the crate's distance to the nearest
// goal. It is a measurement, not a ranking: the push that solves the room
// is often the one that moves farther away.
func goalText(c policy.Candidate) string {
	switch {
	case c.OntoGoal:
		return "lands on a goal"
	case c.DeltaGoal > 0:
		return fmt.Sprintf("nearest goal becomes %d closer", c.DeltaGoal)
	case c.DeltaGoal < 0:
		return fmt.Sprintf("nearest goal becomes %d farther", -c.DeltaGoal)
	default:
		return "nearest goal stays the same distance"
	}
}

// boardRows is the compact ASCII board fed to the planner.
func boardRows(state boxxle.State) []string {
	return boxxle.RenderBoard(state)
}

// Telemetry accumulates planner statistics across a run: how often the planner
// was consulted (replans), how often it fell back to the deterministic policy,
// how many model outputs were rejected as invalid, and the identity and cost of
// the most recent backend consult. It is safe for concurrent use: the run loop
// consults the planner from a stepping goroutine while the operator reads the
// tally.
type Telemetry struct {
	mu        sync.Mutex
	Replans   int
	Fallbacks int
	Invalid   int
	// Identity and cost of the most recent backend consult.
	Backend string
	Model   string
	Latency time.Duration
	Usage   agent.DecisionUsage
}

// Record folds one selection into the run tally. A selection that fell back
// counts as a replan and a fallback; one rejected as a malformed, impossible,
// or stale model output also counts as an invalid action.
func (t *Telemetry) Record(sel Selection) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Replans++
	if sel.Fallback {
		t.Fallbacks++
	}
	if sel.DecisionErr != nil && errors.Is(sel.DecisionErr, agent.ErrInvalidDecision) {
		t.Invalid++
	}
	if sel.Response.Backend != "" || sel.Response.Model != "" {
		t.Backend = sel.Response.Backend
		t.Model = sel.Response.Model
		t.Latency = sel.Response.Duration
		t.Usage = sel.Response.Usage
	}
}

// Snapshot is a copy of the run tally for reporting.
type Snapshot struct {
	Replans   int                 `json:"replans"`
	Fallbacks int                 `json:"fallbacks"`
	Invalid   int                 `json:"invalid"`
	Backend   string              `json:"backend,omitempty"`
	Model     string              `json:"model,omitempty"`
	Latency   time.Duration       `json:"latency,omitempty"`
	Usage     agent.DecisionUsage `json:"usage,omitempty"`
}

// Snapshot returns a copy of the tally without holding the lock on the caller.
func (t *Telemetry) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return Snapshot{
		Replans:   t.Replans,
		Fallbacks: t.Fallbacks,
		Invalid:   t.Invalid,
		Backend:   t.Backend,
		Model:     t.Model,
		Latency:   t.Latency,
		Usage:     t.Usage,
	}
}
