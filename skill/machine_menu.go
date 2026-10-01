package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

const machineMenuSettleBudget = 180

func machineMenuDecoderFor(m *emu.Emu) (game.MachineMenuDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: machine menu: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: machine menu: detect profile: %w", err)
	}
	decoder, ok := profile.(game.MachineMenuDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: machine menu: profile %s@%s does not expose machine-menu semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

// openMachineEntry opens the active game's carried-items menu and selects one
// owned native TM/HM entry. It deliberately stops at the machine's action
// submenu; answering USE/teach prompts belongs to the teaching transaction.
func openMachineEntry(m *emu.Emu, native game.NativeFieldMove) error {
	menu, err := menuDecoderFor(m)
	if err != nil {
		return err
	}
	machines, err := machineMenuDecoderFor(m)
	if err != nil {
		return err
	}
	if err := openStartMenuEntryWithDecoder(m, menu, startMenuItems); err != nil {
		return fmt.Errorf("skill: machine menu: open items: %w", err)
	}
	if !waitMenuUntil(m, machineMenuSettleBudget, func() bool {
		return machines.DecodeMachineMenu(m).Visible
	}) {
		return fmt.Errorf("skill: machine menu: carried-items menu did not appear")
	}
	return selectMachineEntryWithDecoder(m, machines, native)
}

func selectMachineEntryWithDecoder(m menuMachine, decoder game.MachineMenuDecoder, native game.NativeFieldMove) error {
	if decoder == nil {
		return fmt.Errorf("skill: machine menu: nil decoder")
	}
	state := decoder.DecodeMachineMenu(m)
	if !state.Visible {
		return fmt.Errorf("skill: machine menu is not visible")
	}
	target, ok := decoder.MachineMenuEntryIndex(m, native)
	if !ok {
		return fmt.Errorf("skill: native machine %#04x is not an owned menu entry", native.MachineItemID)
	}

	// Gold/Silver-style multi-pocket inventories cycle left. Profiles that
	// expose this contract must report semantic pocket changes; the controller
	// never knows native pocket numbers.
	for attempts := 0; state.Pocket != game.MachinePocketTMHM && attempts < 4; attempts++ {
		previous := state.Pocket
		m.Tap(emu.Left, 3, 7)
		if !waitMenuUntil(m, machineMenuSettleBudget, func() bool {
			next := decoder.DecodeMachineMenu(m)
			return next.Visible && next.Pocket != previous
		}) {
			return fmt.Errorf("skill: machine menu pocket did not move left from %q", previous)
		}
		state = decoder.DecodeMachineMenu(m)
	}
	if state.Pocket != game.MachinePocketTMHM {
		return fmt.Errorf("skill: machine menu could not reach TM/HM pocket from %q", state.Pocket)
	}
	if !state.Ready {
		if !waitMenuUntil(m, machineMenuSettleBudget, func() bool {
			next := decoder.DecodeMachineMenu(m)
			return next.Visible && next.Pocket == game.MachinePocketTMHM && next.Ready
		}) {
			return fmt.Errorf("skill: TM/HM pocket did not become ready")
		}
		state = decoder.DecodeMachineMenu(m)
	}
	if target < 0 || target >= state.Count {
		return fmt.Errorf("skill: machine menu target %d outside owned count %d", target, state.Count)
	}

	const stuckLimit = 6
	stuck := 0
	for state.Position != target {
		previous := state.Position
		btn := emu.Down
		if previous > target {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if !waitMenuUntil(m, machineMenuSettleBudget, func() bool {
			next := decoder.DecodeMachineMenu(m)
			return next.Ready && next.Position != previous
		}) {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("skill: machine menu cursor stuck at %d, wanted %d: %w", previous, target, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
		state = decoder.DecodeMachineMenu(m)
		if !state.Visible || state.Pocket != game.MachinePocketTMHM {
			return fmt.Errorf("skill: machine menu left TM/HM pocket while selecting entry %d", target)
		}
	}

	m.Tap(emu.A, 3, 7)
	if !waitMenuUntil(m, machineMenuSettleBudget, func() bool {
		return !decoder.DecodeMachineMenu(m).Ready
	}) {
		return fmt.Errorf("skill: machine entry %d did not open its action submenu", target)
	}
	return nil
}
