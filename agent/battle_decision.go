package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/game"
)

const DecisionKindBattleTurn = "battle_turn"

// BattleDecisionRequest turns an adapter-built battle turn into a typed
// choice. Choices are exactly the state's derived legal actions, so no backend
// can name a move, switch, item or RUN the adapter did not declare.
func BattleDecisionRequest(s game.BattleDecisionState) (DecisionRequest, error) {
	if err := s.Validate(); err != nil {
		return DecisionRequest{}, err
	}
	raw, err := DecisionState(s)
	if err != nil {
		return DecisionRequest{}, err
	}
	actions := s.Actions()
	choices := make([]DecisionChoice, 0, len(actions))
	for _, action := range actions {
		choices = append(choices, DecisionChoice{ID: action.ID(), Label: battleActionLabel(s, action)})
	}
	req := DecisionRequest{
		Kind:         DecisionKindBattleTurn,
		Question:     "Which legal action should the player take this battle turn?",
		State:        raw,
		Choices:      choices,
		Instructions: "Choose exactly one declared action. Moves, switches, items and run are listed only when they are legal this turn; options marked unusable in state are context, not choices. Prefer winning the battle safely over speed.",
	}
	return req, ValidateDecisionRequest(req)
}

// ResolveBattleDecision is the execution gate: a validated response is mapped
// back through the state's legal set before any caller acts on it.
func ResolveBattleDecision(s game.BattleDecisionState, resp DecisionResponse) (game.BattleAction, error) {
	action, err := s.Legal(resp.Choice)
	if err != nil {
		return game.BattleAction{}, fmt.Errorf("%w: %w", ErrInvalidDecision, err)
	}
	return action, nil
}

func battleActionLabel(s game.BattleDecisionState, a game.BattleAction) string {
	switch a.Kind {
	case game.BattleActionMove:
		for _, mv := range s.Moves {
			if mv.Slot != a.Slot {
				continue
			}
			var parts []string
			if mv.Type != "" {
				parts = append(parts, mv.Type)
			}
			if mv.Power > 0 {
				parts = append(parts, fmt.Sprintf("power %d", mv.Power))
			}
			if mv.Effectiveness > 0 {
				parts = append(parts, fmt.Sprintf("x%g vs opponent", mv.Effectiveness))
			}
			parts = append(parts, fmt.Sprintf("pp %d", mv.PP))
			return fmt.Sprintf("use %s (%s)", mv.Move, strings.Join(parts, ", "))
		}
	case game.BattleActionSwitch:
		for _, sw := range s.Switches {
			if sw.Slot == a.Slot {
				return fmt.Sprintf("switch to %s L%d (%d/%d hp)", sw.Species, sw.Level, sw.HP, sw.MaxHP)
			}
		}
	case game.BattleActionItem:
		return fmt.Sprintf("use %s on party slot %d", a.Item, a.Slot)
	case game.BattleActionRun:
		return "run from the battle"
	}
	return a.ID()
}
