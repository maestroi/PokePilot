package agent

import (
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
// There is no free Center between League battles. Resource preparation is a
// separate future slice: this controller must not manufacture HP/PP or use
// the Gen-I-only League item flow on a Gen-II cartridge.
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
	start := profile.DecodeOverworld(m)
	if start.NativeMapID != prevID && start.NativeMapID != roomID {
		return fmt.Errorf("%w: %s must resume in %s or %s, observed map %#04x",
			errGSSecondBadgeUnexpectedState, spec.encounter, spec.previousRoom, spec.room, start.NativeMapID)
	}
	complete := func() bool {
		return gsFirstBadgeProgressComplete(profile, m, spec.progress)
	}
	for attempt := 0; attempt < gsLeagueMemberAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, spec.encounter); err != nil {
				return fmt.Errorf("gen2 %s: settle room entrance or battle: %w", spec.encounter, err)
			}
		}
		if complete() && profile.DecodeOverworld(m).Controllable {
			return nil
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
