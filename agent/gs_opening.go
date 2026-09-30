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
	gsOpeningScriptFrameBudget   uint64 = 120_000
	gsOpeningBattleFrameBudget   uint64 = 180_000
	gsStarterReactionFrameBudget uint64 = 1_200
	gsOpeningMaxBattlePresses           = 900
	gsOpeningRouteAttempts              = 8
	gsErrandRouteAttempts               = 32
	// gsEncounterTransitionFrames bounds a wild encounter's pre-battle script.
	gsEncounterTransitionFrames = 180
	gsErrandMaxNamePresses      = 16

	// gsScriptFrozenPresses is how many consecutive A presses may leave the
	// rendered dialogue page unchanged before a script is called stuck.
	//
	// A total-press cap cannot bound scripted dialogue. The mandatory
	// PlayersHouse1F MeetMom scene spends A presses that never reach the game
	// at all: taps that only accelerate the drawing of a long paragraph, taps
	// that land while an applymovement/showemote owns the machine, and taps
	// into the clock/day-of-week and YES/NO prompts. The scene is finite and
	// finishes, but it legitimately needs more presses than the opening ever
	// budgeted for, so the old 160-press cap aborted a scene that was making
	// progress the whole time (the same failure mode skill.dialoguePagingStuck
	// already fixed for Gen-I dialogue). Frozen text is the stuck signal.
	//
	// MEASURED on run-nk4u8m5acmn5's captured checkpoint, by replaying
	// "take the chikorita starter" through cmd/pokerepro with this value
	// varied: the full opening completes at 24 and still aborts at 20, so the
	// longest legitimate frozen run is 21-23 presses (a scripted walk-in that
	// ignores input while it owns the machine). 48 keeps a full text box of
	// margin over that.
	gsScriptFrozenPresses = 48
)

// gsScriptProgress tracks whether A presses are still moving a script forward.
// It is deliberately measured on the observable dialogue page rather than on a
// press count, so a long text, a forced walk, or a prompt that eats several
// taps does not look like a stall.
type gsScriptProgress struct {
	page      string
	seen      bool
	unchanged int
	presses   int
}

// observe records one observed dialogue page and reports whether the scene has
// stopped responding to A. It must be consulted immediately before the press it
// is judging.
func (p *gsScriptProgress) observe(page string) bool {
	switch {
	case !p.seen:
		p.seen = true
		p.unchanged = 0
	case page == p.page:
		p.unchanged++
	default:
		p.unchanged = 0
	}
	p.page = page
	p.presses++
	return p.unchanged >= gsScriptFrozenPresses
}

// reset forgets the observed page, so the next press is judged against the
// surface the current input produces rather than one a different input replaced
// (an A-tap progress run crossing a START/A naming screen).
func (p *gsScriptProgress) reset() {
	p.page = ""
	p.seen = false
	p.unchanged = 0
	p.presses = 0
}

func (p *gsScriptProgress) frozen(m *emu.Emu, profile *gsprofile.Profile) bool {
	return p.observe(profile.ScreenText(m))
}

type gsStarterSpec struct {
	Starter              skill.Starter
	Species              game.SpeciesID
	BallX, BallY         uint8
	ApproachX, ApproachY uint8
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
	var progress gsScriptProgress
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
			if progress.frozen(m, profile) {
				return fmt.Errorf("%w: dialogue page frozen for %d consecutive A presses (of %d sent) on map %#04x at (%d,%d)",
					errGSOpeningStalled, gsScriptFrozenPresses, progress.presses, facts.NativeMapID, facts.X, facts.Y)
			}
			m.Tap(emu.A, 3, 7)
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
	var progress gsScriptProgress
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
			if progress.frozen(m, profile) {
				return fmt.Errorf("%w: dialogue page frozen for %d consecutive A presses (of %d sent) while selecting %s",
					errGSOpeningStalled, gsScriptFrozenPresses, progress.presses, spec.Species)
			}
			m.Tap(emu.A, 3, 7)
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

func gsErrandScriptMap(mapID uint16) bool {
	for _, name := range []string{"ELMS_LAB", "MR_POKEMONS_HOUSE", "ROUTE_30", "CHERRYGROVE_CITY"} {
		id, err := gsOpeningMapID(name)
		if err == nil && mapID == id {
			return true
		}
	}
	return false
}

// driveGSEarlyBattle routes Gold/Silver encounters through the shared semantic
// battle controller. The GS profile now owns the native 2D battle menu, move
// list, execution phases, party/resource projection and runtime boundary;
// generic code only chooses portable battle actions. Gen-II battle-item use
// remains fail-closed until native item semantics are generation-neutral.
func driveGSEarlyBattle(m *emu.Emu, profile *gsprofile.Profile) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing emulator/profile for early battle", errGSOpeningUnexpectedState)
	}
	if !profile.DecodeOpening(m).InBattle {
		return nil
	}
	if _, err := skill.Battle(m, skill.FirstUsableMove); err != nil {
		facts := profile.DecodeOpening(m)
		return fmt.Errorf("%w: shared Gen-II battle failed on map %#04x at (%d,%d): %v",
			errGSOpeningStalled, facts.NativeMapID, facts.X, facts.Y, err)
	}
	return nil
}

