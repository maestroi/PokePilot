package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsAquaBoardFrameBudget    uint64 = 30_000
	gsAquaBoardMaxPresses            = 90
	gsAquaInteractionAttempts        = 3
)

var gsAquaShipMaps = []string{
	"FAST_SHIP_1F", "FAST_SHIP_B1F",
	"FAST_SHIP_CABINS_NNW_NNE_NE",
	"FAST_SHIP_CABINS_SW_SSW_NW",
	"FAST_SHIP_CABINS_SE_SSE_CAPTAINS_CABIN",
}

// executeGSSSTicket collects Elm's post-League key item. The retail event
// alone is not sufficient: boarding scripts check the key-item pocket.
func executeGSSSTicket(m *emu.Emu, romData []byte) error {
	const label = "postleague:elm-ss-ticket"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressSSTicketReceived, gsprofile.ProgressLeagueHallOfFameEntered, label)
	if err != nil || done {
		return err
	}
	if !profile.DecodeOverworld(m).Controllable {
		// Hall of Fame is an in-flight credits/reset scene; it must be
		// completed by a post-credits CONTINUE transaction before navigating.
		return fmt.Errorf("%w: S.S. Ticket requires a controllable post-credits save", errGSSecondBadgeUnexpectedState)
	}
	lab, err := gsOpeningMapID("ELMS_LAB")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsAquaInteractionAttempts; attempt++ {
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(lab, 5, 3), 5, 2, label); err != nil {
			return fmt.Errorf("gen2 S.S. Ticket: reach Professor Elm: %w", err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 S.S. Ticket: settle Elm gift: %w", err)
		}
		if profile.DecodeFirstShipVoyage(m).HasTicket && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Elm dialogue ended without S.S. Ticket in key-item pocket", errGSSecondBadgeUnexpectedState)
}

// gsAquaMissingPrerequisites keeps ferry schedule and ticket policy in the
// cartridge's region-transport contract, not in map or dialogue guesses.
func gsAquaMissingPrerequisites(options []game.RegionTransportOption) error {
	for _, option := range options {
		if option.Service != "ss_aqua" || option.FromRegion != game.RegionJohto ||
			option.ToRegion != game.RegionKanto {
			continue
		}
		if option.Available {
			return nil
		}
		missing := make([]game.Prerequisite, 0, len(option.Missing))
		for _, capability := range option.Missing {
			missing = append(missing, game.CapabilityPrerequisite(capability))
		}
		return &game.PrerequisiteMissingError{Missing: missing}
	}
	return fmt.Errorf("%w: Olivine to Vermilion transport is not published by this cartridge", errGSControllerUnavailable)
}

// gsAquaBoardFirstVoyage owns the actual Olivine coord event and YES choice.
// A warp edge in the static native graph is not permission to bypass the
// weekday/ticket/choice script. The scripted transition to FAST_SHIP_1F
// is the only positive postcondition of boarding, never Kanto arrival.
func gsAquaBoardFirstVoyage(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	port, err := gsOpeningMapID("OLIVINE_PORT")
	if err != nil {
		return err
	}
	ship, err := gsOpeningMapID("FAST_SHIP_1F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(port, 7, 14)); err != nil {
		return fmt.Errorf("gen2 S.S. Aqua: reach Olivine gangway trigger: %w", err)
	}
	if profile.DecodeOverworld(m).NativeMapID != port {
		return fmt.Errorf("%w: ferry approach left Olivine Port", errGSSecondBadgeUnexpectedState)
	}
	// The retail boarding coord_event is (7,15); GoToNative may report the
	// script as a dialogue interruption when that step triggers. Do not let
	// the ordinary route interruption handler answer the YES/NO choice.
	err = skill.GoToNative(m, romData, skill.ExactNativeDestination(port, 7, 15))
	if err != nil && !errors.Is(err, skill.ErrDialogueInterrupted) {
		return fmt.Errorf("gen2 S.S. Aqua: trigger boarding scene: %w", err)
	}
	start := m.FrameCount()
	presses := 0
	accepted := false
	for m.FrameCount()-start < gsAquaBoardFrameBudget {
		world := profile.DecodeOverworld(m)
		if world.NativeMapID == ship {
			if !accepted {
				return fmt.Errorf("%w: ship entered without a verified boarding choice", errGSSecondBadgeUnexpectedState)
			}
			return driveGSSecondBadgeInterruption(m, profile, "ss-aqua:embark")
		}
		if world.NativeMapID != port {
			return fmt.Errorf("%w: boarding scene moved to unexpected map %#04x", errGSSecondBadgeUnexpectedState, world.NativeMapID)
		}
		if choice, visible := profile.DecodeTwoOption(m); visible {
			if accepted {
				return fmt.Errorf("%w: repeated ferry boarding prompt", errGSSecondBadgeUnexpectedState)
			}
			if choice.Current != 0 {
				m.Tap(emu.Up, 3, 7) // YES is the first entry.
			} else {
				m.Tap(emu.A, 3, 7)
				accepted = true
			}
			continue
		}
		if world.InDialogue && world.MovementIdle {
			if presses >= gsAquaBoardMaxPresses {
				return fmt.Errorf("%w: ferry boarding dialogue exceeded %d presses", errGSSecondBadgeStalled, presses)
			}
			m.Tap(emu.A, 3, 7)
			presses++
			continue
		}
		if world.Controllable {
			return fmt.Errorf("%w: Olivine boarding script returned without entering S.S. Aqua", errGSSecondBadgeUnexpectedState)
		}
		m.StepFrame()
	}
	return fmt.Errorf("%w: Olivine boarding scene exceeded %d frames", errGSSecondBadgeStalled, gsAquaBoardFrameBudget)
}

