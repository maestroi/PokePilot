package decision

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/policy"
)

type fakeEngine struct {
	decide func(agent.DecisionRequest) (agent.DecisionResponse, error)
	req    agent.DecisionRequest
}

func (e *fakeEngine) Decide(_ context.Context, req agent.DecisionRequest) (agent.DecisionResponse, error) {
	e.req = req
	return e.decide(req)
}

// buildState converts a compact ASCII grid into a boxxle.State for tests:
//
//	# = wall, . = floor, @ = player, $ = crate, * = crate on goal, + = goal
func buildState(t *testing.T, g []string) boxxle.State {
	t.Helper()
	h := len(g)
	w := 0
	for _, row := range g {
		if len(row) > w {
			w = len(row)
		}
	}
	var walls, goals, crates []boxxle.Pos
	var player *boxxle.Pos
	for y, row := range g {
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case '#':
				walls = append(walls, boxxle.Pos{X: x, Y: y})
			case '@':
				player = &boxxle.Pos{X: x, Y: y}
			case '$':
				crates = append(crates, boxxle.Pos{X: x, Y: y})
			case '*':
				crates = append(crates, boxxle.Pos{X: x, Y: y})
				goals = append(goals, boxxle.Pos{X: x, Y: y})
			case '+':
				goals = append(goals, boxxle.Pos{X: x, Y: y})
			}
		}
	}
	return boxxle.State{
		Screen: boxxle.ScreenPuzzle,
		Width:  w,
		Height: h,
		Walls:  walls,
		Goals:  goals,
		Crates: crates,
		Player: player,
	}
}

// testGrid is a board with at least two legal, non-deadlocking crate pushes so
// the model can pick one the deterministic policy did not.
var testGrid = []string{
	"#########",
	"#.$..$..#",
	"#.@.....#",
	"#....+..#",
	"#########",
}

func responseFor(req agent.DecisionRequest, choice string, selected float64) agent.DecisionResponse {
	probs := make(map[string]float64, len(req.Choices))
	other := 0.0
	if len(req.Choices) > 1 {
		other = (1 - selected) / float64(len(req.Choices)-1)
	}
	for _, c := range req.Choices {
		probs[c.ID] = other
	}
	probs[choice] = selected
	return agent.DecisionResponse{
		Choice:        choice,
		Probabilities: probs,
		Backend:       "system-one-local",
		Model:         "27b-test",
		Duration:      42 * time.Millisecond,
		Usage:         agent.DecisionUsage{PromptTokens: 10, CompletionTokens: 3},
	}
}

func TestSelectorActiveCanChooseDifferentLegalPush(t *testing.T) {
	// A board with two legal, non-deadlocking pushes: the model picks the one
	// the deterministic policy did not.
	state := buildState(t, testGrid)
	det, err := policy.Choose(state)
	if err != nil {
		t.Fatal(err)
	}
	detID := pushID(det.Candidate.Push)
	candidates, err := policy.Candidates(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) < 2 {
		t.Fatalf("need at least two legal pushes, got %d", len(candidates))
	}
	engine := &fakeEngine{}
	engine.decide = func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		for _, choice := range req.Choices {
			if choice.ID != detID {
				return responseFor(req, choice.ID, 0.8), nil
			}
		}
		t.Fatal("no alternate legal push")
		return agent.DecisionResponse{}, nil
	}

	got, err := (Selector{Engine: engine, MinConfidence: 0.65}).Choose(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fallback || got.DecisionErr != nil || got.Shadow {
		t.Fatalf("selection flags = fallback=%v err=%v shadow=%v", got.Fallback, got.DecisionErr, got.Shadow)
	}
	if pushID(got.Plan.Push) == detID {
		t.Fatalf("active backend choice did not replace deterministic push %s", detID)
	}
	if got.Response.Backend != "system-one-local" || got.Response.Model != "27b-test" {
		t.Fatalf("response identity = backend %q model %q", got.Response.Backend, got.Response.Model)
	}
	if got.Response.Confidence < 0.79 {
		t.Fatalf("confidence = %v, want >= 0.79", got.Response.Confidence)
	}
	if got.Agreed == nil || *got.Agreed {
		t.Fatalf("agreement = %v, want false for alternate push", got.Agreed)
	}
	if got.Request.Kind != KindPush {
		t.Fatalf("request kind = %q, want %q", got.Request.Kind, KindPush)
	}
	// The chosen push must still be legal on the board.
	if err := validatePush(state, got.Plan.Push); err != nil {
		t.Fatalf("chosen push is not legal: %v", err)
	}
}

