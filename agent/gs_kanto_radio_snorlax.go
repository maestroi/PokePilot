package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsKantoPokeFluteKnob byte = 78 // pret/pokegold: RadioChannels (channel 20.0)
	gsKantoRadioPage          = 12 // POKEGEARSTATE_RADIOJOYPAD
	gsKantoRadioClockPage     = 1  // POKEGEARSTATE_CLOCKJOYPAD
	gsKantoRadioStepBudget    = 120
	gsKantoRadioExitBudget    = 180
	gsKantoSnorlaxX uint8     = 34
	gsKantoSnorlaxY uint8     = 8
	gsKantoSnorlaxStandX uint8 = 34
	gsKantoSnorlaxStandY uint8 = 10
)

// executeGSKantoExpansionCard talks to the Lavender Radio Tower gentleman.
// He awards ENGINE_EXPN_CARD only after EVENT_RETURNED_MACHINE_PART, so power
// restoration is a positive prerequisite and flag bit 3 is the postcondition.
func executeGSKantoExpansionCard(m *emu.Emu, romData []byte) error {
	const label = "kanto:lavender-expansion-card"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoExpansionCard, gsprofile.ProgressKantoPowerRestored, label)
	if err != nil || done {
		return err
	}
	return gsKantoStoryTalk(m, romData, profile, gsprofile.ProgressKantoExpansionCard,
		"LAV_RADIO_TOWER_1F", 9, 2, 9, 1, label)
}

// gsKantoRadioDirection is a bounded decision based on the actual Gold/Silver
// radio control state. A returns nothing (and will never select a phone entry).
// Pokegear starts on CLOCK, and the owned card pages are advanced by RIGHT.
// On the RADIO page UP/DOWN moves the knob in two-unit increments to 78.
func gsKantoRadioDirection(s gsprofile.PokegearRadioState) (emu.Button, bool, error) {
	if !s.HasRadio || !s.HasExpansion {
		return 0, false, fmt.Errorf("%w: Snorlax requires Radio and Kanto EXPN cards",
			errGSSecondBadgeUnexpectedState)
	}
	switch s.Page {
	case 0, 2, 3, 5, 7, 11:
		return 0, false, nil // the ROM is drawing the next card
	case 1, 4, 6, 8:
		return emu.Right, true, nil
	case gsKantoRadioPage:
		switch {
		case s.Tuning < gsKantoPokeFluteKnob:
			return emu.Up, true, nil
		case s.Tuning > gsKantoPokeFluteKnob:
			return emu.Down, true, nil
		default:
			return 0, false, nil
		}
	default:
		return 0, false, fmt.Errorf("%w: unexpected Pokégear page %d while tuning Poké Flute",
			errGSSecondBadgeUnexpectedState, s.Page)
	}
}

