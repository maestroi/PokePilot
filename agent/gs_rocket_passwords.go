package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const gsRocketInteractionAttempts = 3

// pret/pokegold maps/TeamRocketBaseB3F.asm object_const_def starts at 2.
const (
	gsRocketMurkrowSlot       = 4 // TEAMROCKETBASEB3F_MOLTRES
	gsRocketSlowpokeGruntSlot = 5 // TEAMROCKETBASEB3F_ROCKET_GIRL
	gsRocketRaticateGruntSlot = 6 // TEAMROCKETBASEB3F_ROCKET2
)

// gsRocketB3FTalk settles the actual retail trainer or character script at a
// pinned B3F object location. A battle can happen before its password gift;
// the shared interruption driver must settle both before verification.
//
// objectSlot is the pret object_const_def index for a map object (0 for a
// fixed bg/coord tile such as Giovanni's office door). Trainer sight scripts
// walk NPCs off their ROM home before the talk-after setevent runs, so the
// interaction must face the live sprite tile — facing the home tile after a
// sight battle presses A into empty space and never learns the password.
func gsRocketB3FTalk(m *emu.Emu, romData []byte, profile *gsprofile.Profile,
	standX, standY, homeX, homeY uint8, objectSlot int, encounter string,
	completed func() bool,
) error {
	mapID, err := gsOpeningMapID("TEAM_ROCKET_BASE_B3F")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsRocketInteractionAttempts; attempt++ {
		if !profile.DecodeOverworld(m).Controllable {
			if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
				return fmt.Errorf("gen2 Rocket B3F: resume %s: %w", encounter, err)
			}
		}
		if completed() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		sx, sy, tx, ty := gsRocketB3FInteractionTarget(profile, m,
			standX, standY, homeX, homeY, objectSlot)
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(mapID, sx, sy),
			tx, ty, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B3F: approach %s: %w", encounter, err)
		}
		// Sight engagement can move the sprite during the approach. Refresh
		// before the talk-after A press so password events behind
		// endifjustbattled still land on an adjacent NPC.
		if completed() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
		sx, sy, tx, ty = gsRocketB3FInteractionTarget(profile, m,
			standX, standY, homeX, homeY, objectSlot)
		ow := profile.DecodeOverworld(m)
		if ow.X != sx || ow.Y != sy || !gsOrthogonallyAdjacent(ow.X, ow.Y, tx, ty) {
			if err := gsSecondBadgeFaceFrom(m, romData, profile,
				skill.ExactNativeDestination(mapID, sx, sy),
				tx, ty, encounter); err != nil {
				return fmt.Errorf("gen2 Rocket B3F: re-face live %s: %w", encounter, err)
			}
		} else if err := skill.Face(m, tx, ty); err != nil {
			return fmt.Errorf("gen2 Rocket B3F: face live %s: %w", encounter, err)
		}
		m.StepFrames(gsLakeScriptSettleFrames)
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B3F: settle %s: %w", encounter, err)
		}
		if completed() && profile.DecodeOverworld(m).Controllable {
			return nil
		}
	}
	return fmt.Errorf("%w: Rocket B3F %s script did not set expected ROM event", errGSSecondBadgeUnexpectedState, encounter)
}

// gsRocketB3FInteractionTarget resolves the stand/face pair for a B3F
// interaction. Fixed bg tiles keep their preferred coordinates. Map objects
// refresh against live sprite RAM so sight-moved trainers stay addressable.
func gsRocketB3FInteractionTarget(
	profile *gsprofile.Profile,
	m *emu.Emu,
	preferredStandX, preferredStandY, homeX, homeY uint8,
	objectSlot int,
) (standX, standY, targetX, targetY uint8) {
	standX, standY, targetX, targetY = preferredStandX, preferredStandY, homeX, homeY
	if objectSlot <= 0 || profile == nil || m == nil {
		return
	}
	live, err := profile.DecodeLiveTopology(m)
	if err != nil {
		return
	}
	pt, ok := live.ObjectPositions[objectSlot]
	if !ok || pt.X < 0 || pt.Y < 0 || pt.X > 255 || pt.Y > 255 {
		return
	}
	targetX, targetY = uint8(pt.X), uint8(pt.Y)
	if gsOrthogonallyAdjacent(preferredStandX, preferredStandY, targetX, targetY) {
		return preferredStandX, preferredStandY, targetX, targetY
	}
	ow := profile.DecodeOverworld(m)
	standX, standY = gsOrthogonalNeighborNear(targetX, targetY,
		preferredStandX, preferredStandY, ow.X, ow.Y)
	return standX, standY, targetX, targetY
}

func gsOrthogonallyAdjacent(x0, y0, x1, y1 uint8) bool {
	dx := int(x0) - int(x1)
	if dx < 0 {
		dx = -dx
	}
	dy := int(y0) - int(y1)
	if dy < 0 {
		dy = -dy
	}
	return dx+dy == 1
}