func TestSelectorAgreesWithDeterministic(t *testing.T) {
	state := buildState(t, testGrid)
	det, err := policy.Choose(state)
	if err != nil {
		t.Fatal(err)
	}
	detID := pushID(det.Candidate.Push)
	engine := &fakeEngine{decide: func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		return responseFor(req, detID, 0.9), nil
	}}
	got, err := (Selector{Engine: engine, MinConfidence: 0.5}).Choose(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fallback || got.DecisionErr != nil {
		t.Fatalf("fallback=%v err=%v", got.Fallback, got.DecisionErr)
	}
	if got.Agreed == nil || !*got.Agreed {
		t.Fatalf("agreement = %v, want true", got.Agreed)
	}
	if pushID(got.Plan.Push) != detID {
		t.Fatalf("plan push = %s, want deterministic %s", pushID(got.Plan.Push), detID)
	}
}

func TestSelectorLowConfidenceFallsBackDeterministic(t *testing.T) {
	state := buildState(t, testGrid)
	det, err := policy.Choose(state)
	if err != nil {
		t.Fatal(err)
	}
	engine := &fakeEngine{decide: func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		return responseFor(req, req.Choices[0].ID, 0.4), nil
	}}
	got, err := (Selector{Engine: engine, MinConfidence: 0.7}).Choose(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Fallback || !errors.Is(got.DecisionErr, agent.ErrDecisionLowConfidence) {
		t.Fatalf("fallback=%v err=%v", got.Fallback, got.DecisionErr)
	}
	if pushID(got.Plan.Push) != pushID(det.Candidate.Push) {
		t.Fatalf("fallback push = %s, want deterministic %s", pushID(got.Plan.Push), pushID(det.Candidate.Push))
	}
}

func TestSelectorInvalidBackendChoiceFallsBack(t *testing.T) {
	state := buildState(t, testGrid)
	det, err := policy.Choose(state)
	if err != nil {
		t.Fatal(err)
	}
	engine := &fakeEngine{decide: func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		// A probability distribution that names an undeclared choice.
		return agent.DecisionResponse{Choice: "push:9,9:up", Probabilities: map[string]float64{"push:9,9:up": 1}}, nil
	}}
	got, err := (Selector{Engine: engine}).Choose(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Fallback || !errors.Is(got.DecisionErr, agent.ErrInvalidDecision) {
		t.Fatalf("fallback=%v err=%v", got.Fallback, got.DecisionErr)
	}
	if pushID(got.Plan.Push) != pushID(det.Candidate.Push) {
		t.Fatalf("fallback push = %s, want deterministic %s", pushID(got.Plan.Push), pushID(det.Candidate.Push))
	}
}

