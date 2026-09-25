package story

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	"github.com/maestroi/pokepilot/yellow/sym"
)

// Opening geometry, from pokeyellow/data/maps/objects/OaksLab.asm and
// pokeyellow/scripts/PalletTown.asm / OaksLab.asm.
const (
	// PalletTownDefaultScript fires when the player reaches wYCoord == 0.
	// (10,1) is the north-exit approach tile measured on Red's identical
	// Pallet Town blocks; one step up from it trips the gate.
	gateApproachX, gateApproachY = 10, 1
	// The lone Eevee Poké Ball sits at (7,3). Standing below it at (7,4) is
	// the case OaksLabRivalTakesPokeballScript scripts explicitly (the rival
	// pushes the player aside only when wYCoord == 4).
	eeveeBallX, eeveeBallY = 7, 3
	ballApproachX          = 7
	ballApproachY          = 4
	// OaksLabRivalChallengesPlayerScript fires on wYCoord == 6 once Pikachu
	// is in the party. (5,6) is the lab's central walkable tile on that row.
	rivalRowX, rivalRowY = 5, 6
)

// Frame budgets. They bound loops, not expected durations: exhausting one is
// a controller failure, never slowness.
const (
	advanceScriptBudget = 30000
	ballReactionBudget  = 600
)

// ErrOpeningChoicePrompt is returned when a two-option prompt opens while the
// controller is only advancing scripted text. Yellow's opening has no choice
// in it (Pikachu is given without a nickname prompt), so a prompt means the
// state is not what the controller owns; it never answers one blindly.
var ErrOpeningChoicePrompt = errors.New("yellow story: unexpected two-option prompt during the opening")

type emuOpeningDriver struct {
	m       *emu.Emu
	romData []byte
	policy  skill.MovePolicy
}

// NewOpeningDriver returns the emulator-backed OpeningDriver. Movement and
// battle reuse the shared Gen-I skills (Yellow binds the canonical memory
// view and its own ROM tables); every story decision reads Yellow's native
// flags through yellowprofile.DecodeOpening.
func NewOpeningDriver(m *emu.Emu, romData []byte, policy skill.MovePolicy) OpeningDriver {
	return &emuOpeningDriver{m: m, romData: romData, policy: policy}
}

// Opening runs Yellow's opening on m until target is reached.
func Opening(m *emu.Emu, romData []byte, policy skill.MovePolicy, target OpeningMilestone) error {
	if m == nil {
		return fmt.Errorf("yellow story: nil emulator")
	}
	if policy == nil {
		return fmt.Errorf("yellow story: nil move policy")
	}
	return RunOpening(NewOpeningDriver(m, romData, policy), target)
}

func (d *emuOpeningDriver) Facts() yellowprofile.OpeningFacts {
	return yellowprofile.DecodeOpening(d.m)
}

func (d *emuOpeningDriver) WalkToGate() error {
	if d.Facts().Map != yellowprofile.PalletTownMap {
		if err := skill.GoTo(d.m, d.romData, skill.MapDestination(yellowprofile.PalletTownMap)); err != nil {
			return fmt.Errorf("reach Pallet Town: %w", err)
		}
	}
	if err := skill.GoTo(d.m, d.romData, skill.ExactDestination(yellowprofile.PalletTownMap, gateApproachX, gateApproachY)); err != nil {
		return fmt.Errorf("reach the north exit approach: %w", err)
	}
	// The gate freezes input the moment the player lands on row 0, so an
	// interrupted step is the expected trigger; RunOpening judges it by
	// whether EVENT_OAK_APPEARED_IN_PALLET became true.
	return skill.StepOnce(d.m, world.StepUp)
}

func (d *emuOpeningDriver) AdvanceScript() error {
	var mem state.Mem
	for spent := 0; spent < advanceScriptBudget; {
		phase, err := PhaseFor(d.Facts())
		if err != nil || phase != OpeningAdvanceScript {
			return nil
		}
		state.Snapshot(d.m, &mem)
		if state.DecodeTwoOptionMenu(&mem) != nil {
			return ErrOpeningChoicePrompt
		}
		if d.m.Peek8Native(sym.FontLoaded) != 0 {
			d.m.Tap(emu.A, 3, 7)
			spent += 10
			continue
		}
		d.m.StepFrame()
		spent++
	}
	f := d.Facts()
	return fmt.Errorf("advance scripted opening: %w: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
		skill.ErrCutsceneTimeout, f.Map, f.X, f.Y, d.m.Peek8Native(sym.JoyIgnore), d.m.Peek8Native(sym.FontLoaded))
}

func (d *emuOpeningDriver) TakeBall() error {
	if err := skill.GoTo(d.m, d.romData, skill.ExactDestination(yellowprofile.OaksLabMap, ballApproachX, ballApproachY)); err != nil {
		return fmt.Errorf("reach the Poké Ball: %w", err)
	}
	if err := skill.Face(d.m, eeveeBallX, eeveeBallY); err != nil {
		return fmt.Errorf("face the Poké Ball: %w", err)
	}
	d.m.Tap(emu.A, 3, 7)
	// Pressing A on the ball hands control to the rival's script. Wait for
	// that positive sign so RunOpening sees the state move.
	for i := 0; i < ballReactionBudget; i++ {
		f := d.Facts()
		if !f.Controllable || f.GotStarter {
			return nil
		}
		d.m.StepFrame()
	}
	f := d.Facts()
	return fmt.Errorf("the rival's script did not start within %d frames of pressing A on the ball at (%d,%d): player at (%d,%d)",
		ballReactionBudget, eeveeBallX, eeveeBallY, f.X, f.Y)
}

func (d *emuOpeningDriver) WalkToRival() error {
	err := skill.GoTo(d.m, d.romData, skill.ExactDestination(yellowprofile.OaksLabMap, rivalRowX, rivalRowY))
	if err != nil && errors.Is(err, skill.ErrDialogueInterrupted) {
		// The challenge opens the moment the player reaches row 6.
		return nil
	}
	return err
}

func (d *emuOpeningDriver) FightRival() error {
	// The lab battle is not required to be won: the end-battle script heals
	// the party, records the rival's Eevee branch and sets
	// EVENT_BATTLED_RIVAL_IN_OAKS_LAB either way.
	_, err := skill.Battle(d.m, d.policy)
	return err
}