// driveGSErrandScript owns the deterministic scripts in Mr. Pokemon's house,
// Cherrygrove's first rival encounter and Elm's post-theft sequence. The only
// non-A surface is the officer's rival naming screen: START moves the naming
// cursor to END and A accepts the game's version-specific default name.
func driveGSErrandScript(m *emu.Emu, profile *gsprofile.Profile) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing emulator/profile for errand script", errGSOpeningUnexpectedState)
	}
	start := m.FrameCount()
	var progress gsScriptProgress
	namePresses := 0
	for m.FrameCount()-start < gsOpeningScriptFrameBudget {
		facts := profile.DecodeOpening(m)
		if facts.InBattle {
			if err := driveGSEarlyBattle(m, profile); err != nil {
				return err
			}
			continue
		}
		if facts.Controllable {
			return nil
		}
		if !gsErrandScriptMap(facts.NativeMapID) {
			return fmt.Errorf("%w: errand script active on unowned map %#04x at (%d,%d)",
				errGSOpeningUnexpectedState, facts.NativeMapID, facts.X, facts.Y)
		}
		if facts.RivalNamePrompt {
			// Naming-screen input is START (jump the cursor to END) then A
			// (accept the version's default name). It is not dialogue paging,
			// so it keeps its own small, explicit bound.
			if namePresses+2 > gsErrandMaxNamePresses {
				return fmt.Errorf("%w: exceeded %d errand inputs at rival naming screen",
					errGSOpeningStalled, gsErrandMaxNamePresses)
			}
			progress.reset()
			m.Tap(emu.Start, 3, 7)
			m.Tap(emu.A, 3, 7)
			namePresses += 2
			continue
		}
		if facts.ScriptActive && facts.MovementIdle {
			if progress.frozen(m, profile) {
				return fmt.Errorf("%w: dialogue page frozen for %d consecutive A presses (of %d sent) on map %#04x",
					errGSOpeningStalled, gsScriptFrozenPresses, progress.presses, facts.NativeMapID)
			}
			m.Tap(emu.A, 3, 7)
			continue
		}
		m.StepFrame()
	}
	facts := profile.DecodeOpening(m)
	return fmt.Errorf("%w: errand script exceeded %d frames on map %#04x at (%d,%d)",
		errGSOpeningStalled, gsOpeningScriptFrameBudget, facts.NativeMapID, facts.X, facts.Y)
}

func gsErrandGoTo(m *emu.Emu, romData []byte, profile *gsprofile.Profile, dest skill.NativeDestination) error {
	for attempt := 0; attempt < gsErrandRouteAttempts; attempt++ {
		err := skill.GoToNative(m, romData, dest)
		if err == nil {
			return nil
		}
		switch {
		case errors.Is(err, skill.ErrBattle):
			if battleErr := driveGSEarlyBattle(m, profile); battleErr != nil {
				return fmt.Errorf("gen2 opening errand: settle route battle: %w", battleErr)
			}
		case errors.Is(err, skill.ErrDialogueInterrupted):
			facts := profile.DecodeOpening(m)
			if !gsErrandScriptMap(facts.NativeMapID) {
				// A wild encounter runs a script with idle movement and a blank
				// screen for ~30 frames before BattleMode is set, which the
				// overworld decoder reads as a dialogue. Let that transition
				// resolve; anything still scripted afterwards really is unowned.
				if _, waitErr := m.StepUntil(gsEncounterTransitionFrames, func(m *emu.Emu) bool {
					f := profile.DecodeOpening(m)
					return f.InBattle || f.Controllable
				}); waitErr == nil {
					continue
				}
				return fmt.Errorf("%w: route interrupted by unowned script on map %#04x at (%d,%d)",
					errGSOpeningUnexpectedState, facts.NativeMapID, facts.X, facts.Y)
			}
			if scriptErr := driveGSErrandScript(m, profile); scriptErr != nil {
				return fmt.Errorf("gen2 opening errand: settle route script: %w", scriptErr)
			}
		default:
			return err
		}
	}
	return fmt.Errorf("%w: errand route did not settle after %d interruptions", errGSOpeningStalled, gsErrandRouteAttempts)
}

