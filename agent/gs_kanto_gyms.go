package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// gsKantoGymSpec names exact retail Gold/Silver maps and leader objects.
// All seven use the existing native pathfinder and the same owned trainer
// interruption driver as Lt. Surge. Their badges are verified separately in
// gs/profile from wKantoBadges; a trainer victory event or map arrival alone
// never claims a milestone. These stages are opt-in experimental until a
// full ROM-backed journey qualifies each route and its prerequisite story.
type gsKantoGymSpec struct {
	progress, previous  game.ProgressID
	leader, gym, center string
	x, y, sx, sy        uint8
}

var gsKantoGymStages = []gsKantoGymSpec{
	{gsprofile.ProgressKantoMarshBadgeEarned, gsprofile.ProgressKantoThunderBadgeEarned,
		"sabrina", "SAFFRON_GYM", "SAFFRON_POKECENTER_1F", 9, 8, 9, 9},
	{gsprofile.ProgressKantoRainbowBadgeEarned, gsprofile.ProgressKantoMarshBadgeEarned,
		"erika", "CELADON_GYM", "CELADON_POKECENTER_1F", 5, 3, 5, 4},
	{gsprofile.ProgressKantoSoulBadgeEarned, gsprofile.ProgressKantoRainbowBadgeEarned,
		"janine", "FUCHSIA_GYM", "FUCHSIA_POKECENTER_1F", 1, 10, 1, 11},
	{gsprofile.ProgressKantoCascadeBadgeEarned, gsprofile.ProgressKantoSoulBadgeEarned,
		"misty", "CERULEAN_GYM", "CERULEAN_POKECENTER_1F", 5, 3, 5, 4},
	{gsprofile.ProgressKantoBoulderBadgeEarned, gsprofile.ProgressKantoCascadeBadgeEarned,
		"brock", "PEWTER_GYM", "PEWTER_POKECENTER_1F", 5, 1, 5, 2},
	{gsprofile.ProgressKantoVolcanoBadgeEarned, gsprofile.ProgressKantoBoulderBadgeEarned,
		"blaine", "SEAFOAM_GYM", "CINNABAR_POKECENTER_1F", 5, 2, 5, 3},
	{gsprofile.ProgressKantoEarthBadgeEarned, gsprofile.ProgressKantoVolcanoBadgeEarned,
		"blue", "VIRIDIAN_GYM", "VIRIDIAN_POKECENTER_1F", 5, 3, 5, 4},
}

// Kanto native corridors used by the experimental post-League gym stages.
// Outside these known retail map names the interruption driver fails closed;
// the autofixer can then report an unsupported crossing with a checkpoint.
var gsKantoGymOwnedMaps = []string{
	"ROUTE_6", "ROUTE_6_SAFFRON_GATE", "ROUTE_6_UNDERGROUND_PATH_ENTRANCE",
	"SAFFRON_CITY", "SAFFRON_GYM", "SAFFRON_POKECENTER_1F",
	"ROUTE_7", "ROUTE_7_SAFFRON_GATE",
	"CELADON_CITY", "CELADON_GYM", "CELADON_POKECENTER_1F",
	"ROUTE_16", "ROUTE_16_GATE", "ROUTE_17", "ROUTE_17_ROUTE_18_GATE",
	"ROUTE_18", "FUCHSIA_CITY", "FUCHSIA_GYM", "FUCHSIA_POKECENTER_1F",
	"ROUTE_15", "ROUTE_15_FUCHSIA_GATE", "ROUTE_14", "ROUTE_13",
	"ROUTE_12", "ROUTE_12_SUPER_ROD_HOUSE",
	"ROUTE_8", "ROUTE_8_SAFFRON_GATE",
	"LAVENDER_TOWN", "LAVENDER_POKECENTER_1F",
	"ROUTE_5", "ROUTE_5_SAFFRON_GATE", "ROUTE_5_UNDERGROUND_PATH_ENTRANCE",
	"CERULEAN_CITY", "CERULEAN_GYM", "CERULEAN_POKECENTER_1F",
	"ROUTE_24", "ROUTE_25", "ROUTE_4",
	"ROUTE_9", "ROUTE_10_NORTH", "ROUTE_10_POKECENTER_1F", "POWER_PLANT",
	"ROUTE_11", "DIGLETTS_CAVE", "ROUTE_2", "ROUTE_2_GATE",
	"PEWTER_CITY", "PEWTER_GYM", "PEWTER_POKECENTER_1F",
	"ROUTE_3", "ROUTE_1",
	"VIRIDIAN_CITY", "VIRIDIAN_GYM", "VIRIDIAN_POKECENTER_1F",
	"ROUTE_19", "ROUTE_19_FUCHSIA_GATE", "ROUTE_20", "SEAFOAM_GYM",
	"ROUTE_21", "CINNABAR_ISLAND", "CINNABAR_POKECENTER_1F",
	"PALLET_TOWN", "OAKS_LAB", "ROUTE_22", "ROUTE_28",
	"SILVER_CAVE_OUTSIDE", "SILVER_CAVE_POKECENTER_1F",
	"SILVER_CAVE_ROOM_1", "SILVER_CAVE_ROOM_2", "SILVER_CAVE_ROOM_3",
}