// gsAquaTalk executes one known, retail ship interaction and asserts that it
// changed the expected event, rather than treating dialogue as success.
func gsAquaTalk(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	mapName string, sx, sy, tx, ty uint8, label string, complete func(gsprofile.FirstShipVoyageState) bool,
) error {
	mapID, err := gsOpeningMapID(mapName)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsAquaInteractionAttempts; attempt++ {
		if complete(profile.DecodeFirstShipVoyage(m)) {
			return nil
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(mapID, sx, sy), tx, ty, label); err != nil {
			return fmt.Errorf("gen2 %s: stage interaction: %w", label, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 %s: settle interaction: %w", label, err)
		}
		if complete(profile.DecodeFirstShipVoyage(m)) && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: %s ended without its retail event", errGSSecondBadgeUnexpectedState, label)
}

// The retail B1F sailor may occupy (30,6) or (31,6) depending on which
// side's coord event positioned him. Try the east approach first (the ship's
// east staircase), then the west. Only absence of the retail informed event
// permits the alternate approach; navigation and combat failures propagate.
func gsAquaInformDutySailor(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	if profile.DecodeFirstShipVoyage(m).InformedSailor {
		return nil
	}
	for _, x := range []uint8{31, 30} {
		err := gsAquaTalk(m, romData, profile, "FAST_SHIP_B1F",
			x, 7, x, 6, "ss-aqua:inform-duty-sailor",
			func(s gsprofile.FirstShipVoyageState) bool { return s.InformedSailor })
		if err == nil {
			return nil
		}
		if !errors.Is(err, errGSSecondBadgeUnexpectedState) {
			return err
		}
	}
	return fmt.Errorf("%w: neither B1F sailor position set the informed event", errGSSecondBadgeUnexpectedState)
}

// executeGSFirstKantoLanding owns only the first Olivine->Vermilion voyage.
// Unlike later trips, it must find the off-duty sailor, win his battle, help
// the granddaughter, and speak to the entrance sailor to disembark. The
// final event is set in VermilionPortLeaveShipScript, never on the boat.
func executeGSFirstKantoLanding(m *emu.Emu, romData []byte) error {
	const label = "ss-aqua:first-kanto-landing"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoFirstLanding, gsprofile.ProgressSSTicketReceived, label)
	if err != nil || done {
		return err
	}
	world := profile.DecodeOverworld(m)
	aboard := gsMapIn(world.NativeMapID, gsAquaShipMaps)
	if !world.Controllable {
		if !aboard {
			// Boarding choices and title screens are not ordinary scripted
			// dialogue: without a known prompt owner, guessing A is unsafe.
			return fmt.Errorf("%w: first voyage requires an owned, stable port boundary", errGSSecondBadgeUnexpectedState)
		}
		// A ship script (including the lazy-sailor battle or the
		// granddaughter scene) is safe to settle and re-observe on resume.
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 S.S. Aqua: resume ship scene: %w", err)
		}
	}
	world = profile.DecodeOverworld(m)
	if !gsMapIn(world.NativeMapID, gsAquaShipMaps) {
		if err := gsAquaMissingPrerequisites(profile.DecodeRegionTransports(m)); err != nil {
			return err
		}
		if err := gsAquaBoardFirstVoyage(m, romData, profile); err != nil {
			return err
		}
	}
	// B1F's duty sailor must first reveal the off-duty sailor in the NNW
	// cabin. After the trainer battle, the captain-side corridor opens.
	if err := gsAquaInformDutySailor(m, romData, profile); err != nil {
		return err
	}
	stages := []struct {
		name           string
		mapName        string
		sx, sy, tx, ty uint8
		complete       func(gsprofile.FirstShipVoyageState) bool
	}{
		{"recover-lazy-sailor", "FAST_SHIP_CABINS_NNW_NNE_NE", 4, 25, 4, 26,
			func(s gsprofile.FirstShipVoyageState) bool { return s.RecoveredSailor }},
		{"find-granddaughter", "FAST_SHIP_CABINS_SE_SSE_CAPTAINS_CABIN", 2, 24, 2, 25,
			func(s gsprofile.FirstShipVoyageState) bool { return s.FoundGirl && s.Arrived }},
	}
	for _, stage := range stages {
		if err := gsAquaTalk(m, romData, profile, stage.mapName,
			stage.sx, stage.sy, stage.tx, stage.ty,
			"ss-aqua:"+stage.name, stage.complete); err != nil {
			return err
		}
	}
	// The first-trip flag is still false until the player has left the
	// ship through the sailor's retail disembarkation script.
	ship, err := gsOpeningMapID("FAST_SHIP_1F")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(ship, 25, 3),
		25, 2, label); err != nil {
		return fmt.Errorf("gen2 S.S. Aqua: return to boarding sailor: %w", err)
	}
	m.StepFrames(gsLakeScriptSettleFrames)
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
		return fmt.Errorf("gen2 S.S. Aqua: disembark: %w", err)
	}
	vermilion, err := gsOpeningMapID("VERMILION_PORT")
	if err != nil {
		return err
	}
	for i := 0; i < 900; i++ {
		world := profile.DecodeOverworld(m)
		if world.NativeMapID != vermilion {
			return fmt.Errorf("%w: S.S. Aqua left ship for map %#04x, want Vermilion Port", errGSSecondBadgeUnexpectedState, world.NativeMapID)
		}
		if profile.DecodeFirstShipVoyage(m).FirstTrip && world.Controllable {
			return nil
		}
		if world.InDialogue {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return err
			}
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("%w: Vermilion Port did not set the first-voyage event", errGSSecondBadgeUnexpectedState)
}