func TestSelectorNoEngineFallsBack(t *testing.T) {
	state := buildState(t, testGrid)
	got, err := (Selector{}).Choose(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Fallback || !errors.Is(got.DecisionErr, agent.ErrDecisionDisabled) {
		t.Fatalf("fallback=%v err=%v", got.Fallback, got.DecisionErr)
	}
}

func TestSelectorShadowKeepsDeterministic(t *testing.T) {
	state := buildState(t, testGrid)
	det, err := policy.Choose(state)
	if err != nil {
		t.Fatal(err)
	}
	detID := pushID(det.Candidate.Push)
	candidates, _ := policy.Candidates(state)
	altID := ""
	for _, c := range candidates {
		if pushID(c.Push) != detID {
			altID = pushID(c.Push)
			break
		}
	}
	if altID == "" {
		t.Fatal("need an alternate push for the shadow test")
	}
	engine := &fakeEngine{decide: func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		return responseFor(req, altID, 0.8), nil
	}}
	got, err := (Selector{Engine: engine, Shadow: true}).Choose(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Shadow {
		t.Fatal("expected shadow flag")
	}
	// Shadow records the backend choice but executes the deterministic push.
	if pushID(got.Plan.Push) != detID {
		t.Fatalf("shadow plan push = %s, want deterministic %s", pushID(got.Plan.Push), detID)
	}
	// The response still carries the backend's answer for telemetry.
	if got.Response.Choice != altID {
		t.Fatalf("shadow response choice = %s, want %s", got.Response.Choice, altID)
	}
	if got.Agreed == nil || *got.Agreed {
		t.Fatalf("agreement = %v, want false for alternate push", got.Agreed)
	}
}

func TestSelectorSolvedBoard(t *testing.T) {
	g := []string{
		"#####",
		"#*..#",
		"#...#",
		"#####",
	}
	state := buildState(t, g)
	state.Player = &boxxle.Pos{X: 2, Y: 2}
	state.Solved = true
	engine := &fakeEngine{decide: func(req agent.DecisionRequest) (agent.DecisionResponse, error) {
		return responseFor(req, req.Choices[0].ID, 0.9), nil
	}}
	if _, err := (Selector{Engine: engine}).Choose(context.Background(), state); err == nil {
		t.Fatal("expected error for solved board")
	}
}

func TestValidatePushRejectsStale(t *testing.T) {
	state := buildState(t, testGrid)
	candidates, err := policy.Candidates(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("need a candidate")
	}
	// A legal push validates.
	if err := validatePush(state, candidates[0].Push); err != nil {
		t.Fatalf("legal push rejected: %v", err)
	}
	// A stale push: the crate is no longer where the model says.
	stale := candidates[0].Push
	stale.Crate = boxxle.Pos{X: 99, Y: 99}
	stale.CrateTo = boxxle.Pos{X: 99, Y: 98}
	if err := validatePush(state, stale); err == nil {
		t.Fatal("expected stale push to be rejected")
	}
}

func TestPushIDUnique(t *testing.T) {
	state := buildState(t, testGrid)
	candidates, err := policy.Candidates(state)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, c := range candidates {
		id := pushID(c.Push)
		if seen[id] {
			t.Fatalf("duplicate push id %q", id)
		}
		seen[id] = true
	}
}

func TestBoardRows(t *testing.T) {
	// Exercises every tile type: wall, floor, player, crate, crate-on-goal, goal.
	g := []string{
		"######",
		"#@$*+#",
		"#...##",
		"######",
	}
	state := buildState(t, g)
	rows := boardRows(state)
	if len(rows) != len(g) {
		t.Fatalf("got %d rows, want %d", len(rows), len(g))
	}
	for y, row := range rows {
		if row != g[y] {
			t.Errorf("row %d = %q, want %q", y, row, g[y])
		}
	}
}

func TestTelemetryRecords(t *testing.T) {
	var tel Telemetry
	// A successful backend choice.
	tel.Record(Selection{
		Plan:     Plan{Push: boxxle.LegalPush{Crate: boxxle.Pos{X: 1, Y: 1}}},
		Response: agent.DecisionResponse{Backend: "b", Model: "m", Duration: 5 * time.Millisecond, Usage: agent.DecisionUsage{PromptTokens: 7}},
	})
	// A fallback (no engine).
	tel.Record(Selection{Fallback: true, DecisionErr: agent.ErrDecisionDisabled})
	// An invalid model output.
	tel.Record(Selection{Fallback: true, DecisionErr: fmtErrorInvalid()})

	snap := tel.Snapshot()
	if snap.Replans != 3 {
		t.Errorf("replans = %d, want 3", snap.Replans)
	}
	if snap.Fallbacks != 2 {
		t.Errorf("fallbacks = %d, want 2", snap.Fallbacks)
	}
	if snap.Invalid != 1 {
		t.Errorf("invalid = %d, want 1", snap.Invalid)
	}
	if snap.Backend != "b" || snap.Model != "m" {
		t.Errorf("identity = %q/%q, want b/m", snap.Backend, snap.Model)
	}
	if snap.Usage.PromptTokens != 7 {
		t.Errorf("usage = %+v, want prompt tokens 7", snap.Usage)
	}
}

func TestTelemetryNilSafe(t *testing.T) {
	var tel *Telemetry
	tel.Record(Selection{})
	if snap := tel.Snapshot(); snap.Replans != 0 {
		t.Errorf("nil telemetry snapshot = %+v", snap)
	}
}

// fmtErrorInvalid wraps agent.ErrInvalidDecision the way the selector does.
func fmtErrorInvalid() error {
	return fmt.Errorf("%w: unknown push choice %q", agent.ErrInvalidDecision, "x")
}
