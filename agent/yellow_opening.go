package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

var (
	errYellowOpeningStalled         = errors.New("yellow opening made no progress")
	errYellowOpeningUnexpectedState = errors.New("yellow opening state has no owning phase")
	errYellowOpeningChoiceRequired  = errors.New("yellow opening exposed an unexpected choice prompt")
)

type yellowOpeningPhase string

const (
	yellowOpeningWalkToGate  yellowOpeningPhase = "walk_to_gate"
	yellowOpeningScript      yellowOpeningPhase = "advance_script"
	yellowOpeningTakeBall    yellowOpeningPhase = "take_ball"
	yellowOpeningWalkToRival yellowOpeningPhase = "walk_to_rival"
	yellowOpeningFightRival  yellowOpeningPhase = "fight_rival"
	yellowOpeningDone        yellowOpeningPhase = "done"
)

const (
	yellowOpeningMessageBoxID       uint8 = 0x01
	yellowOpeningBattleMenuID       uint8 = 0x0b
	yellowOpeningListMenuBoxID      uint8 = 0x0d
	yellowOpeningMaxSteps                 = 40
	yellowOpeningScriptBudget             = 30000
	yellowOpeningBallReactionBudget       = 600
	yellowOpeningRivalTriggerBudget       = 180
	yellowOpeningGateApproachX      uint8 = 10
	yellowOpeningGateApproachY      uint8 = 1
	yellowOpeningBallX              uint8 = 7
	yellowOpeningBallY              uint8 = 3
	yellowOpeningBallStandX         uint8 = 7
	yellowOpeningBallStandY         uint8 = 4
	yellowOpeningRivalX             uint8 = 5
	yellowOpeningRivalY             uint8 = 6
)

func yellowOpeningReached(f yellowprofile.OpeningFacts) bool {
	return f.GotStarter && f.PartyCount > 0 && f.BattledRival && f.Controllable && !f.InBattle
}

// yellowOpeningPhaseFor mirrors Yellow's native opening scripts. In
// particular, OakAskedToChoose prevents the controller from treating a brief
// controllable frame in the lab as permission to touch the Eevee ball before
// Oak's speech has actually completed.
func yellowOpeningPhaseFor(f yellowprofile.OpeningFacts) (yellowOpeningPhase, error) {
	switch {
	case yellowOpeningReached(f):
		return yellowOpeningDone, nil
	case f.InBattle:
		// Gen I's 2x2 battle menu is cursor-shaped like a two-option prompt:
		// wMaxMenuItem is 1 for each column and the live cursor is the same
		// glyph. Battle ownership is therefore stronger evidence than the
		// generic prompt decoder during Yellow's opening.
		if f.GotStarter && !f.BattledRival {
			return yellowOpeningFightRival, nil
		}
		return yellowOpeningScript, nil
	case f.ChoicePrompt && f.Map == yellowprofile.OaksLabMap && f.OakAskedToChoose && !f.GotStarter:
		// Yellow's rival snatches the Eevee and Oak grants Pikachu without a
		// player choice. During that ROM-owned handoff the shared Gen-I prompt
		// shape decoder can transiently see a choice-shaped cursor. Keep this
		// narrow story window script-owned until EVENT_GOT_STARTER commits.
		return yellowOpeningScript, nil
	case f.ChoicePrompt:
		return "", errYellowOpeningChoiceRequired
	case !f.Controllable:
		return yellowOpeningScript, nil
	case !f.OakAppeared && !f.FollowedOak:
		return yellowOpeningWalkToGate, nil
	case !f.OakAskedToChoose:
		return yellowOpeningScript, nil
	case !f.GotStarter:
		if f.Map != yellowprofile.OaksLabMap {
			return "", fmt.Errorf("%w: Oak asked to choose on map %#02x at (%d,%d)",
				errYellowOpeningUnexpectedState, f.Map, f.X, f.Y)
		}
		return yellowOpeningTakeBall, nil
	case !f.BattledRival:
		if f.Map != yellowprofile.OaksLabMap {
			return "", fmt.Errorf("%w: Pikachu received outside Oak's Lab on map %#02x at (%d,%d)",
				errYellowOpeningUnexpectedState, f.Map, f.X, f.Y)
		}
		return yellowOpeningWalkToRival, nil
	default:
		return yellowOpeningScript, nil
	}
}

