package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

var (
	errGSFirstBadgeStalled         = errors.New("gen2 first-badge progression made no progress")
	errGSFirstBadgeUnexpectedState = errors.New("gen2 first-badge progression reached an unowned state")
)

const (
	gsFirstBadgeScriptFrameBudget uint64 = 180_000
	gsFirstBadgeMaxScriptPresses         = 1_200
	gsFirstBadgeRouteAttempts            = 80
)

func gsFirstBadgeOwnedMap(mapID uint16) bool {
	for _, name := range []string{
		"ELMS_LAB",
		"NEW_BARK_TOWN",
		"ROUTE_29",
		"CHERRYGROVE_CITY",
		"CHERRYGROVE_POKECENTER_1F",
		"ROUTE_30",
		"ROUTE_31",
		"ROUTE_31_VIOLET_GATE",
		"VIOLET_CITY",
		"VIOLET_POKECENTER_1F",
		"SPROUT_TOWER_1F",
		"SPROUT_TOWER_2F",
		"SPROUT_TOWER_3F",
		"VIOLET_GYM",
	} {
		id, err := gsOpeningMapID(name)
		if err == nil && mapID == id {
			return true
		}
	}
	return false
}

func gsFirstBadgeProgressComplete(profile *gsprofile.Profile, m *emu.Emu, id game.ProgressID) bool {
	if profile == nil || m == nil {
		return false
	}
	return profile.DecodeFirstBadgeProgress(m).Has(id)
}

// driveGSFirstBadgeInterruption owns only mandatory scripts and encounters on
// the verified Elm -> Violet -> Sprout Tower -> Falkner corridor. Trainer
// sightlines, the Sprout rival scene and leader introductions are deterministic
// interruptions: A advances their text, while every battle is delegated to the
// shared semantic battle controller and converted to a typed required-battle
// outcome before progression may continue.
func driveGSFirstBadgeInterruption(
	m *emu.Emu,
	profile *gsprofile.Profile,
	encounter string,
) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing emulator/profile", errGSFirstBadgeUnexpectedState)
	}
	start := m.FrameCount()
	presses := 0
	for m.FrameCount()-start < gsFirstBadgeScriptFrameBudget {
		world := profile.DecodeOverworld(m)
		if !gsFirstBadgeOwnedMap(world.NativeMapID) {
			return fmt.Errorf("%w: map=%#04x at (%d,%d)", errGSFirstBadgeUnexpectedState, world.NativeMapID, world.X, world.Y)
		}
		if world.InBattle {
			battle, ok := profile.DecodeBattleState(m)
			if !ok {
				return fmt.Errorf("%w: battle mode has no semantic state on map %#04x", errGSFirstBadgeUnexpectedState, world.NativeMapID)
			}
			result, err := skill.Battle(m, skill.FirstUsableMove)
			if err != nil {
				return fmt.Errorf("gen2 first badge: battle %q: %w", encounter, err)
			}
			if battle.Kind == game.BattleTrainer {
				if err := skill.RequireTrainerBattleWin(encounter, result); err != nil {
					return err
				}
			} else if err := skill.RequireBattleWin(encounter, result); err != nil {
				return err
			}
			continue
		}
		if world.Controllable {
			return nil
		}
		if world.InDialogue && world.MovementIdle {
			if presses >= gsFirstBadgeMaxScriptPresses {
				return fmt.Errorf("%w: exceeded %d owned inputs on map %#04x", errGSFirstBadgeStalled, gsFirstBadgeMaxScriptPresses, world.NativeMapID)
			}
			m.Tap(emu.A, 3, 7)
			presses++
			continue
		}
		m.StepFrame()
	}
	world := profile.DecodeOverworld(m)
	return fmt.Errorf("%w: interruption exceeded %d frames on map %#04x at (%d,%d)",
		errGSFirstBadgeStalled, gsFirstBadgeScriptFrameBudget, world.NativeMapID, world.X, world.Y)
}

func gsFirstBadgeGoTo(
	m *emu.Emu,
	romData []byte,
	profile *gsprofile.Profile,
	dest skill.NativeDestination,
) error {
	mem := skill.NewNativeRouteMemory()
	for attempt := 0; attempt < gsFirstBadgeRouteAttempts; attempt++ {
		err := skill.GoToNativeRemembering(m, romData, dest, mem)
		if err == nil {
			return nil
		}
		switch {
		case errors.Is(err, skill.ErrBattle), errors.Is(err, skill.ErrDialogueInterrupted):
			world := profile.DecodeOverworld(m)
			label := fmt.Sprintf("gen2:first-badge-route:%04x", world.NativeMapID)
			if settleErr := driveGSFirstBadgeInterruption(m, profile, label); settleErr != nil {
				return settleErr
			}
		default:
			return err
		}
	}
	return fmt.Errorf("%w: route did not settle after %d interruptions", errGSFirstBadgeStalled, gsFirstBadgeRouteAttempts)
}