// gsOrthogonalNeighborNear picks a tile beside (tx,ty), preferring proximity
// to the retail staging tile and then to the player.
func gsOrthogonalNeighborNear(tx, ty, preferX, preferY, playerX, playerY uint8) (uint8, uint8) {
	candidates := make([][2]uint8, 0, 4)
	if ty < 255 {
		candidates = append(candidates, [2]uint8{tx, ty + 1})
	}
	if ty > 0 {
		candidates = append(candidates, [2]uint8{tx, ty - 1})
	}
	if tx < 255 {
		candidates = append(candidates, [2]uint8{tx + 1, ty})
	}
	if tx > 0 {
		candidates = append(candidates, [2]uint8{tx - 1, ty})
	}
	if len(candidates) == 0 {
		return preferX, preferY
	}
	best := candidates[0]
	bestScore := gsTileScore(best[0], best[1], preferX, preferY, playerX, playerY)
	for _, c := range candidates[1:] {
		score := gsTileScore(c[0], c[1], preferX, preferY, playerX, playerY)
		if score < bestScore {
			best, bestScore = c, score
		}
	}
	return best[0], best[1]
}

func gsTileScore(x, y, preferX, preferY, playerX, playerY uint8) int {
	return gsManhattan(x, y, preferX, preferY)*1000 + gsManhattan(x, y, playerX, playerY)
}

func gsManhattan(x0, y0, x1, y1 uint8) int {
	dx := int(x0) - int(x1)
	if dx < 0 {
		dx = -dx
	}
	dy := int(y0) - int(y1)
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

func executeGSRocketPasswords(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b3f:passwords"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketPasswordsLearned, gsprofile.ProgressRocketLanceHealing, encounter)
	if err != nil || done {
		return err
	}
	// From pokegold/maps/TeamRocketBaseB3F.asm: the RaticateTail
	// male grunt at (5,15), then the SlowpokeTail female at (21,7).
	// Winning alone does not prove learning either password: their post-win
	// conversations set two separate permanent events. Those conversations
	// only run on a talk-after (endifjustbattled), so the A press must face
	// the grunt's live tile after any sight-range walk-up.
	for _, script := range []struct {
		label                string
		standX, standY, x, y uint8
		slot                 int
		fact                 game.ProgressID
	}{
		{"raticate-tail", 5, 14, 5, 15, gsRocketRaticateGruntSlot, gsprofile.ProgressRocketRaticatePassword},
		{"slowpoke-tail", 21, 8, 21, 7, gsRocketSlowpokeGruntSlot, gsprofile.ProgressRocketSlowpokePassword},
	} {
		if gsFirstBadgeProgressComplete(profile, m, script.fact) {
			continue
		}
		if err := gsRocketB3FTalk(m, romData, profile,
			script.standX, script.standY, script.x, script.y, script.slot,
			encounter+":"+script.label,
			func() bool { return gsFirstBadgeProgressComplete(profile, m, script.fact) }); err != nil {
			return err
		}
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketPasswordsLearned) {
		return fmt.Errorf("%w: both retail Rocket passwords are required", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

func executeGSRocketOfficeDoor(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b3f:office-door"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketOfficeOpened, gsprofile.ProgressRocketPasswordsLearned, encounter)
	if err != nil || done {
		return err
	}
	// Door event at (10,9): both passwords unlock the B3F floor tile.
	return gsRocketB3FTalk(m, romData, profile, 10, 10, 10, 9, 0, encounter,
		func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketOfficeOpened) })
}

func executeGSRocketExecutiveM4(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b3f:executive"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketExecutiveBeaten, gsprofile.ProgressRocketOfficeOpened, encounter)
	if err != nil || done {
		return err
	}
	b3f, err := gsOpeningMapID("TEAM_ROCKET_BASE_B3F")
	if err != nil {
		return err
	}
	// Stepping on (10,8) triggers RocketBaseBossLeft. The scene owns the
	// encounter and defeat flag (not a synthetic trainer victory).
	if err := gsSecondBadgeGoTo(m, romData, profile,
		skill.ExactNativeDestination(b3f, 10, 8)); err != nil {
		return fmt.Errorf("gen2 Rocket B3F: reach Giovanni office executive: %w", err)
	}
	if !profile.DecodeOverworld(m).Controllable {
		if err := driveGSSecondBadgeInterruption(m, profile, encounter); err != nil {
			return fmt.Errorf("gen2 Rocket B3F: settle executive battle: %w", err)
		}
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketExecutiveBeaten) ||
		!profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Giovanni office executive win event missing", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

func executeGSRocketMurkrow(m *emu.Emu, romData []byte) error {
	const encounter = "rocket-b3f:murkrow"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRocketMurkrowPassword, gsprofile.ProgressRocketExecutiveBeaten, encounter)
	if err != nil || done {
		return err
	}
	return gsRocketB3FTalk(m, romData, profile, 7, 3, 7, 2, gsRocketMurkrowSlot, encounter,
		func() bool { return gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRocketMurkrowPassword) })
}
