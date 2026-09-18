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
func bootInput(state game.BootState, iteration int) emu.Button {
	if iteration < 4 {
		return emu.Start
	}
	if !state.NameMenu {
		return emu.A
	}
	switch {
	case state.CurrentMenuItem < introPresetNameIndex:
		return emu.Down
	case state.CurrentMenuItem > introPresetNameIndex:
		return emu.Up
	default:
		return emu.A
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
// The input policy is shared Gen-I behavior: clear title/menu with Start,
// page ordinary intro dialogue with A, and steer Oak's preset-name menus to
// the second built-in preset. Profiles own all RAM decoding, the ready map and
// controllability test.
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

		// The cursor rests on the selected preset for several frames while A is
		// pressed. Capture the live menu text rather than hard-coding names.
		if last.NameMenu && last.CurrentMenuItem == introPresetNameIndex &&
			len(last.PresetNames) >= introPresetNameIndex {
			name := last.PresetNames[introPresetNameIndex-1]
			if n := len(expectedNames); n == 0 || expectedNames[n-1] != name {
				expectedNames = append(expectedNames, name)
			}
		}
		m.Tap(bootInput(last, i), 3, 7)
	}

	return game.ProfileObservation{}, fmt.Errorf(
		"boot: no controllable overworld for %s@%s within %d iterations; last: map=%#04x %s x=%d y=%d mapW=%d mapH=%d fontLoaded=%#04x menu=(cur=%d max=%d name=%v) controllable=%v",
		profile.ID(), profile.Revision(), budget,
		last.NativeMapID, last.MapName, last.X, last.Y,
		last.MapWidth, last.MapHeight, last.FontLoaded,
		last.CurrentMenuItem, last.MaxMenuItem, last.NameMenu, last.Controllable)
}
