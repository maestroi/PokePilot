package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/profiles"
	"github.com/maestroi/pokepilot/skill"
)

var (
	errGSOpeningStalled         = errors.New("gen2 opening made no progress")
	errGSOpeningUnexpectedState = errors.New("gen2 opening state has no owning phase")
)

const (
	gsOpeningScriptFrameBudget  uint64 = 120_000
	gsStarterReactionFrameBudget uint64 = 1_200
	gsOpeningMaxScriptPresses          = 160
	gsOpeningRouteAttempts             = 8
)

type gsStarterSpec struct {
	Starter                  skill.Starter
	Species                  game.SpeciesID
	BallX, BallY             uint8
	ApproachX, ApproachY     uint8
}

func gsStarterSpecFor(starter skill.Starter) (gsStarterSpec, bool) {
	switch starter {
	case skill.StarterChikorita:
		return gsStarterSpec{
			Starter: starter, Species: "chikorita",
			BallX: 8, BallY: 3, ApproachX: 8, ApproachY: 4,
		}, true
	case skill.StarterCyndaquil:
		return gsStarterSpec{
			Starter: starter, Species: "cyndaquil",
			BallX: 6, BallY: 3, ApproachX: 6, ApproachY: 4,
		}, true
	case skill.StarterTotodile:
		return gsStarterSpec{
			Starter: starter, Species: "totodile",
			BallX: 7, BallY: 3, ApproachX: 7, ApproachY: 4,
		}, true
	default:
		return gsStarterSpec{}, false
	}
}

func gsOpeningMapID(name string) (uint16, error) {
	info, ok := gsdata.MapByName(name)
	if !ok {
		return 0, fmt.Errorf("%w: generated map %q is unavailable", errGSOpeningUnexpectedState, name)
	}
	return gsdata.NativeMapID(info.Group, info.Number), nil
}

func gsOpeningProfile(romData []byte) (*gsprofile.Profile, error) {
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		return nil, fmt.Errorf("gen2 opening: detect profile: %w", err)
	}
	gs, ok := profile.(*gsprofile.Profile)
	if !ok {
		return nil, fmt.Errorf("%w: profile %s@%s is not Gold/Silver", errGSOpeningUnexpectedState, profile.ID(), profile.Revision())
	}
	return gs, nil
}

func gsOpeningScriptMap(mapID uint16) bool {
	house, houseErr := gsOpeningMapID("PLAYERS_HOUSE_1F")
	lab, labErr := gsOpeningMapID("ELMS_LAB")
	return (houseErr == nil && mapID == house) || (labErr == nil && mapID == lab)
}

// driveGSOpeningScript owns only the mandatory fresh-game setup scripts in
// Player's House 1F and Elm's Lab. Those scripts contain known setup/YES-NO
// prompts; selecting their default affirmative entries is part of this opening
// transaction. No generic Gen-II dialogue/menu recovery calls this helper.
func driveGSOpeningScript(m *emu.Emu, profile *gsprofile.Profile) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing emulator/profile", errGSOpeningUnexpectedState)
	}
	start := m.FrameCount()
	presses := 0
	for m.FrameCount()-start < gsOpeningScriptFrameBudget {
		facts := profile.DecodeOpening(m)
		if facts.InBattle {
			return fmt.Errorf("%w: battle began on map %#04x at (%d,%d)", errGSOpeningUnexpectedState, facts.NativeMapID, facts.X, facts.Y)
		}
		if facts.Controllable {
			return nil
		}
		if !gsOpeningScriptMap(facts.NativeMapID) {
			return fmt.Errorf("%w: opening script active on map %#04x at (%d,%d)", errGSOpeningUnexpectedState, facts.NativeMapID, facts.X, facts.Y)
		}

		// Forced walks and transition animations own the machine; do not send
		// input into them. Script-owned idle states are the text/prompt surfaces
		// verified in PlayersHouse1F.asm and ElmsLab.asm.
		if facts.ScriptActive && facts.MovementIdle {
			if presses >= gsOpeningMaxScriptPresses {
				return fmt.Errorf("%w: exceeded %d owned A presses on map %#04x", errGSOpeningStalled, gsOpeningMaxScriptPresses, facts.NativeMapID)
			}
			m.Tap(emu.A, 3, 7)
			presses++
			continue
		}
		m.StepFrame()
	}
	facts := profile.DecodeOpening(m)
	return fmt.Errorf("%w: opening script exceeded %d frames on map %#04x at (%d,%d)",
		errGSOpeningStalled, gsOpeningScriptFrameBudget, facts.NativeMapID, facts.X, facts.Y)
}

func gsOpeningReachElmLab(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	lab, err := gsOpeningMapID("ELMS_LAB")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < gsOpeningRouteAttempts; attempt++ {
		err := skill.GoToNative(m, romData, skill.NativeMapDestination(lab))
		if err == nil {
			return nil
		}
		if !errors.Is(err, skill.ErrDialogueInterrupted) {
			return fmt.Errorf("gen2 opening: reach Elm's Lab: %w", err)
		}
		facts := profile.DecodeOpening(m)
		if !gsOpeningScriptMap(facts.NativeMapID) {
			return fmt.Errorf("%w: route interrupted by unowned script on map %#04x at (%d,%d)",
				errGSOpeningUnexpectedState, facts.NativeMapID, facts.X, facts.Y)
		}
		if err := driveGSOpeningScript(m, profile); err != nil {
			return fmt.Errorf("gen2 opening: settle mandatory script: %w", err)
		}
	}
	return fmt.Errorf("%w: Elm's Lab route did not settle after %d script handoffs", errGSOpeningStalled, gsOpeningRouteAttempts)
}