// executeYellowOpening is a resumable semantic state machine. Yellow owns the
// facts and trigger geometry; ordinary travel, facing, stepping and battle
// execution remain shared Gen-I mechanics.
func executeYellowOpening(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow opening: nil emulator")
	}
	policy := skill.StatAwareMove(romData)
	facts := yellowprofile.DecodeOpening(m)
	stalls := 0

	for step := 0; step < yellowOpeningMaxSteps; step++ {
		if yellowOpeningReached(facts) {
			return nil
		}
		phase, err := yellowOpeningPhaseFor(facts)
		if err != nil {
			return fmt.Errorf("yellow opening: %w", err)
		}
		if phase == yellowOpeningDone {
			return nil
		}

		actionErr := runYellowOpeningPhase(m, romData, policy, phase)
		next := yellowprofile.DecodeOpening(m)
		if next == facts {
			if actionErr != nil {
				return fmt.Errorf("yellow opening: %s: %w", phase, actionErr)
			}
			stalls++
			if stalls >= 2 {
				return fmt.Errorf("%w: phase=%s map=%#02x at (%d,%d)",
					errYellowOpeningStalled, phase, facts.Map, facts.X, facts.Y)
			}
			continue
		}

		// A scripted trigger often reports an interruption exactly when it
		// succeeds (Oak's gate, rival challenge). Progress in Yellow's own
		// facts wins over that transport-level error.
		stalls = 0
		facts = next
	}

	return fmt.Errorf("%w: lab rival not resolved within %d semantic steps; map=%#02x at (%d,%d)",
		errYellowOpeningStalled, yellowOpeningMaxSteps, facts.Map, facts.X, facts.Y)
}

func runYellowOpeningPhase(m *emu.Emu, romData []byte, policy skill.MovePolicy, phase yellowOpeningPhase) error {
	switch phase {
	case yellowOpeningWalkToGate:
		return yellowOpeningWalkGate(m, romData)
	case yellowOpeningScript:
		return yellowOpeningAdvanceScript(m)
	case yellowOpeningTakeBall:
		return yellowOpeningTakeEeveeBall(m, romData)
	case yellowOpeningWalkToRival:
		return yellowOpeningTriggerRival(m, romData)
	case yellowOpeningFightRival:
		_, err := skill.Battle(m, policy)
		return err
	case yellowOpeningDone:
		return nil
	default:
		return fmt.Errorf("%w: unknown phase %q", errYellowOpeningUnexpectedState, phase)
	}
}

func yellowOpeningWalkGate(m *emu.Emu, romData []byte) error {
	if yellowprofile.DecodeOpening(m).Map != yellowprofile.PalletTownMap {
		if err := skill.GoTo(m, romData, skill.MapDestination(yellowprofile.PalletTownMap)); err != nil {
			return fmt.Errorf("reach Pallet Town: %w", err)
		}
	}
	if err := skill.GoTo(m, romData, skill.ExactDestination(
		yellowprofile.PalletTownMap, yellowOpeningGateApproachX, yellowOpeningGateApproachY)); err != nil {
		return fmt.Errorf("reach Oak gate approach: %w", err)
	}
	// The northward step is itself the story trigger. An interruption error is
	// acceptable when the following semantic read shows Oak appeared.
	return skill.StepOnce(m, world.StepUp)
}

