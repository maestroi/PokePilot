package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

// counterDirection returns the step toward the center's counter. Local map
// geometry is still a Gen-I navigation concern in this slice; nurse dialogue,
// party recovery and transaction completion are profile-owned.
func counterDirection(m *emu.Emu, decoder game.OverworldDecoder) (world.Step, error) {
	romData := m.ROM()
	live, err := healRuntimeStateWithDecoder(m, decoder)
	if err != nil {
		return world.Step{}, err
	}
	cur := live.Map
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return world.Step{}, fmt.Errorf("skill: Heal: parse map %#04x: %w", cur, err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return world.Step{}, fmt.Errorf("skill: Heal: build map %#04x: %w", cur, err)
	}
	x, y := live.X, live.Y
	var solid []world.Step
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		nx, ny := int(x)+s.DX, int(y)+s.DY
		if !grid.InBounds(nx, ny) {
			continue
		}
		if !grid.Walkable(nx, ny) {
			solid = append(solid, s)
		}
	}
	if len(solid) != 1 {
		return world.Step{}, fmt.Errorf("skill: Heal: map %#04x at (%d,%d): expected exactly one non-walkable neighbor (the counter), found %d",
			cur, x, y, len(solid))
	}
	return solid[0], nil
}

// Heal restores the party at a Pokemon Center nurse. Navigation to the Gen-I
// service actor remains in this file for now; once interaction starts, every
// UI/recovery/finish decision comes from the active game's semantic Center
// profile rather than Red RAM.
func Heal(m *emu.Emu) error {
	runtime, err := pokemonCenterRuntimeFor(m)
	if err != nil {
		return err
	}
	live, err := healRuntimeStateWithDecoder(m, runtime)
	if err != nil {
		return err
	}
	if !live.Controllable {
		return fmt.Errorf("skill: Heal: player not controllable: map=%#04x at (%d,%d)", live.Map, live.X, live.Y)
	}
	if !runtime.DecodeCenter(m).PartyPresent {
		return fmt.Errorf("skill: Heal: no party to heal: map=%#04x at (%d,%d)", live.Map, live.X, live.Y)
	}

	nurse, ok, err := interactionDestinationForRole(m.ROM(), live.Map, rom.InteractionPokemonCenterNurse)
	if err != nil {
		return fmt.Errorf("skill: Heal: locate nurse: %w", err)
	}
	if !ok {
		return fmt.Errorf("skill: Heal: no Pokemon Center nurse on map %#04x", live.Map)
	}
	if err := GoTo(m, m.ROM(), nurse); err != nil {
		return fmt.Errorf("skill: Heal: approach nurse: %w", err)
	}

	step, err := counterDirection(m, runtime)
	if err != nil {
		return err
	}
	live, err = healRuntimeStateWithDecoder(m, runtime)
	if err != nil {
		return err
	}
	x, y := live.X, live.Y
	if err := Face(m, uint8(int(x)+step.DX), uint8(int(y)+step.DY)); err != nil {
		return fmt.Errorf("skill: Heal: face the counter %s from (%d,%d): %w", step, x, y, err)
	}

	if err := healAtNurse(m, runtime); err != nil {
		return err
	}

	live, err = healRuntimeStateWithDecoder(m, runtime)
	if err != nil {
		return err
	}
	center := runtime.DecodeCenter(m)
	if !center.Recovered {
		return fmt.Errorf("skill: Heal: party not fully recovered after the heal: map=%#04x at (%d,%d)", live.Map, live.X, live.Y)
	}
	if !live.Controllable {
		return fmt.Errorf("skill: Heal: not controllable after the heal: map=%#04x at (%d,%d)", live.Map, live.X, live.Y)
	}
	return nil
}
