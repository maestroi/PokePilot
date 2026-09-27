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
	yellowOpeningMaxSteps                 = 40
	yellowOpeningScriptBudget             = 30000
	yellowOpeningBallReactionBudget       = 600
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
	if f.ChoicePrompt {
		return "", errYellowOpeningChoiceRequired
	}
	switch {
	case yellowOpeningReached(f):
		return yellowOpeningDone, nil
	case f.InBattle:
		if f.GotStarter && !f.BattledRival {
			return yellowOpeningFightRival, nil
		}
		return yellowOpeningScript, nil
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
		// Before the player receives Pikachu, Yellow's only opening battle is
		// Professor Oak's BATTLE_TYPE_PIKACHU tutorial. The ROM simulates its
		// menu/item inputs itself; player A presses can race that script and
		// leave the opening in a non-progressing tutorial state (#2050).
		// Rival combat is classified as yellowOpeningFightRival instead and is
		// driven by skill.Battle, so an in-battle scripted phase is safe to
		// advance with frames only.
		if facts.InBattle {
			m.StepFrame()
			continue
		}
		if facts.TextOpen {
			m.Tap(emu.A, 3, 7)
			continue
		}
		m.StepFrame()
	}
	f := yellowprofile.DecodeOpening(m)
	return fmt.Errorf("%w: scripted phase exceeded %d frames on map %#02x at (%d,%d)",
		errYellowOpeningStalled, yellowOpeningScriptBudget, f.Map, f.X, f.Y)
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
	return err
}
