package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

// introPresetNameIndex selects the second built-in preset after NEW NAME.
// In Red/Blue this yields the anime ASH/GARY pair; Yellow's profile exposes
// its own live preset text through the same semantic boundary.
const introPresetNameIndex = 2

// bootInput chooses input from semantic boot state. It contains no RAM
// addresses, map ids or game-name branches.
func bootInput(state game.BootState, iteration int) (emu.Button, bool) {
	switch state.NextInput {
	case game.BootInputConfirm:
		return emu.A, true
	case game.BootInputStart:
		return emu.Start, true
	case game.BootInputUp:
		return emu.Up, true
	case game.BootInputDown:
		return emu.Down, true
	case game.BootInputWait:
		return 0, false
	}

	// Legacy profiles keep the established Gen-I boot policy.
	if iteration < 4 {
		return emu.Start, true
	}
	if !state.NameMenu {
		return emu.A, true
	}
	switch {
	case state.CurrentMenuItem < introPresetNameIndex:
		return emu.Down, true
	case state.CurrentMenuItem > introPresetNameIndex:
		return emu.Up, true
	default:
		return emu.A, true
	}
}

func verifyBootedOverworld(state game.BootState, expectedNames []string) error {
	actual := []string{state.PlayerName, state.RivalName}
	for i, want := range expectedNames {
		if i >= len(actual) {
			break
		}
		if actual[i] != want {
			return fmt.Errorf("boot: reached overworld with name %q, want preset %q", actual[i], want)
		}
	}
	return nil
}

// BootToOverworld drives any profile implementing game.BootProfile from a
// fresh cartridge to its profile-owned normal-overworld starting state.
//
// Legacy profiles retain the shared Gen-I policy. Profiles with distinct boot
// flows may recommend semantic confirm/start/direction/wait inputs while still
// keeping RAM addresses, menu detection and the ready-state decision private.
func BootToOverworld(m *emu.Emu) (game.ProfileObservation, error) {
	if m == nil {
		return game.ProfileObservation{}, fmt.Errorf("boot: nil emulator")
	}
	romData := m.ROM()
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return game.ProfileObservation{}, fmt.Errorf("boot: detect game profile: %w", err)
	}
	bootProfile, ok := profile.(game.BootProfile)
	if !ok {
		return game.ProfileObservation{}, fmt.Errorf(
			"boot: profile %s@%s does not implement fresh-game boot semantics",
			profile.ID(), profile.Revision())
	}

	m.StepFrames(300)
	var expectedNames []string
	var last game.BootState

	const budget = 900
	for i := 0; i < budget; i++ {
		last = bootProfile.DecodeBootState(m)
		if last.Ready {
			if err := verifyBootedOverworld(last, expectedNames); err != nil {
				return game.ProfileObservation{}, err
			}
			obs, err := profile.DecodeObservation(m, romData)
			if err != nil {
				return game.ProfileObservation{}, fmt.Errorf("boot: decode final observation: %w", err)
			}
			if !obs.Controllable {
				return game.ProfileObservation{}, fmt.Errorf(
					"boot: profile %s reported ready but final observation is not controllable",
					profile.ID())
			}
			return obs, nil
		}

		// Profiles may provide the exact preset implied by their semantic menu
		// state. Legacy Gen-I profiles still expose live preset text.
		name := last.SelectedPresetName
		if name == "" && last.NameMenu && last.CurrentMenuItem == introPresetNameIndex &&
			len(last.PresetNames) >= introPresetNameIndex {
			name = last.PresetNames[introPresetNameIndex-1]
		}
		if name != "" {
			if n := len(expectedNames); n == 0 || expectedNames[n-1] != name {
				expectedNames = append(expectedNames, name)
			}
		}

		btn, press := bootInput(last, i)
		if !press {
			m.StepFrames(10)
			continue
		}
		m.Tap(btn, 3, 7)
	}

	return game.ProfileObservation{}, fmt.Errorf(
		"boot: no controllable overworld for %s@%s within %d iterations; last: map=%#04x %s x=%d y=%d mapW=%d mapH=%d fontLoaded=%#04x menu=(cur=%d max=%d name=%v) controllable=%v",
		profile.ID(), profile.Revision(), budget,
		last.NativeMapID, last.MapName, last.X, last.Y,
		last.MapWidth, last.MapHeight, last.FontLoaded,
		last.CurrentMenuItem, last.MaxMenuItem, last.NameMenu, last.Controllable)
}
