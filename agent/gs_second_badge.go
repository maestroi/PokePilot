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
	errGSSecondBadgeStalled         = errors.New("gen2 second-badge progression made no progress")
	errGSSecondBadgeUnexpectedState = errors.New("gen2 second-badge progression reached an unowned state")
)

const (
	gsSecondBadgeScriptFrameBudget uint64 = 240_000
	gsSecondBadgeMaxScriptPresses         = 1_800
	gsSecondBadgeRouteAttempts            = 160
)

func gsSecondBadgeOwnedMap(mapID uint16) bool {
	for _, name := range []string{
		"VIOLET_CITY",
		"VIOLET_POKECENTER_1F",
		"ROUTE_32",
		"UNION_CAVE_1F",
		"ROUTE_33",
		"AZALEA_TOWN",
		"AZALEA_POKECENTER_1F",
		"AZALEA_GYM",
		"KURTS_HOUSE",
		"SLOWPOKE_WELL_B1F",
		"ILEX_FOREST_AZALEA_GATE",
		"ILEX_FOREST",
		"ROUTE_34_ILEX_FOREST_GATE",
		"ROUTE_34",
		"GOLDENROD_CITY",
		"GOLDENROD_GYM",
		"GOLDENROD_FLOWER_SHOP",
		"GOLDENROD_POKECENTER_1F",
		"ROUTE_35_GOLDENROD_GATE",
		"ROUTE_35",
		"ROUTE_36",
		"ROUTE_37",
		"ECRUTEAK_CITY",
		"ECRUTEAK_POKECENTER_1F",
		"ECRUTEAK_GYM",
		"BURNED_TOWER_1F",
		"BURNED_TOWER_B1F",
	} {
		id, err := gsOpeningMapID(name)
		if err == nil && mapID == id {
			return true
		}
	}
	return false
}

func driveGSSecondBadgeInterruption(
	m *emu.Emu,
	profile *gsprofile.Profile,
	encounter string,
) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing emulator/profile", errGSSecondBadgeUnexpectedState)
	}
	start := m.FrameCount()
	presses := 0
	for m.FrameCount()-start < gsSecondBadgeScriptFrameBudget {
		if answerGSNicknameSurface(m, profile) {
			if presses >= gsSecondBadgeMaxScriptPresses {
				return fmt.Errorf("%w: exceeded %d owned inputs at a nickname prompt", errGSSecondBadgeStalled, gsSecondBadgeMaxScriptPresses)
			}
			presses++
			continue
		}
		world := profile.DecodeOverworld(m)
		if !gsSecondBadgeOwnedMap(world.NativeMapID) {
			return fmt.Errorf("%w: map=%#04x at (%d,%d)", errGSSecondBadgeUnexpectedState, world.NativeMapID, world.X, world.Y)
		}
		if world.InBattle {
			battle, ok := profile.DecodeBattleState(m)
			if !ok {
				return fmt.Errorf("%w: battle mode has no semantic state on map %#04x", errGSSecondBadgeUnexpectedState, world.NativeMapID)
			}
			result, err := skill.Battle(m, skill.StatAwareMove(m.ROM()))
			if err != nil {
				return fmt.Errorf("gen2 second badge: battle %q: %w", encounter, err)
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
			if presses >= gsSecondBadgeMaxScriptPresses {
				return fmt.Errorf("%w: exceeded %d owned inputs on map %#04x", errGSSecondBadgeStalled, gsSecondBadgeMaxScriptPresses, world.NativeMapID)
			}
			m.Tap(emu.A, 3, 7)
			presses++
			continue
		}
		m.StepFrame()
	}
	world := profile.DecodeOverworld(m)
	return fmt.Errorf("%w: interruption exceeded %d frames on map %#04x at (%d,%d)",
		errGSSecondBadgeStalled, gsSecondBadgeScriptFrameBudget, world.NativeMapID, world.X, world.Y)
}

func gsSecondBadgeGoTo(
	m *emu.Emu,
	romData []byte,
	profile *gsprofile.Profile,
	dest skill.NativeDestination,
) error {
	// One route memory per journey: a battle interruption must not make the
	// retry forget which edges and entries it already proved dead.
	mem := skill.NewNativeRouteMemory()
	for attempt := 0; attempt < gsSecondBadgeRouteAttempts; attempt++ {
		err := skill.GoToNativeRemembering(m, romData, dest, mem)
		if err == nil {
			return nil
		}
		switch {
		case errors.Is(err, skill.ErrBattle), errors.Is(err, skill.ErrDialogueInterrupted):
			world := profile.DecodeOverworld(m)
			label := fmt.Sprintf("gen2:second-badge-route:%04x", world.NativeMapID)
			if settleErr := driveGSSecondBadgeInterruption(m, profile, label); settleErr != nil {
				return settleErr
			}
		default:
			return err
		}
	}
	return fmt.Errorf("%w: route did not settle after %d interruptions", errGSSecondBadgeStalled, gsSecondBadgeRouteAttempts)
}

