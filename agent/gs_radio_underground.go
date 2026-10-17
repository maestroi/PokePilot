package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsRadioUndergroundAttempts       = 3
	gsRadioBasementDoorX       uint8 = 18
	gsRadioBasementDoorY       uint8 = 6
	gsRadioCardShutterX        uint8 = 14
	gsRadioCardShutterY        uint8 = 2
)

// gsRadioScriptInteraction drives a named background or object event at a
// stable map tile and accepts ONLY the cartridge postcondition. A Yes/No
// prompt is answered by the shared owned script driver (retail defaults YES).
func gsRadioScriptInteraction(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	mapName string, standX, standY, targetX, targetY uint8, label string,
	complete func() bool,
) error {
	mapID, err := gsOpeningMapID(mapName)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRadioUndergroundAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
				return fmt.Errorf("gen2 Radio Tower %s: resume script: %w", label, err)
			}
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(mapID, standX, standY),
			targetX, targetY, label); err != nil {
			return fmt.Errorf("gen2 Radio Tower %s: approach: %w", label, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 Radio Tower %s: settle: %w", label, err)
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Radio Tower %s did not set its retail event",
		errGSSecondBadgeUnexpectedState, label)
}

// executeGSRadioTowerBasementDoor uses the Basement Key obtained from the
// false Director on Radio Tower 5F. GoldenrodUnderground.asm owns the locked
// door at (18,6); EVENT_USED_BASEMENT_KEY is the only durable unlock proof.
// The tile can also be a warp, but navigating through it cannot unlock it.
func executeGSRadioTowerBasementDoor(m *emu.Emu, romData []byte) error {
	const encounter = "radio-tower:basement-key-door"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRadioTowerBasementUnlocked,
		gsprofile.ProgressRadioTowerFakeDirectorBeaten, encounter)
	if err != nil || done {
		return err
	}
	return gsRadioScriptInteraction(m, romData, profile, "GOLDENROD_UNDERGROUND",
		18, 7, gsRadioBasementDoorX, gsRadioBasementDoorY, encounter,
		func() bool {
			return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRadioTowerBasementUnlocked)
		})
}

// gsRadioWarehouseSwitchOrder is the RETAIL puzzle solution, not a generic
// press-all-switches shortcut. In SwitchRoomEntrances.asm the switches are
// numbered right-to-left (x=16,10,2), and the final door 11 opens only when
// the order 3,2,1 yields sum 6. The emergency switch is not used.
type gsRadioSwitch struct {
	name           string
	x, y           uint8
	standX, standY uint8
	enabled        func(gsprofile.RadioTowerSwitches) bool
}

var gsRadioWarehouseSwitchOrder = []gsRadioSwitch{
	{"3", 2, 1, 2, 2, func(s gsprofile.RadioTowerSwitches) bool { return s.Switch3 }},
	{"2", 10, 1, 10, 2, func(s gsprofile.RadioTowerSwitches) bool { return s.Switch2 }},
	{"1", 16, 1, 16, 2, func(s gsprofile.RadioTowerSwitches) bool { return s.Switch1 }},
}

// gsRadioResetSwitchRoom deliberately re-enters the Goldenrod Underground.
// Its retail MAPCALLBACK_NEWMAP clears volatile switches and shutters, making
// an interrupted maze replay restartable rather than trying to guess old
// door states from the character's current location.
func gsRadioResetSwitchRoom(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	tunnel, err := gsOpeningMapID("GOLDENROD_UNDERGROUND")
	if err != nil {
		return err
	}
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(tunnel, 22, 27)); err != nil {
		return fmt.Errorf("gen2 Radio Tower: reset maze through Underground: %w", err)
	}
	// A re-entry into the tunnel is mandatory if the player was already
	// inside the switch room. If already here, only a fresh all-off puzzle
	// qualifies as a clean start.
	switches := profile.DecodeRadioTowerSwitches(m)
	if switches.Switch1 || switches.Switch2 || switches.Switch3 ||
		switches.Emergency || switches.FinalDoorOpen {
		return fmt.Errorf("%w: Goldenrod Underground reset callback left an active switch/door",
			errGSSecondBadgeUnexpectedState)
	}
	return nil
}

func gsRadioSolveWarehouseSwitches(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	room := "GOLDENROD_UNDERGROUND_SWITCH_ROOM_ENTRANCES"
	for _, sw := range gsRadioWarehouseSwitchOrder {
		state := profile.DecodeRadioTowerSwitches(m)
		if sw.enabled(state) {
			return fmt.Errorf("%w: %s was on before its 3-2-1 turn",
				errGSSecondBadgeUnexpectedState, sw.name)
		}
		label := "radio-tower:switch-" + sw.name
		if err := gsRadioScriptInteraction(m, romData, profile, room,
			sw.standX, sw.standY, sw.x, sw.y, label,
			func() bool { return sw.enabled(profile.DecodeRadioTowerSwitches(m)) }); err != nil {
			return err
		}
	}
	state := profile.DecodeRadioTowerSwitches(m)
	if !state.Switch1 || !state.Switch2 || !state.Switch3 ||
		!state.FinalDoorOpen || !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: 3-2-1 basement switch sequence did not open retail door 11",
			errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSRadioTowerCardKey travels through the mandatory Underground rival
// and Rocket/Burglar trainers, solves the retail switch puzzle from a reset
// state, and talks to the *real* Director in the warehouse. The Director
// sets EVENT_RECEIVED_CARD_KEY after successfully giving the item; merely
// reaching the warehouse or winning a trainer fight cannot complete it.
func executeGSRadioTowerCardKey(m *emu.Emu, romData []byte) error {
	const encounter = "radio-tower:director-card-key"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRadioTowerCardKeyReceived,
		gsprofile.ProgressRadioTowerBasementUnlocked, encounter)
	if err != nil || done {
		return err
	}
	if err := gsCoastalHeal(m, romData, profile, "GOLDENROD_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 Radio Tower: pre-warehouse recovery: %w", err)
	}
	if err := gsRadioResetSwitchRoom(m, romData, profile); err != nil {
		return err
	}
	if err := gsRadioSolveWarehouseSwitches(m, romData, profile); err != nil {
		return err
	}
	return gsRadioScriptInteraction(m, romData, profile,
		"GOLDENROD_UNDERGROUND_WAREHOUSE",
		12, 9, 12, 8, encounter,
		func() bool {
			return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRadioTowerCardKeyReceived)
		})
}

// executeGSRadioTowerShutter uses the Card Key at RadioTower3F.asm's
// BGEVENT_UP (14,2). The event only flips after the actual scripted item
// interaction and changeblock, so it remains a reliable restart boundary.
func executeGSRadioTowerShutter(m *emu.Emu, romData []byte) error {
	const encounter = "radio-tower:3f-card-key-shutter"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRadioTowerShutterOpened,
		gsprofile.ProgressRadioTowerCardKeyReceived, encounter)
	if err != nil || done {
		return err
	}
	return gsRadioScriptInteraction(m, romData, profile, "RADIO_TOWER_3F",
		14, 3, gsRadioCardShutterX, gsRadioCardShutterY, encounter,
		func() bool {
			return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRadioTowerShutterOpened)
		})
}
