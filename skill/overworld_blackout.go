package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func overworldBlackoutDecoderFor(m *emu.Emu) (game.OverworldBlackoutDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: overworld blackout: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: overworld blackout: detect profile: %w", err)
	}
	decoder, ok := profile.(game.OverworldBlackoutDecoder)
	if !ok {
		return nil, fmt.Errorf(
			"skill: overworld blackout: profile %s@%s does not expose blackout semantics",
			profile.ID(), profile.Revision(),
		)
	}
	return decoder, nil
}

func blackoutInProgress(reader game.MemoryReader, decoder game.OverworldBlackoutDecoder) bool {
	if reader == nil || decoder == nil {
		return false
	}
	return decoder.DecodeOverworldBlackout(reader).BlackoutInProgress
}

func partyAllFaintedWithDecoder(reader game.MemoryReader, decoder game.OverworldBlackoutDecoder) bool {
	if reader == nil || decoder == nil {
		return false
	}
	return decoder.DecodeOverworldBlackout(reader).PartyAllFainted
}

func respawnedFromFaintWithDecoders(
	reader game.MemoryReader,
	startMap uint16,
	overworld game.OverworldDecoder,
	blackout game.OverworldBlackoutDecoder,
) bool {
	if reader == nil || overworld == nil || blackout == nil {
		return false
	}
	world := overworld.DecodeOverworld(reader)
	blackoutState := blackout.DecodeOverworldBlackout(reader)
	if world.NativeMapID == startMap || world.NativeMapID != blackoutState.RespawnNativeMapID {
		return false
	}
	return world.Controllable && !blackoutState.PartyAllFainted
}

type faintRespawnMachine interface {
	game.MemoryReader
	StepFrame()
}

// waitForFaintRespawnWithDecoders preserves Gen-I's poison-wipe recovery
// behavior without exposing any Gen-I symbols to the movement driver. Other
// profiles can project equivalent semantics or simply report PartyAllFainted
// false when their overworld does not have this transition shape.
func waitForFaintRespawnWithDecoders(
	m faintRespawnMachine,
	startMap uint16,
	startFainted bool,
	overworld game.OverworldDecoder,
	blackout game.OverworldBlackoutDecoder,
) error {
	if !startFainted {
		return nil
	}
	if respawnedFromFaintWithDecoders(m, startMap, overworld, blackout) {
		return ErrBlackedOut
	}

	world := overworld.DecodeOverworld(m)
	blackoutState := blackout.DecodeOverworldBlackout(m)
	if world.NativeMapID != startMap && world.NativeMapID != blackoutState.RespawnNativeMapID {
		return nil
	}
	if world.NativeMapID == startMap && world.Controllable && !world.InBattle && !world.InDialogue {
		return nil
	}

	for i := 0; i < arriveBudget; i++ {
		if respawnedFromFaintWithDecoders(m, startMap, overworld, blackout) {
			return ErrBlackedOut
		}
		world = overworld.DecodeOverworld(m)
		blackoutState = blackout.DecodeOverworldBlackout(m)
		if world.NativeMapID != startMap &&
			world.NativeMapID != blackoutState.RespawnNativeMapID &&
			world.Controllable {
			return nil
		}
		m.StepFrame()
	}
	if respawnedFromFaintWithDecoders(m, startMap, overworld, blackout) {
		return ErrBlackedOut
	}
	return nil
}
