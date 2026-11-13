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
	gsLeagueTrainerX       uint8 = 5
	gsLeagueTrainerY       uint8 = 7
	gsLeagueTrainerStandX  uint8 = 5
	gsLeagueTrainerStandY  uint8 = 8
	gsLeagueMemberAttempts       = 4
)

// Each member's post-battle event is a separate durable objective. An open
// exit is not inferred from reaching a map; the retail battle script sets
// the event and opens the door only after the dialogue has settled.
type gsLeagueMemberSpec struct {
	progress     game.ProgressID
	previous     game.ProgressID
	previousRoom string
	room         string
	encounter    string
}

var gsLeagueMembers = [...]gsLeagueMemberSpec{
	{
		progress:     gsprofile.ProgressLeagueKogaDefeated,
		previous:     gsprofile.ProgressLeagueWillDefeated,
		previousRoom: "WILLS_ROOM",
		room:         "KOGAS_ROOM",
		encounter:    "league:koga",
	},
	{
		progress:     gsprofile.ProgressLeagueBrunoDefeated,
		previous:     gsprofile.ProgressLeagueKogaDefeated,
		previousRoom: "KOGAS_ROOM",
		room:         "BRUNOS_ROOM",
		encounter:    "league:bruno",
	},
	{
		progress:     gsprofile.ProgressLeagueKarenDefeated,
		previous:     gsprofile.ProgressLeagueBrunoDefeated,
		previousRoom: "BRUNOS_ROOM",
		room:         "KARENS_ROOM",
		encounter:    "league:karen",
	},
}

// errGSLeagueAttemptReset is a recoverable boundary: the current Elite Four
// attempt no longer exists, so the planner must start again from Will.
var errGSLeagueAttemptReset = errors.New("gen2 League attempt was reset")

// gsLeagueResetAttempt owns a lost League attempt. A whiteout inside the
// League respawns on Route 23 (spawn_points.asm SPAWN_INDIGO) with the earlier
// EVENT_BEAT_ELITE_4_* flags still set, but the only way back in is the Indigo
// Center, whose MAPCALLBACK_NEWMAP clears all of them. Heal there, prove the
// reset happened, and hand the replan a typed outcome instead of resuming a
// room that cannot be reached.
//
// A resume elsewhere (a planner detour with the flags still set) first flies
// to the Indigo landing from Blackthorn's outdoor map, as the plateau approach
// does; Fly only shortens the trip, so without it the owned route walks.
func gsLeagueResetAttempt(m *emu.Emu, romData []byte, profile *gsprofile.Profile, label string) error {
	if !gsLeagueApproachMap(profile.DecodeOverworld(m).NativeMapID) {
		blackthorn, err := gsOpeningMapID("BLACKTHORN_CITY")
		if err != nil {
			return err
		}
		route23, err := gsOpeningMapID("ROUTE_23")
		if err != nil {
			return err
		}
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.NativeMapDestination(blackthorn)); err != nil {
			return fmt.Errorf("gen2 %s: reach Blackthorn before flying to Indigo Plateau: %w", label, err)
		}
		if err := skill.UseNativeFly(m, route23); err != nil && !errors.Is(err, skill.ErrFieldMovePrerequisite) {
			return fmt.Errorf("gen2 %s: fly to Indigo Plateau: %w", label, err)
		}
	}
	if err := gsCoastalHeal(m, romData, profile, "INDIGO_PLATEAU_POKECENTER_1F"); err != nil {
		return fmt.Errorf("gen2 %s: return to Indigo Plateau after a League whiteout: %w", label, err)
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressLeagueWillDefeated) {
		return fmt.Errorf("%w: %s: Indigo Center kept Will's victory event", errGSSecondBadgeUnexpectedState, label)
	}
	return fmt.Errorf("%w: %s resumed outside the League; Indigo Center cleared the attempt", errGSLeagueAttemptReset, label)
}