func executeGSSproutTower(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Sprout Tower: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSproutTowerCleared) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSFirstBadgeInterruption(m, profile, "sprout:sage_li")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressMysteryEggReturned) {
		return fmt.Errorf("%w: Sprout Tower requires the Mystery Egg return boundary", errGSFirstBadgeUnexpectedState)
	}

	tower3, err := gsOpeningMapID("SPROUT_TOWER_3F")
	if err != nil {
		return err
	}
	// Sage Li is at (10,2). Standing at (10,3) also forces the route through
	// the 3F rival scene before the final interaction.
	if err := gsFirstBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(tower3, 10, 3)); err != nil {
		return fmt.Errorf("gen2 Sprout Tower: reach Sage Li: %w", err)
	}
	if err := skill.Face(m, 10, 2); err != nil {
		return fmt.Errorf("gen2 Sprout Tower: face Sage Li: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSFirstBadgeInterruption(m, profile, "sprout:sage_li"); err != nil {
		return fmt.Errorf("gen2 Sprout Tower: Sage Li: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSproutTowerCleared) {
		return fmt.Errorf("%w: Sage Li script returned without the FLASH handoff", errGSFirstBadgeUnexpectedState)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Sprout Tower completed without stable overworld control", errGSFirstBadgeUnexpectedState)
	}
	return nil
}

func executeGSFalkner(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Falkner: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressZephyrBadgeEarned) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSFirstBadgeInterruption(m, profile, "gym:falkner")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSproutTowerCleared) {
		return fmt.Errorf("%w: Falkner requires Sprout Tower completion", errGSFirstBadgeUnexpectedState)
	}

	gym, err := gsOpeningMapID("VIOLET_GYM")
	if err != nil {
		return err
	}
	// Falkner stands at (5,1); the approach naturally crosses both gym trainer
	// sightlines when their scripts still own the corridor.
	if err := gsFirstBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, 5, 2)); err != nil {
		return fmt.Errorf("gen2 Falkner: reach leader: %w", err)
	}
	if err := skill.Face(m, 5, 1); err != nil {
		return fmt.Errorf("gen2 Falkner: face leader: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSFirstBadgeInterruption(m, profile, "gym:falkner"); err != nil {
		return fmt.Errorf("gen2 Falkner: battle/script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressZephyrBadgeEarned) {
		return fmt.Errorf("%w: Falkner script returned without Zephyr Badge", errGSFirstBadgeUnexpectedState)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Falkner completed without stable overworld control", errGSFirstBadgeUnexpectedState)
	}
	return nil
}

// executeGSTogepiEggPickup owns the mandatory post-Falkner handoff that opens
// Route 32. The retail script waits in Violet's Pokemon Center after Elm's
// phone call, asks a YES/NO question, gives the egg, then advances Route 32's
// scene. The existing interruption driver safely accepts the default YES and
// waits for the aide's exit movement to return stable overworld control.
func executeGSTogepiEggPickup(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Togepi egg: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressTogepiEggReceived) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSFirstBadgeInterruption(m, profile, "violet:elms_aide_togepi_egg")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressZephyrBadgeEarned) {
		return fmt.Errorf("%w: Togepi Egg handoff requires the Zephyr Badge", errGSFirstBadgeUnexpectedState)
	}

	center, err := gsOpeningMapID("VIOLET_POKECENTER_1F")
	if err != nil {
		return err
	}
	// VioletPokecenter1F.asm places Elm's aide at (4,3), facing down.
	if err := gsFirstBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(center, 4, 4)); err != nil {
		return fmt.Errorf("gen2 Togepi egg: reach Elm's aide: %w", err)
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressTogepiEggReceived) {
		return nil
	}
	if err := skill.Face(m, 4, 3); err != nil {
		return fmt.Errorf("gen2 Togepi egg: face Elm's aide: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSFirstBadgeInterruption(m, profile, "violet:elms_aide_togepi_egg"); err != nil {
		return fmt.Errorf("gen2 Togepi egg: aide script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressTogepiEggReceived) {
		return fmt.Errorf("%w: Elm's aide script returned without the Togepi Egg event", errGSFirstBadgeUnexpectedState)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Togepi Egg handoff completed without stable overworld control", errGSFirstBadgeUnexpectedState)
	}
	return nil
}
