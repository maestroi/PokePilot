package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

type gsIlexBirdStep struct {
	position       int
	birdX, birdY   uint8
	standX, standY uint8
}

const (
	gsIlexHeadbuttTutorX uint8 = 15
	gsIlexHeadbuttTutorY uint8 = 14
	gsIlexHeadbuttStandX uint8 = 15
	gsIlexHeadbuttStandY uint8 = 15
)

var gsIlexBirdSteps = map[int]gsIlexBirdStep{
	// Each staging tile must leave the player facing the bird from a side whose
	// branch in maps/IlexForest.asm sends the bird forward. Facing from a bounce
	// side only moves the bird back to a position already solved, and the puzzle
	// then ping-pongs between the two until the interaction budget is spent.
	//
	// Position 1 has two legal branches. Approaching from below faces UP and
	// skips directly to position 3, which is the shorter retail solution.
	1: {position: 1, birdX: 14, birdY: 31, standX: 14, standY: 32},
	3: {position: 3, birdX: 20, birdY: 24, standX: 20, standY: 23},
	// FarfetchdPosition4 bounces on UP, and (29,22)'s only other walkable
	// neighbour is (28,22): standing below the bird sent it back to position 3
	// forever, so stage beside it and face RIGHT instead.
	4:  {position: 4, birdX: 29, birdY: 22, standX: 28, standY: 22},
	5:  {position: 5, birdX: 28, birdY: 31, standX: 28, standY: 30},
	6:  {position: 6, birdX: 24, birdY: 35, standX: 25, standY: 35},
	7:  {position: 7, birdX: 22, birdY: 31, standX: 22, standY: 32},
	8:  {position: 8, birdX: 15, birdY: 29, standX: 15, standY: 28},
	9:  {position: 9, birdX: 10, birdY: 35, standX: 11, standY: 35},
	10: {position: 10, birdX: 6, birdY: 28, standX: 6, standY: 29},
	// A resumed save may already be at position 2. Facing UP from below takes
	// the normal forward branch to position 3.
	2: {position: 2, birdX: 15, birdY: 25, standX: 15, standY: 26},
}

func executeGSAzaleaRival(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Azalea rival: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressAzaleaRivalResolved) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "azalea:rival")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHiveBadgeEarned) {
		return fmt.Errorf("%w: Azalea rival requires the Hive Badge", errGSSecondBadgeUnexpectedState)
	}

	azalea, err := gsOpeningMapID("AZALEA_TOWN")
	if err != nil {
		return err
	}
	// Slowpoke Well arms scene 1. Entering (5,10) owns the scripted rival
	// approach. The event bit is set before startbattle, so success is verified
	// only by the profile's post-win NOOP scene boundary.
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(azalea, 5, 10)); err != nil {
		return fmt.Errorf("gen2 Azalea rival: reach trigger: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressAzaleaRivalResolved) {
		return fmt.Errorf("%w: rival trigger settled without the post-win Azalea scene", errGSSecondBadgeUnexpectedState)
	}
	return nil
}

func executeGSFarfetchd(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 Farfetchd: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressFarfetchdHerded) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "ilex:farfetchd")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressAzaleaRivalResolved) {
		return fmt.Errorf("%w: Farfetchd puzzle requires the Azalea rival victory", errGSSecondBadgeUnexpectedState)
	}
	forest, err := gsOpeningMapID("ILEX_FOREST")
	if err != nil {
		return err
	}

	for kicks := 0; kicks < 12; kicks++ {
		state := profile.DecodeIlexState(m)
		if state.Herded {
			return nil
		}
		step, ok := gsIlexBirdSteps[state.Position]
		if !ok {
			return fmt.Errorf("%w: Ilex Farfetchd has no active semantic position (decoded %d)", errGSSecondBadgeUnexpectedState, state.Position)
		}
		if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(forest, step.standX, step.standY)); err != nil {
			return fmt.Errorf("gen2 Farfetchd: reach position %d: %w", state.Position, err)
		}
		if err := skill.Face(m, step.birdX, step.birdY); err != nil {
			return fmt.Errorf("gen2 Farfetchd: face position %d: %w", state.Position, err)
		}
		before := state.Position
		m.Tap(emu.A, 3, 7)
		if err := driveGSSecondBadgeInterruption(m, profile, fmt.Sprintf("ilex:farfetchd:%d", before)); err != nil {
			return fmt.Errorf("gen2 Farfetchd: position %d script: %w", before, err)
		}
		after := profile.DecodeIlexState(m)
		if after.Herded {
			return nil
		}
		if after.Position == 0 || after.Position == before {
			return fmt.Errorf("%w: Ilex Farfetchd interaction at position %d made no durable progress", errGSSecondBadgeStalled, before)
		}
	}
	return fmt.Errorf("%w: Farfetchd puzzle exceeded 12 interactions", errGSSecondBadgeStalled)
}