func gsLeagueMemberForProgress(progress game.ProgressID) (gsLeagueMemberSpec, bool) {
	for _, member := range gsLeagueMembers {
		if member.progress == progress {
			return member, true
		}
	}
	return gsLeagueMemberSpec{}, false
}

// executeGSLeagueMember owns one of the three middle Elite Four rooms. The
// prior member's event is required before advancing through its unlocked exit.
// Checkpoints in either the previous room or the current locked room resume
// without visiting Indigo Plateau: that Pokecenter's MAPCALLBACK_NEWMAP would
// clear the entire current League attempt, including all victory flags.
//
// There is no free Center between League battles. Before entering the next
// trainer room, use the Gen2 native field-item menu for bounded recovery;
// never use the Red/Blue item IDs or a synthetic recovery state.
func executeGSLeagueMember(m *emu.Emu, romData []byte, progress game.ProgressID) error {
	spec, ok := gsLeagueMemberForProgress(progress)
	if !ok {
		return fmt.Errorf("%w: unsupported League progress %s", errGSControllerUnavailable, progress)
	}
	profile, done, err := gsCoastalProfile(m, romData, spec.progress, spec.previous, spec.encounter)
	if err != nil || done {
		return err
	}
	prevID, err := gsOpeningMapID(spec.previousRoom)
	if err != nil {
		return err
	}
	roomID, err := gsOpeningMapID(spec.room)
	if err != nil {
		return err
	}
	// The League rooms lock behind the player and the only way back in is the
	// Indigo Center, which clears every victory flag. Any resume outside the
	// two owned rooms (a whiteout spawn or a planner detour) is a lost attempt.
	if here := profile.DecodeOverworld(m).NativeMapID; here != prevID && here != roomID {
		return gsLeagueResetAttempt(m, romData, profile, spec.encounter)
	}
	complete := func() bool {
		return gsFirstBadgeProgressComplete(profile, m, spec.progress)
	}
	prepared := false
	for attempt := 0; attempt < gsLeagueMemberAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, spec.encounter); err != nil {
				return fmt.Errorf("gen2 %s: settle room entrance or battle: %w", spec.encounter, err)
			}
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if !prepared && profile.DecodeOverworld(m).Controllable {
			if err := gsLeaguePrepare(m, profile, spec.encounter); err != nil {
				return fmt.Errorf("gen2 %s: prepare finite medicine before battle: %w", spec.encounter, err)
			}
			prepared = true
		}
		if profile.DecodeOverworld(m).NativeMapID == prevID {
			if err := gsSecondBadgeGoTo(m, romData, profile, skill.NativeMapDestination(roomID)); err != nil {
				return fmt.Errorf("gen2 %s: cross the previous member's opened exit: %w", spec.encounter, err)
			}
			// The warp can report the destination map before the retail
			// entrance autowalk locks control and closes the door.
			m.StepFrames(gsLakeScriptSettleFrames)
			if !profile.DecodeOverworld(m).Controllable {
				if err := driveGSSecondBadgeInterruption(m, profile, spec.encounter); err != nil {
					return fmt.Errorf("gen2 %s: settle locked entrance: %w", spec.encounter, err)
				}
			}
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		if got := profile.DecodeOverworld(m).NativeMapID; got != roomID {
			return fmt.Errorf("%w: %s entered map %#04x, want %#04x", errGSSecondBadgeUnexpectedState, spec.encounter, got, roomID)
		}
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(roomID, gsLeagueTrainerStandX, gsLeagueTrainerStandY),
			gsLeagueTrainerX, gsLeagueTrainerY, spec.encounter); err != nil {
			return fmt.Errorf("gen2 %s: reach and face the trainer: %w", spec.encounter, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, spec.encounter); err != nil {
			return fmt.Errorf("gen2 %s: trainer battle or post-battle dialogue: %w", spec.encounter, err)
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: %s returned without its retail victory event", errGSSecondBadgeUnexpectedState, spec.encounter)
}