func settleCompletedGSErrand(m *emu.Emu, profile *gsprofile.Profile) error {
	facts := profile.DecodeOpening(m)
	if !facts.GaveMysteryEggToElm {
		return fmt.Errorf("%w: Mystery Egg return flag is not set", errGSOpeningUnexpectedState)
	}
	if facts.Controllable {
		return nil
	}
	if !gsErrandScriptMap(facts.NativeMapID) {
		return fmt.Errorf("%w: completed errand is unstable on map %#04x", errGSOpeningUnexpectedState, facts.NativeMapID)
	}
	if err := driveGSErrandScript(m, profile); err != nil {
		return err
	}
	facts = profile.DecodeOpening(m)
	if !facts.GaveMysteryEggToElm || !facts.Controllable {
		return fmt.Errorf("%w: Mystery Egg return did not settle; flag=%v controllable=%v",
			errGSOpeningUnexpectedState, facts.GaveMysteryEggToElm, facts.Controllable)
	}
	return nil
}

// executeGSPostStarterErrand resumes the mandatory Elm -> Mr. Pokemon -> Oak
// -> Cherrygrove rival -> officer -> Elm loop from any durable intermediate
// state. Completion is the cartridge's EVENT_GAVE_MYSTERY_EGG_TO_ELM bit.
func executeGSPostStarterErrand(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 opening errand: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	facts := profile.DecodeOpening(m)
	if facts.GaveMysteryEggToElm {
		return settleCompletedGSErrand(m, profile)
	}
	if !facts.GotStarter || len(facts.Party) == 0 {
		return fmt.Errorf("%w: post-starter errand requires Elm's starter handoff; party=%v event=%v",
			errGSOpeningUnexpectedState, facts.Party, facts.GotStarter)
	}

	mrPokemon, err := gsOpeningMapID("MR_POKEMONS_HOUSE")
	if err != nil {
		return err
	}
	lab, err := gsOpeningMapID("ELMS_LAB")
	if err != nil {
		return err
	}

	if !facts.MrPokemonVisitComplete {
		if err := gsErrandGoTo(m, romData, profile, skill.NativeMapDestination(mrPokemon)); err != nil {
			return fmt.Errorf("gen2 opening errand: reach Mr. Pokemon: %w", err)
		}
		facts = profile.DecodeOpening(m)
		if !facts.Controllable {
			if err := driveGSErrandScript(m, profile); err != nil {
				return fmt.Errorf("gen2 opening errand: finish Mr. Pokemon visit: %w", err)
			}
			facts = profile.DecodeOpening(m)
		}
		if !facts.MrPokemonVisitComplete || !facts.GotMysteryEgg || !facts.HasPokedex {
			return fmt.Errorf("%w: Mr. Pokemon visit ended without egg/dex completion; egg=%v dex=%v scene=%d",
				errGSOpeningUnexpectedState, facts.GotMysteryEgg, facts.HasPokedex, facts.MrPokemonsHouseScene)
		}
	}

	// Returning through Cherrygrove owns the mandatory can-lose rival battle.
	// Routing onward to the lab naturally crosses its coordinate trigger.
	if !facts.CherrygroveRivalResolved {
		if err := gsErrandGoTo(m, romData, profile, skill.NativeMapDestination(lab)); err != nil {
			return fmt.Errorf("gen2 opening errand: return through Cherrygrove: %w", err)
		}
		facts = profile.DecodeOpening(m)
		if !facts.CherrygroveRivalResolved {
			return fmt.Errorf("%w: reached Elm's Lab without resolving Cherrygrove rival; scene=%d",
				errGSOpeningUnexpectedState, facts.CherrygroveCityScene)
		}
	}

	// The officer trigger is at lab y=5; Elm stands at (5,2). Routing to (5,3)
	// deliberately crosses the trigger, then resumes after the officer leaves.
	if !facts.RivalNamed {
		if err := gsErrandGoTo(m, romData, profile, skill.ExactNativeDestination(lab, 5, 3)); err != nil {
			return fmt.Errorf("gen2 opening errand: resolve officer and rival name: %w", err)
		}
		facts = profile.DecodeOpening(m)
		if !facts.RivalNamed {
			return fmt.Errorf("%w: officer sequence ended without durable rival-name boundary; lab scene=%d rival=%q",
				errGSOpeningUnexpectedState, facts.ElmsLabScene, facts.RivalName)
		}
	}

	if err := gsErrandGoTo(m, romData, profile, skill.ExactNativeDestination(lab, 5, 3)); err != nil {
		return fmt.Errorf("gen2 opening errand: approach Elm: %w", err)
	}
	if err := skill.Face(m, 5, 2); err != nil {
		return fmt.Errorf("gen2 opening errand: face Elm: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	if err := driveGSErrandScript(m, profile); err != nil {
		return fmt.Errorf("gen2 opening errand: hand Mystery Egg to Elm: %w", err)
	}
	return settleCompletedGSErrand(m, profile)
}
