package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

const settleStableFrames = 20

// ErrCampaignComplete reports that a battle's aftermath was the game's ending:
// the main story is now complete and control will not return to the caller's
// walk. The run goal check, not the interrupted objective, owns what follows.
var ErrCampaignComplete = errors.New("skill: Battle: campaign complete; the ending never returns control")

// errBattleRestarted reports that the aftermath of a battle was another battle:
// a script that queues back-to-back trainers (Mt. Moon B2F Jessie then James,
// run-acduyt1qbev9c) clears wIsInBattle between them. Settling cannot play a
// battle, so the caller that owns the battle loop must resume it.
var errBattleRestarted = errors.New("skill: Battle: a new battle began while settling")

func settleAfterBattle(m *emu.Emu, decoder game.BattleRuntimeDecoder) error {
	if decoder == nil {
		var err error
		decoder, err = battleRuntimeDecoderFor(m)
		if err != nil {
			return fmt.Errorf("skill: Battle: settle: %w", err)
		}
	}

	startFrame := m.FrameCount()
	stable := 0
	for int(m.FrameCount()-startFrame) < settleBudget {
		live := decoder.DecodeBattleRuntime(m)
		if live.InBattle {
			return errBattleRestarted
		}
		if live.Controllable {
			stable++
			if stable >= settleStableFrames {
				return nil
			}
		} else {
			stable = 0
		}
		if live.TextActive {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}

	live := decoder.DecodeBattleRuntime(m)
	// Red/Blue currently hand the Champion aftermath to a game-specific Hall
	// of Fame executor. The portable runtime owns detection; the story executor
	// remains deliberately separate from battle-state decoding.
	if live.CampaignComplete {
		if err := finishHallOfFame(m); err != nil {
			return err
		}
		return ErrCampaignComplete
	}
	return fmt.Errorf("skill: Battle: not controllable %d frames after the battle ended: %s",
		settleBudget, battleRuntimeContext(live))
}

func stuckError(
	m *emu.Emu,
	battleDecoder game.BattleStateDecoder,
	runtimeDecoder game.BattleRuntimeDecoder,
	detail string,
) (game.BattleResult, error) {
	live := runtimeDecoder.DecodeBattleRuntime(m)
	bs := "<none>"
	if b, ok := battleDecoder.DecodeBattleState(m); ok {
		bs = fmt.Sprintf("%+v", b)
	}
	return 0, fmt.Errorf("skill: Battle: %s: %s battle %s",
		detail, battleRuntimeContext(live), bs)
}

func menuError(m *emu.Emu, detail string, err error) (game.BattleResult, error) {
	battleDecoder, battleErr := battleStateDecoderFor(m)
	runtimeDecoder, runtimeErr := battleRuntimeDecoderFor(m)
	if battleErr != nil || runtimeErr != nil {
		return 0, fmt.Errorf("skill: Battle: %s: %w", detail, err)
	}
	live := runtimeDecoder.DecodeBattleRuntime(m)
	bs := "<none>"
	if b, ok := battleDecoder.DecodeBattleState(m); ok {
		bs = fmt.Sprintf("%+v", b)
	}
	return 0, fmt.Errorf("skill: Battle: %s: %s battle %s: %w",
		detail, battleRuntimeContext(live), bs, err)
}
