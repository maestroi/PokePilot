package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

const (
	nativeFlyPickerBudget  = 1000
	nativeFlyChoicesBudget = 30
	nativeFlyLandingBudget = 3600
)

// UseNativeFly selects a game-adapter-owned Fly destination through the real
// party submenu and the cartridge's visited-flypoint picker. A Fly move
// without a destination is still rejected by UseFieldMove. No generation
// specific spawn ids or town names are kept in this executor.
func UseNativeFly(m *emu.Emu, nativeDestination uint16) error {
	if m == nil {
		return fmt.Errorf("skill: Fly: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return fmt.Errorf("skill: Fly: detect game: %w", err)
	}
	fly, ok := profile.(game.FlyDestinationDecoder)
	if !ok {
		return fmt.Errorf("skill: Fly: game profile does not implement destination selection")
	}
	overworld, ok := profile.(game.OverworldDecoder)
	if !ok {
		return fmt.Errorf("skill: Fly: game profile does not decode destination arrival")
	}
	desiredLabel, ok := fly.FlyDestinationLabel(nativeDestination)
	if !ok {
		return fmt.Errorf("%w: native map %#04x is not a Fly landing", ErrFieldMovePrerequisite, nativeDestination)
	}
	start := overworld.DecodeOverworld(m)
	if !start.Controllable || start.InBattle || start.InDialogue {
		return fmt.Errorf("skill: Fly: player must be controllable before opening the map")
	}
	if start.NativeMapID == nativeDestination {
		return nil
	}
	slot, err := EnsureFieldMove(m, FieldFly)
	if err != nil {
		return err
	}
	menu, err := menuDecoderFor(m)
	if err != nil {
		return err
	}
	party, err := partyMenuDecoderFor(m)
	if err != nil {
		return err
	}
	if err := openStartMenuEntryWithDecoder(m, menu, startMenuPokemon); err != nil {
		return fmt.Errorf("skill: Fly: open POKEMON: %w", err)
	}
	if !waitMenuUntil(m, 1000, func() bool {
		s := party.DecodePartyMenu(m)
		return s.Visible && s.Kind == game.PartyMenuFieldMove
	}) {
		return fmt.Errorf("skill: Fly: party menu did not appear")
	}
	if err := selectPartySlotWithDecoder(m, party, slot); err != nil {
		return fmt.Errorf("skill: Fly: select party slot %d: %w", slot, err)
	}
	if err := selectFieldMoveMenuEntry(m, FieldFly); err != nil {
		return fmt.Errorf("skill: Fly: choose Fly: %w", err)
	}
	if !waitMenuUntil(m, nativeFlyPickerBudget, func() bool {
		return fly.DecodeFlyMenu(m).Visible
	}) {
		return fmt.Errorf("skill: Fly: destination picker did not open")
	}

	// The cartridge itself skips unvisited destinations when scrolling,
	// and selects Johto or Kanto depending on the source region. Never use
	// a fixed number of UP presses or send A for a label we did not observe.
	choice := false
	for i := 0; i < nativeFlyChoicesBudget; i++ {
		state := fly.DecodeFlyMenu(m)
		if !state.Visible {
			return fmt.Errorf("skill: Fly: destination picker closed before selection")
		}
		if strings.HasPrefix(state.SelectedLabel, strings.ToUpper(desiredLabel)) {
			choice = true
			break
		}
		m.Tap(emu.Up, 3, 7)
		if !waitMenuUntil(m, 80, func() bool {
			s := fly.DecodeFlyMenu(m)
			return s.Visible && s.SelectedLabel != state.SelectedLabel
		}) {
			// A missed input is not evidence of reaching an unvisited town.
			m.StepFrames(8)
		}
	}
	if !choice {
		closeNativeFlyPicker(m, fly, overworld)
		return fmt.Errorf("%w: Fly landing %q is unvisited or outside the active region", ErrFieldMovePrerequisite, desiredLabel)
	}

	m.Tap(emu.A, 3, 7)
	for spent := 0; spent < nativeFlyLandingBudget; spent += 10 {
		state := overworld.DecodeOverworld(m)
		if state.NativeMapID == nativeDestination && state.Controllable {
			return nil
		}
		if state.NativeMapID != start.NativeMapID && state.NativeMapID != nativeDestination &&
			state.Controllable {
			return fmt.Errorf("skill: Fly: landed on %#04x instead of %#04x: %w", state.NativeMapID, nativeDestination, ErrNavigationStalled)
		}
		m.StepFrames(10)
	}
	return fmt.Errorf("skill: Fly: destination %#04x never became controllable: %w", nativeDestination, ErrNavigationStalled)
}

// Only the Fly picker opened by this executor is dismissible here. Never
// answer unrelated post-Fly gameplay dialogue or battle choices.
func closeNativeFlyPicker(m *emu.Emu, fly game.FlyDestinationDecoder, overworld game.OverworldDecoder) {
	for i := 0; i < 50; i++ {
		if overworld.DecodeOverworld(m).Controllable {
			return
		}
		if !fly.DecodeFlyMenu(m).Visible && i > 10 {
			return
		}
		m.Tap(emu.B, 3, 7)
		m.StepFrames(10)
	}
}
