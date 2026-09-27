// Package decision lets a bounded typed-decision backend choose only from
// placements the deterministic Tetris policy already proved reachable.
package decision

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/tetris"
	"github.com/maestroi/pokepilot/tetris/policy"
)

const KindPlacement = "tetris_placement"

// MaxChoicesFromEnv reads the runner-local limit for typed Tetris choices.
func MaxChoicesFromEnv() (int, error) {
	raw := strings.TrimSpace(os.Getenv("POKEPILOT_TETRIS_MAX_CHOICES"))
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 2 {
		return 0, fmt.Errorf("POKEPILOT_TETRIS_MAX_CHOICES must be an integer of at least 2")
	}
	return limit, nil
}

// Selection is one policy-bounded placement decision. Decision is the
// placement that should execute. Deterministic is always the policy fallback.
// DecisionErr is non-nil when the typed backend was unusable and fallback won.
type Selection struct {
	Decision      policy.Decision
	Deterministic policy.Decision
	Request       agent.DecisionRequest
	Response      agent.DecisionResponse
	DecisionErr   error
	Fallback      bool
	Shadow        bool
	Agreed        *bool
}

// Selector consults one typed backend after deterministic legality/scoring.
// Shadow records the backend choice but always executes Deterministic.
type Selector struct {
	Engine        agent.DecisionEngine
	MinConfidence float64
	Shadow        bool
	// MaxChoices limits the request to the policy's best legal placements.
	// Zero keeps the complete candidate set for backends without a choice limit.
	MaxChoices int
}

type placementState struct {
	Mode      tetris.Mode          `json:"mode"`
	Objective policy.Objective     `json:"objective"`
	Board     []string             `json:"board"`
	Active    tetris.PieceState    `json:"active"`
	Next      *tetris.PiecePreview `json:"next,omitempty"`
	Level     int                  `json:"level"`
	Score     int                  `json:"score"`
	Lines     int                  `json:"lines"`
}

func (s Selector) Choose(ctx context.Context, state tetris.State, objective policy.Objective) (Selection, error) {
	resolved, candidates, err := policy.Candidates(state, objective)
	if err != nil {
		return Selection{}, err
	}
	best, ok := policy.BestCandidate(candidates)
	if state.Active == nil {
		return Selection{}, policy.ErrNotReady
	}
	if !ok {
		return Selection{}, fmt.Errorf("%w for %s", policy.ErrNoPlacement, state.Active.Piece)
	}
	deterministic := policy.Decision{
		Objective:  resolved,
		Piece:      state.Active.Piece,
		Candidate:  best,
		Considered: len(candidates),
	}
	req, candidateByID, err := placementRequest(state, resolved, policy.TopCandidates(candidates, s.MaxChoices))
	if err != nil {
		return Selection{}, err
	}
	out := Selection{
		Decision:      deterministic,
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
	if decisionErr == nil && s.MinConfidence > 0 && resp.Confidence < s.MinConfidence {
		decisionErr = fmt.Errorf("%w: %.3f < %.3f", agent.ErrDecisionLowConfidence, resp.Confidence, s.MinConfidence)
	}
	if decisionErr != nil {
		out.Fallback = true
		out.DecisionErr = decisionErr
		return out, nil
	}

	chosen, ok := candidateByID[resp.Choice]
	if !ok {
		// DecideChecked already validates choice membership; keep this local
		// guard so a future request-builder refactor can never escape policy.
		out.Fallback = true
		out.DecisionErr = fmt.Errorf("%w: unknown placement choice %q", agent.ErrInvalidDecision, resp.Choice)
		return out, nil
	}
	same := samePlacement(chosen, deterministic.Candidate)
	out.Agreed = &same
	if s.Shadow {
		return out, nil
	}
	out.Decision.Candidate = chosen
	return out, nil
}

func placementRequest(state tetris.State, objective policy.Objective, candidates []policy.Candidate) (agent.DecisionRequest, map[string]policy.Candidate, error) {
	if state.Active == nil {
		return agent.DecisionRequest{}, nil, policy.ErrNotReady
	}
	choices := make([]agent.DecisionChoice, 0, len(candidates))
	byID := make(map[string]policy.Candidate, len(candidates))
	for _, candidate := range candidates {
		id := placementID(candidate)
		choices = append(choices, agent.DecisionChoice{
			ID:    id,
			Label: fmt.Sprintf("rotation %d, column %d", candidate.Placement.Rotation, candidate.Placement.Column),
			Description: fmt.Sprintf(
				"clears %d lines; line score %d; holes %d; aggregate height %d; max height %d; bumpiness %d; wells %d; input steps %d; deterministic score %d (lookahead %d)",
				candidate.LinesCleared,
				candidate.ExpectedLineScore,
				candidate.Metrics.Holes,
				candidate.Metrics.AggregateHeight,
				candidate.Metrics.MaxHeight,
				candidate.Metrics.Bumpiness,
				candidate.Metrics.Wells,
				candidate.InputSteps,
				candidate.TotalScore,
				candidate.LookaheadScore,
			),
		})
		byID[id] = candidate
	}
	rawState, err := agent.DecisionState(placementState{
		Mode:      state.Mode,
		Objective: objective,
		Board:     boardRows(state.Board),
		Active:    *state.Active,
		Next:      state.Next,
		Level:     state.Level,
		Score:     state.Score,
		Lines:     state.LinesCleared,
	})
	if err != nil {
		return agent.DecisionRequest{}, nil, err
	}
	return agent.DecisionRequest{
		Kind:     KindPlacement,
		Question: "Which already-legal Tetris placement should execute for the current piece?",
		State:    rawState,
		Choices:  choices,
		Instructions: "Choose exactly one declared placement. Prefer the run objective while avoiding holes, dangerous stack height, and needless input. " +
			"The choices are pre-validated reachable placements; do not invent another rotation or column.",
	}, byID, nil
}

func placementID(candidate policy.Candidate) string {
	return fmt.Sprintf("r%d-c%d", candidate.Placement.Rotation, candidate.Placement.Column)
}

func samePlacement(a, b policy.Candidate) bool {
	return a.Placement.Rotation == b.Placement.Rotation && a.Placement.Column == b.Placement.Column
}

func boardRows(board tetris.Board) []string {
	rows := make([]string, tetris.BoardHeight)
	for y := 0; y < tetris.BoardHeight; y++ {
		row := make([]byte, tetris.BoardWidth)
		for x := 0; x < tetris.BoardWidth; x++ {
			if board[y][x] {
				row[x] = '#'
			} else {
				row[x] = '.'
			}
		}
		rows[y] = string(row)
	}
	return rows
}