// gsSecondBadgeFaceFrom stages on dest and turns toward (tx,ty), leaving the
// player controllable and ready for the interaction press. A wild encounter
// rolled by the arrival step surfaces as Face's ErrBattle; it is fought by the
// owned interruption driver and the staging is retried, because neither the
// tile nor the target changed.
func gsSecondBadgeFaceFrom(
	m *emu.Emu,
	romData []byte,
	profile *gsprofile.Profile,
	dest skill.NativeDestination,
	tx, ty uint8,
	encounter string,
) error {
	for attempt := 0; attempt < gsSecondBadgeRouteAttempts; attempt++ {
		if err := gsSecondBadgeGoTo(m, romData, profile, dest); err != nil {
			return fmt.Errorf("reach staging tile: %w", err)
		}
		err := skill.Face(m, tx, ty)
		if err == nil {
			return nil
		}
		if !errors.Is(err, skill.ErrBattle) {
			return fmt.Errorf("face (%d,%d): %w", tx, ty, err)
		}
		if settleErr := driveGSSecondBadgeInterruption(m, profile, encounter); settleErr != nil {
			return settleErr
		}
	}
	return fmt.Errorf("%w: facing (%d,%d) did not settle after %d interruptions", errGSSecondBadgeStalled, tx, ty, gsSecondBadgeRouteAttempts)
}

// executeGSSlowpokeWell continues from the durable Togepi-Egg handoff through
// Route 32, Union Cave and Route 33. In Azalea it explicitly triggers Kurt's
// retail story script, then clears the four-Rocket B1F corridor. Completion is
// the cartridge EVENT_CLEARED_SLOWPOKE_WELL bit; the final Rocket script also
// warps the player back to Kurt's house and heals the party.
func executeGSSlowpokeWell(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Slowpoke Well: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSlowpokeWellCleared) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "slowpoke-well:completion")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressTogepiEggReceived) {
		return fmt.Errorf("%w: Slowpoke Well requires the post-Falkner Togepi Egg handoff", errGSSecondBadgeUnexpectedState)
	}

	kurtHouse, err := gsOpeningMapID("KURTS_HOUSE")
	if err != nil {
		return err
	}
	// Kurt's initial object is at (3,2). Talking from (3,3) makes him run to
	// Slowpoke Well and removes the Rocket blocking Azalea's well entrance.
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(kurtHouse, 3, 3)); err != nil {
		return fmt.Errorf("gen2 Slowpoke Well: reach Kurt: %w", err)
	}
	if err := skill.Face(m, 3, 2); err != nil {
		return fmt.Errorf("gen2 Slowpoke Well: face Kurt: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "azalea:kurt"); err != nil {
		return fmt.Errorf("gen2 Slowpoke Well: Kurt script: %w", err)
	}

	well, err := gsOpeningMapID("SLOWPOKE_WELL_B1F")
	if err != nil {
		return err
	}
	// Grunt M1 at (5,2) owns the victory script. Reaching (5,3) naturally
	// crosses the other Rocket sightlines; the shared battle controller handles
	// each mandatory trainer battle and any wild cave encounter.
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(well, 5, 3)); err != nil {
		return fmt.Errorf("gen2 Slowpoke Well: reach final Rocket: %w", err)
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSlowpokeWellCleared) {
		return nil
	}
	if err := skill.Face(m, 5, 2); err != nil {
		return fmt.Errorf("gen2 Slowpoke Well: face final Rocket: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "slowpoke-well:rocket-grunt-m1"); err != nil {
		return fmt.Errorf("gen2 Slowpoke Well: final Rocket: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSlowpokeWellCleared) {
		return fmt.Errorf("%w: final Rocket script returned without Slowpoke Well completion", errGSSecondBadgeUnexpectedState)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Slowpoke Well cleared without stable overworld control", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

// executeGSBugsy owns the second Johto gym after Slowpoke Well. Native routing
// may trigger any gym-trainer sightline; those battles and Bugsy's own battle
// are delegated to the shared semantic battle controller. The durable
// postcondition is the Hive Badge bit, which the retail script sets before the
// optional TM49 handoff, so a full item pocket cannot make a won badge look
// incomplete.
func executeGSBugsy(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Bugsy: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHiveBadgeEarned) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "gym:bugsy")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressSlowpokeWellCleared) {
		return fmt.Errorf("%w: Bugsy requires Slowpoke Well completion", errGSSecondBadgeUnexpectedState)
	}
	if err := gsEnsureLeadReadyForBugsy(m, romData, profile); err != nil {
		return fmt.Errorf("gen2 Bugsy: prepare lead: %w", err)
	}

	gym, err := gsOpeningMapID("AZALEA_GYM")
	if err != nil {
		return err
	}
	// Bugsy stands at (5,7). Route to the adjacent tile and let the normal
	// native-grid path own any trainer sightline interruptions on the way.
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, 5, 8)); err != nil {
		return fmt.Errorf("gen2 Bugsy: reach leader: %w", err)
	}
	// Gym trainers can leave the party injured. Re-heal before the leader so
	// Reflect/Growl setup still sees a healthy active mon against Scyther.
	if !profile.DecodeCenter(m).Recovered {
		if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
			return fmt.Errorf("gen2 Bugsy: recover party before leader: %w", err)
		}
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(gym, 5, 8)); err != nil {
			return fmt.Errorf("gen2 Bugsy: return to leader: %w", err)
		}
	}
	if err := skill.Face(m, 5, 7); err != nil {
		return fmt.Errorf("gen2 Bugsy: face leader: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "gym:bugsy"); err != nil {
		return fmt.Errorf("gen2 Bugsy: battle/script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHiveBadgeEarned) {
		return fmt.Errorf("%w: Bugsy script returned without Hive Badge", errGSSecondBadgeUnexpectedState)
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Bugsy completed without stable overworld control", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