func executeGSHM01Cut(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 HM01 Cut: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHM01CutAcquired) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "ilex:hm01-cut")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressFarfetchdHerded) {
		return fmt.Errorf("%w: HM01 Cut requires the Farfetchd to be returned", errGSSecondBadgeUnexpectedState)
	}

	forest, err := gsOpeningMapID("ILEX_FOREST")
	if err != nil {
		return err
	}
	// After position 10 the Charcoal Master appears at (5,28). Talk from below
	// so the returned Farfetch'd at (6,28) cannot occupy our staging tile.
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(forest, 5, 29)); err != nil {
		return fmt.Errorf("gen2 HM01 Cut: reach Charcoal Master: %w", err)
	}
	if err := skill.Face(m, 5, 28); err != nil {
		return fmt.Errorf("gen2 HM01 Cut: face Charcoal Master: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "ilex:hm01-cut"); err != nil {
		return fmt.Errorf("gen2 HM01 Cut: reward script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHM01CutAcquired) {
		return fmt.Errorf("%w: Charcoal Master returned control without HM01 Cut", errGSSecondBadgeUnexpectedState)
	}
	return nil
}


// executeGSTM02Headbutt is the first progression boundary that proves HM01 is
// not merely owned but executable. The Headbutt tutor stands north of Ilex
// Forest's mandatory Cut tree, so reaching his adjacent tile exercises native
// Cut teaching/execution and live-block re-planning before the reward script.
func executeGSTM02Headbutt(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 TM02 Headbutt: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressTM02HeadbuttAcquired) {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		return driveGSSecondBadgeInterruption(m, profile, "ilex:tm02-headbutt")
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressHM01CutAcquired) {
		return fmt.Errorf("%w: TM02 Headbutt requires HM01 Cut", errGSSecondBadgeUnexpectedState)
	}

	if _, err := skill.EnsureFieldMove(m, skill.FieldCut); err != nil {
		return fmt.Errorf("gen2 TM02 Headbutt: prepare Cut: %w", err)
	}
	forest, err := gsOpeningMapID("ILEX_FOREST")
	if err != nil {
		return err
	}

	// The tutor is fixed at (15,14). His south neighbour is on the north side
	// of the mandatory Cut tree; GoToNative can only reach it after executing
	// Cut and rebuilding the live forest grid.
	if err := gsSecondBadgeGoTo(m, romData, profile, skill.ExactNativeDestination(forest, gsIlexHeadbuttStandX, gsIlexHeadbuttStandY)); err != nil {
		return fmt.Errorf("gen2 TM02 Headbutt: cross Ilex Cut tree: %w", err)
	}
	if err := skill.Face(m, gsIlexHeadbuttTutorX, gsIlexHeadbuttTutorY); err != nil {
		return fmt.Errorf("gen2 TM02 Headbutt: face tutor: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSSecondBadgeInterruption(m, profile, "ilex:tm02-headbutt"); err != nil {
		return fmt.Errorf("gen2 TM02 Headbutt: reward script: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressTM02HeadbuttAcquired) {
		return fmt.Errorf("%w: Headbutt tutor returned control without TM02", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