// gsKantoTunePokeFlute opens the real Pokégear from START and tunes the Kanto
// Poké Flute station. It never writes the knob or wMapMusic; the ROM music
// transition is required for special SnorlaxAwake to succeed after leaving
// the menu. Only the later Snorlax event is proof the gate was actually cleared.
func gsKantoTunePokeFlute(m *emu.Emu, profile *gsprofile.Profile) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: Pokégear tuning requires an emulator and profile", errGSSecondBadgeUnexpectedState)
	}
	s := profile.DecodePokegearRadio(m)
	if !s.HasRadio || !s.HasExpansion {
		return fmt.Errorf("%w: cannot tune Kanto Poké Flute without Radio and EXPN cards", errGSSecondBadgeUnexpectedState)
	}
	if err := skill.OpenStartMenuEntry(m, game.StartMenuPokegear); err != nil {
		return fmt.Errorf("gen2 Poké Flute: open Pokégear: %w", err)
	}
	m.StepFrames(20)
	seenClock := false
	tuned := false
	for i := 0; i < gsKantoRadioStepBudget; i++ {
		s = profile.DecodePokegearRadio(m)
		if s.Page == gsKantoRadioClockPage {
			seenClock = true
		}
		if !seenClock {
			// Do not steer an unrelated jumptable when START selected the
			// wrong entry or the fade has not yielded to the CLOCK page.
			if i > 12 {
				return fmt.Errorf("%w: Pokégear did not render its CLOCK page before tuning; page=%d",
					errGSSecondBadgeStalled, s.Page)
			}
			m.StepFrames(15)
			continue
		}
		if s.PokeFluteTuned() {
			tuned = true
			break
		}
		btn, press, err := gsKantoRadioDirection(s)
		if err != nil {
			return err
		}
		if press {
			m.Tap(btn, 3, 9)
		} else {
			m.StepFrames(12)
		}
	}
	if !tuned {
		return fmt.Errorf("%w: radio never reached station 20.0 in %d steps (page=%d tuning=%d)",
			errGSSecondBadgeStalled, gsKantoRadioStepBudget, s.Page, s.Tuning)
	}
	// RadioMusicRestartDE changes wMapMusic to MUSIC_POKE_FLUTE_CHANNEL
	// during station playback. Give the actual station a few frames to
	// start before the B press exits its UI.
	m.StepFrames(35)
	m.Tap(emu.B, 3, 9)
	// PokeGear returns to the START menu. Seeing that menu, then closing
	// it and regaining overworld control, is stronger evidence than a
	// stale wJumptableIndex after closing a sub-menu.
	seenStartMenu := false
	for i := 0; i < gsKantoRadioExitBudget; i++ {
		menu := profile.DecodeStartMenu(m)
		if menu.Visible {
			seenStartMenu = true
			m.Tap(emu.B, 3, 9)
			continue
		}
		if seenStartMenu && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		m.StepFrames(10)
	}
	return fmt.Errorf("%w: Pokégear tuned 20.0 but never returned through the START menu",
		errGSSecondBadgeStalled)
}

// executeGSKantoSnorlax first reaches the exact retail proximity tile, then
// tunes station 20.0 while standing next to the two-tile Snorlax sprite.
// SnorlaxAwake checks that position AND wMapMusic; ordinary A on Snorlax with
// another station only prints its sleeping text. The scripted battle finally
// sets EVENT_FOUGHT_SNORLAX, the sole positive objective postcondition.
func executeGSKantoSnorlax(m *emu.Emu, romData []byte) error {
	const label = "kanto:vermilion-snorlax"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoSnorlaxCleared, gsprofile.ProgressKantoExpansionCard, label)
	if err != nil || done {
		return err
	}
	vermilion, err := gsOpeningMapID("VERMILION_CITY")
	if err != nil {
		return err
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Snorlax requires a controllable Kanto overworld",
			errGSSecondBadgeUnexpectedState)
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(vermilion, gsKantoSnorlaxStandX, gsKantoSnorlaxStandY),
		gsKantoSnorlaxX, gsKantoSnorlaxY+1, label); err != nil {
		return fmt.Errorf("gen2 Snorlax: reach south-facing proximity tile: %w", err)
	}
	world := profile.DecodeOverworld(m)
	if world.NativeMapID != vermilion || world.X != gsKantoSnorlaxStandX ||
		world.Y != gsKantoSnorlaxStandY || !world.Controllable {
		return fmt.Errorf("%w: Snorlax proximity not established at (%d,%d): map=%#04x pos=(%d,%d)",
			errGSSecondBadgeUnexpectedState, gsKantoSnorlaxStandX, gsKantoSnorlaxStandY,
			world.NativeMapID, world.X, world.Y)
	}
	if err := gsKantoTunePokeFlute(m, profile); err != nil {
		return fmt.Errorf("gen2 Snorlax: tune radio: %w", err)
	}
	m.StepFrames(gsLakeScriptSettleFrames)
	m.Tap(emu.A, 3, 9)
	if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
		return fmt.Errorf("gen2 Snorlax: scripted encounter: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressKantoSnorlaxCleared) {
		return fmt.Errorf("%w: Poké Flute interaction ended without EVENT_FOUGHT_SNORLAX",
			errGSSecondBadgeUnexpectedState)
	}
	return nil
}