func gsKantoGymForProgress(id game.ProgressID) (gsKantoGymSpec, bool) {
	for _, spec := range gsKantoGymStages {
		if spec.progress == id {
			return spec, true
		}
	}
	return gsKantoGymSpec{}, false
}

// gsKantoGymObjectives is called only after the Hall of Fame, S.S. Aqua
// crossing and Lt. Surge objective have been observed. It never synthesizes
// an earned badge or infers an unobserved Gym win from earlier milestones.
func gsKantoGymObjectives(obs Observation) []Objective {
	for _, spec := range gsKantoGymStages {
		if obs.Story.Has(spec.progress) {
			continue // Earned out of order or carried over from a saved Kanto run.
		}
		switch spec.progress {
		case gsprofile.ProgressKantoCascadeBadgeEarned:
			// The Power Plant Manager opens the Rocket encounter and
			// Machine Part, and the Route 25 date reopens Misty's Gym.
			for _, prereq := range []struct {
				id   game.ProgressID
				note string
			}{
				{gsprofile.ProgressKantoManagerMet, "(experimental Kanto: speak to the Power Plant Manager about the stolen Machine Part)"},
				{gsprofile.ProgressKantoRocketAtGym, "(experimental Kanto: enter Cerulean Gym and trigger the Rocket thief scene)"},
				{gsprofile.ProgressKantoMachinePartFound, "(experimental Kanto: recover the hidden Machine Part in Cerulean Gym)"},
				{gsprofile.ProgressKantoPowerRestored, "(experimental Kanto: return the Machine Part and restore power to the region)"},
				{gsprofile.ProgressKantoMistyReturned, "(experimental Kanto: interrupt Misty's date on Route 25 so she returns to Cerulean Gym)"},
			} {
				if !obs.Story.Has(prereq.id) {
					return []Objective{{Kind: KindProgress, Progress: prereq.id, Note: prereq.note}}
				}
			}
		case gsprofile.ProgressKantoEarthBadgeEarned:
			if !obs.Story.Has(gsprofile.ProgressKantoBlueInvited) {
				return []Objective{{Kind: KindProgress, Progress: gsprofile.ProgressKantoBlueInvited,
					Note: "(experimental Kanto: meet Blue on Cinnabar Island to open Viridian Gym)"}}
			}
		}
		if !obs.Story.Has(spec.progress) {
			return []Objective{{Kind: KindProgress, Progress: spec.progress,
				Note: fmt.Sprintf("(experimental Kanto: navigate the native %s gym route, fight %s and verify the badge engine flag)", spec.leader, spec.leader)}}
		}
	}
	if !obs.Story.Has(gsprofile.ProgressMtSilverOpened) {
		return []Objective{{Kind: KindProgress, Progress: gsprofile.ProgressMtSilverOpened,
			Note: "(experimental Kanto: visit Oak after all sixteen badges and unlock Mt. Silver)"}}
	}
	if !obs.Story.Has(gsprofile.ProgressMtSilverRedDefeated) {
		return []Objective{{Kind: KindProgress, Progress: gsprofile.ProgressMtSilverRedDefeated,
			Note: "(experimental final boss: travel through Mt. Silver and defeat Red)"}}
	}
	return nil
}

// executeGSKantoGym explicitly owns a single Gym story, not the entire region.
// Its route is native ROM topology (including teleports, Cut and Surf); any
// still-unimplemented field/story gate becomes a real blocked diagnostic that
// the autofixer can reproduce from a checkpoint. No fake warps or badge writes.
func executeGSKantoGym(m *emu.Emu, romData []byte, id game.ProgressID) error {
	spec, ok := gsKantoGymForProgress(id)
	if !ok {
		return fmt.Errorf("%w: unknown Kanto Gym progress %s", errGSControllerUnavailable, id)
	}
	label := "kanto:gym:" + spec.leader
	profile, done, err := gsCoastalProfile(m, romData, spec.progress, spec.previous, label)
	if err != nil || done {
		return err
	}
	// A historical first ship trip does not by itself prove the player is
	// presently in Kanto. Backtracking to Johto requires a separate transport
	// controller; fail closed rather than pathing through a fictitious ferry.
	obs, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return fmt.Errorf("gen2 %s: decode region: %w", spec.leader, err)
	}
	if obs.Region != game.RegionKanto {
		return fmt.Errorf("%w: %s needs a live Kanto location, got %s on map %#04x",
			errGSSecondBadgeUnexpectedState, spec.leader, obs.Region, obs.NativeMapID)
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 %s: resume Gym script: %w", spec.leader, err)
		}
	}
	gym, err := gsOpeningMapID(spec.gym)
	if err != nil {
		return err
	}
	complete := func() bool {
		return gsFirstBadgeProgressComplete(profile, m, spec.progress)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if !profile.DecodeCenter(m).Recovered {
			if err := gsCoastalHeal(m, romData, profile, spec.center); err != nil {
				return fmt.Errorf("gen2 %s: recover at Kanto Center: %w", spec.leader, err)
			}
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(gym, spec.sx, spec.sy),
			spec.x, spec.y, label); err != nil {
			return fmt.Errorf("gen2 %s: navigate Gym and trainer sightlines: %w", spec.leader, err)
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, label); err != nil {
			return fmt.Errorf("gen2 %s: battle or badge script: %w", spec.leader, err)
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: %s interaction ended without the retail Kanto badge flag",
		errGSSecondBadgeUnexpectedState, spec.leader)
}