func yellowOpeningAdvanceScript(m *emu.Emu) error {
	for frame := 0; frame < yellowOpeningScriptBudget; frame++ {
		facts := yellowprofile.DecodeOpening(m)
		phase, err := yellowOpeningPhaseFor(facts)
		if err != nil {
			return err
		}
		if phase != yellowOpeningScript {
			return nil
		}
		if yellowOpeningScriptNeedsConfirm(facts) {
			m.Tap(emu.A, 3, 7)
			continue
		}
		m.StepFrame()
	}
	f := yellowprofile.DecodeOpening(m)
	return fmt.Errorf("%w: scripted phase exceeded %d frames on map %#02x at (%d,%d)",
		errYellowOpeningStalled, yellowOpeningScriptBudget, f.Map, f.X, f.Y)
}

// yellowOpeningScriptNeedsConfirm separates battle text from Oak's simulated
// menu ownership. BATTLE_TYPE_PIKACHU still prints ordinary MESSAGE_BOX text
// ("Wild PIKACHU appeared!" and capture follow-ups), which waits for A. Once
// DisplayBattleMenu takes over it writes BATTLE_MENU_TEMPLATE, and the
// simulated item list writes LIST_MENU_BOX; those states must advance on
// frames only so player input cannot race the ROM-owned tutorial.
func yellowOpeningScriptNeedsConfirm(f yellowprofile.OpeningFacts) bool {
	if !f.InBattle {
		return f.TextOpen
	}
	return f.TextBoxID == yellowOpeningMessageBoxID
}

func yellowOpeningTakeEeveeBall(m *emu.Emu, romData []byte) error {
	if err := skill.GoTo(m, romData, skill.ExactDestination(
		yellowprofile.OaksLabMap, yellowOpeningBallStandX, yellowOpeningBallStandY)); err != nil {
		return fmt.Errorf("reach Eevee ball: %w", err)
	}
	if err := skill.Face(m, yellowOpeningBallX, yellowOpeningBallY); err != nil {
		return fmt.Errorf("face Eevee ball: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	for frame := 0; frame < yellowOpeningBallReactionBudget; frame++ {
		facts := yellowprofile.DecodeOpening(m)
		if !facts.Controllable || facts.GotStarter {
			return nil
		}
		m.StepFrame()
	}
	f := yellowprofile.DecodeOpening(m)
	return fmt.Errorf("%w: Eevee-ball script did not start at (%d,%d); player at (%d,%d)",
		errYellowOpeningStalled, yellowOpeningBallX, yellowOpeningBallY, f.X, f.Y)
}

func yellowOpeningTriggerRival(m *emu.Emu, romData []byte) error {
	err := skill.GoTo(m, romData, skill.ExactDestination(
		yellowprofile.OaksLabMap, yellowOpeningRivalX, yellowOpeningRivalY))
	if errors.Is(err, skill.ErrDialogueInterrupted) || errors.Is(err, skill.ErrBattleInterrupted) || errors.Is(err, skill.ErrBattle) {
		return nil
	}
	if err != nil {
		return err
	}

	// GoTo can positively finish the exact-tile request on the same frame the
	// player reaches y=6, before OaksLabRivalChallengesPlayerScript consumes
	// that coordinate. A second GoTo then has nothing to do, which previously
	// looked like two identical semantic states and tripped the opening stall
	// detector (#2071). Yield to Yellow's map script until it takes ownership;
	// once dialogue/movement/battle starts, the outer phase machine resumes
	// with the appropriate script or battle owner.
	for frame := 0; frame < yellowOpeningRivalTriggerBudget; frame++ {
		facts := yellowprofile.DecodeOpening(m)
		phase, phaseErr := yellowOpeningPhaseFor(facts)
		if phaseErr != nil {
			return phaseErr
		}
		if phase != yellowOpeningWalkToRival {
			return nil
		}
		m.StepFrame()
	}
	facts := yellowprofile.DecodeOpening(m)
	return fmt.Errorf("%w: rival trigger did not take script ownership within %d frames on map %#02x at (%d,%d)",
		errYellowOpeningStalled, yellowOpeningRivalTriggerBudget, facts.Map, facts.X, facts.Y)
}