func driveGSStarterSelection(m *emu.Emu, profile *gsprofile.Profile, spec gsStarterSpec) error {
	start := m.FrameCount()
	started := false
	presses := 0
	for m.FrameCount()-start < gsOpeningScriptFrameBudget {
		facts := profile.DecodeOpening(m)
		if facts.HasSpecies(spec.Species) && facts.Controllable {
			return nil
		}
		if facts.InBattle {
			return fmt.Errorf("%w: starter selection entered battle", errGSOpeningUnexpectedState)
		}
		if len(facts.Party) > 0 && !facts.HasSpecies(spec.Species) {
			return fmt.Errorf("%w: party contains %v while selecting %s", errGSOpeningUnexpectedState, facts.Party, spec.Species)
		}

		if !facts.Controllable || facts.ScriptActive {
			started = true
		}
		if started && facts.Controllable && !facts.HasSpecies(spec.Species) {
			return fmt.Errorf("%w: Elm starter script ended without %s", errGSOpeningUnexpectedState, spec.Species)
		}
		if !started && m.FrameCount()-start >= gsStarterReactionFrameBudget {
			return fmt.Errorf("%w: starter ball at (%d,%d) did not start a script", errGSOpeningStalled, spec.BallX, spec.BallY)
		}

		if facts.ScriptActive && facts.MovementIdle {
			if presses >= gsOpeningMaxScriptPresses {
				return fmt.Errorf("%w: exceeded %d owned A presses while selecting %s",
					errGSOpeningStalled, gsOpeningMaxScriptPresses, spec.Species)
			}
			m.Tap(emu.A, 3, 7)
			presses++
			continue
		}
		m.StepFrame()
	}
	facts := profile.DecodeOpening(m)
	return fmt.Errorf("%w: starter selection exceeded %d frames; map=%#04x at (%d,%d) party=%v",
		errGSOpeningStalled, gsOpeningScriptFrameBudget, facts.NativeMapID, facts.X, facts.Y, facts.Party)
}

// executeGSOpening is resumable from any stable point between the booted
// bedroom, Mom's mandatory setup, Elm's intro, and the selected starter
// handoff. It stops after the chosen level-5 starter has been received and the
// lab has returned to controllable overworld state.
func executeGSOpening(m *emu.Emu, romData []byte, starter skill.Starter) error {
	if m == nil {
		return fmt.Errorf("gen2 opening: nil emulator")
	}
	spec, ok := gsStarterSpecFor(starter)
	if !ok {
		return fmt.Errorf("%w: unsupported starter %d", errGSOpeningUnexpectedState, starter)
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}

	facts := profile.DecodeOpening(m)
	if facts.HasSpecies(spec.Species) {
		if facts.Controllable {
			return nil
		}
		if gsOpeningScriptMap(facts.NativeMapID) {
			if err := driveGSOpeningScript(m, profile); err != nil {
				return fmt.Errorf("gen2 opening: settle received starter: %w", err)
			}
			facts = profile.DecodeOpening(m)
			if facts.HasSpecies(spec.Species) && facts.Controllable {
				return nil
			}
		}
	}
	if len(facts.Party) > 0 {
		return fmt.Errorf("%w: cannot select %s with existing party %v", errGSOpeningUnexpectedState, spec.Species, facts.Party)
	}

	if err := gsOpeningReachElmLab(m, romData, profile); err != nil {
		return err
	}
	facts = profile.DecodeOpening(m)
	lab, err := gsOpeningMapID("ELMS_LAB")
	if err != nil {
		return err
	}
	if facts.NativeMapID != lab || !facts.Controllable {
		return fmt.Errorf("%w: Elm route ended on map %#04x at (%d,%d), controllable=%v",
			errGSOpeningUnexpectedState, facts.NativeMapID, facts.X, facts.Y, facts.Controllable)
	}

	if err := skill.GoToNative(m, romData, skill.ExactNativeDestination(lab, spec.ApproachX, spec.ApproachY)); err != nil {
		return fmt.Errorf("gen2 opening: reach %s ball approach: %w", spec.Species, err)
	}
	if err := skill.Face(m, spec.BallX, spec.BallY); err != nil {
		return fmt.Errorf("gen2 opening: face %s ball: %w", spec.Species, err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSStarterSelection(m, profile, spec); err != nil {
		return fmt.Errorf("gen2 opening: choose %s: %w", spec.Species, err)
	}

	final := profile.DecodeOpening(m)
	if !final.HasSpecies(spec.Species) || !final.Controllable {
		return fmt.Errorf("%w: %s selection returned without stable starter postcondition; party=%v controllable=%v",
			errGSOpeningUnexpectedState, spec.Species, final.Party, final.Controllable)
	}
	return nil
}
