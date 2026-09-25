package agent

import (
	"context"
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// BattleMoveDecider adapts a DecisionEngine to skill.MovePolicy, the existing
// move-only seam. Any disabled engine, transport error, invalid or undeclared
// choice, or low-confidence answer falls back to the deterministic policy, so
// Battle keeps pressing only a slot BattleState.Usable allows.
type BattleMoveDecider struct {
	Engine        DecisionEngine
	RomData       []byte
	MinConfidence float64
	Fallback      skill.MovePolicy

	Last      DecisionResponse
	Calls     int
	Rejected  int
	Fallbacks int
}

// Policy returns the MovePolicy to hand to Battle. With no engine it returns
// the fallback itself, leaving existing battle behavior byte-for-byte intact.
func (d *BattleMoveDecider) Policy() skill.MovePolicy {
	fallback := d.Fallback
	if fallback == nil {
		fallback = skill.FirstUsableMove
	}
	if d.Engine == nil {
		return fallback
	}
	return func(b state.BattleState) int {
		slot, err := d.decide(b)
		if err != nil {
			d.Fallbacks++
			return fallback(b)
		}
		return slot
	}
}

func (d *BattleMoveDecider) decide(b state.BattleState) (int, error) {
	s, err := skill.MoveOnlyBattleDecisionState(d.RomData, b)
	if err != nil {
		return -1, err
	}
	req, err := BattleDecisionRequest(s)
	if err != nil {
		return -1, err
	}
	d.Calls++
	resp, err := DecideChecked(context.Background(), d.Engine, req)
	d.Last = resp
	if err != nil {
		d.Rejected++
		return -1, err
	}
	if d.MinConfidence > 0 && resp.Confidence < d.MinConfidence {
		d.Rejected++
		return -1, fmt.Errorf("%w: %.3f < %.3f", ErrDecisionLowConfidence, resp.Confidence, d.MinConfidence)
	}
	action, err := ResolveBattleDecision(s, resp)
	if err != nil || action.Kind != game.BattleActionMove {
		d.Rejected++
		return -1, fmt.Errorf("%w: move-only turn resolved to %q", ErrInvalidDecision, resp.Choice)
	}
	return action.Slot, nil
}

// gen1MoveObserver reports each Gen I move turn to a portable
// BattleTurnObserver as the same move-only state BattleMoveDecider asks
// about, paired with the slot the deterministic policy chose. Turns the
// adapter cannot describe (Safari Zone, the Old Man demo) are not reported.
func gen1MoveObserver(romData []byte, observer BattleTurnObserver) skill.MoveObserver {
	if observer == nil {
		return nil
	}
	return func(b state.BattleState, executed int) {
		s, err := skill.MoveOnlyBattleDecisionState(romData, b)
		if err != nil {
			return
		}
		observer.ObserveBattleTurn(s, game.BattleAction{Kind: game.BattleActionMove, Slot: executed})
	}
}
