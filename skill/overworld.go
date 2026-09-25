package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/world"
)

// overworldMovementMachine is the execution surface needed by the portable
// one-step controller. *emu.Emu satisfies it; tests can use a deterministic
// fake without importing a concrete game's RAM layout.
type overworldMovementMachine interface {
	game.MemoryReader
	StepFrame()
	Press(emu.Button)
	Release(emu.Button)
}

func overworldDecoderFor(m *emu.Emu) (game.OverworldDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: overworld: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: overworld: detect profile: %w", err)
	}
	decoder, ok := profile.(game.OverworldDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: overworld: profile %s@%s does not expose overworld semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func overworldPosition(reader game.MemoryReader, decoder game.OverworldDecoder) (uint8, uint8) {
	state := decoder.DecodeOverworld(reader)
	return state.X, state.Y
}

func overworldIdle(reader game.MemoryReader, decoder game.OverworldDecoder) bool {
	return decoder.DecodeOverworld(reader).MovementIdle
}

// movementInterruptionWithDecoder classifies only the two ownership changes
// WalkPath cares about. It never advances gameplay.
func movementInterruptionWithDecoder(reader game.MemoryReader, decoder game.OverworldDecoder) error {
	state := decoder.DecodeOverworld(reader)
	if state.InBattle {
		return ErrBattleInterrupted
	}
	if state.InDialogue {
		return ErrDialogueInterrupted
	}
	return nil
}

// stepOnceWithOverworldDecoder is the reusable local movement primitive.
// It knows directions and semantic overworld state, but no concrete RAM
// addresses, map ids, story flags, or ROM tables.
func stepOnceWithOverworldDecoder(m overworldMovementMachine, s world.Step, decoder game.OverworldDecoder) error {
	if decoder == nil {
		return fmt.Errorf("skill: StepOnce: nil overworld decoder")
	}
	btn, ok := buttonFor(s)
	if !ok {
		return fmt.Errorf("skill: invalid step %s", s)
	}

	start := decoder.DecodeOverworld(m)
	startX, startY := start.X, start.Y
	moved := false
	for attempt := 0; attempt < 2 && !moved; attempt++ {
		m.Press(btn)
		for stepped := 0; stepped < stepMoveBudget; stepped++ {
			state := decoder.DecodeOverworld(m)
			if state.X != startX || state.Y != startY {
				moved = true
				break
			}
			m.StepFrame()
		}
		m.Release(btn)
	}
	if !moved {
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{startX, startY}}
	}

	settleBudget := stepSettleBudget
	if absInt(s.DX)+absInt(s.DY) == 2 {
		settleBudget = hopSettleBudget
	}
	for stepped := 0; stepped < settleBudget; stepped++ {
		if overworldIdle(m, decoder) {
			break
		}
		m.StepFrame()
	}

	final := decoder.DecodeOverworld(m)
	if int(final.X) != int(startX)+s.DX || int(final.Y) != int(startY)+s.DY {
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{startX, startY}}
	}
	return nil
}
