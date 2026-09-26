package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowsym "github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowOpeningPalletTown  uint8 = 0x00
	yellowOpeningRedsHouse1F uint8 = 0x25
	yellowOpeningRedsHouse2F uint8 = 0x26
	yellowOpeningOaksLab     uint8 = 0x28

	yellowOpeningFrameBudget  = 90000
	yellowOpeningBattleBudget = 30000
	yellowOpeningMenuBudget   = 120
)

type yellowOpeningState struct {
	mapID        uint8
	x, y         uint8
	controllable bool
	inBattle     bool
	partyCount   int
	hasPikachu   bool
	starter      bool
	labRival     bool
}

type yellowOpeningPhase string

const (
	yellowOpeningDone              yellowOpeningPhase = "done"
	yellowOpeningBattle            yellowOpeningPhase = "battle"
	yellowOpeningNickname          yellowOpeningPhase = "nickname"
	yellowOpeningScript            yellowOpeningPhase = "script"
	yellowOpeningBedroomUpstairs   yellowOpeningPhase = "bedroom-upstairs"
	yellowOpeningBedroomDownstairs yellowOpeningPhase = "bedroom-downstairs"
	yellowOpeningOakGate           yellowOpeningPhase = "oak-gate"
	yellowOpeningEeveeBall         yellowOpeningPhase = "eevee-ball"
	yellowOpeningAwaitStarter      yellowOpeningPhase = "await-starter"
	yellowOpeningRivalTrigger      yellowOpeningPhase = "rival-trigger"
	yellowOpeningUnexpected        yellowOpeningPhase = "unexpected"
)

func yellowOpeningPhaseFor(state yellowOpeningState, nickname bool) yellowOpeningPhase {
	if state.labRival {
		if state.starter && state.hasPikachu && state.controllable && !state.inBattle {
			return yellowOpeningDone
		}
		if !state.starter || !state.hasPikachu {
			return yellowOpeningUnexpected
		}
	}
	if state.inBattle {
		return yellowOpeningBattle
	}
	if nickname {
		return yellowOpeningNickname
	}
	if !state.controllable {
		return yellowOpeningScript
	}
	if !state.starter {
		switch state.mapID {
		case yellowOpeningRedsHouse2F:
			return yellowOpeningBedroomUpstairs
		case yellowOpeningRedsHouse1F:
			return yellowOpeningBedroomDownstairs
		case yellowOpeningPalletTown:
			return yellowOpeningOakGate
		case yellowOpeningOaksLab:
			if state.partyCount == 0 {
				return yellowOpeningEeveeBall
			}
			return yellowOpeningAwaitStarter
		default:
			return yellowOpeningUnexpected
		}
	}
	if state.mapID == yellowOpeningOaksLab {
		return yellowOpeningRivalTrigger
	}
	return yellowOpeningUnexpected
}

func observeYellowOpening(m *emu.Emu, romData []byte) (yellowOpeningState, error) {
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return yellowOpeningState{}, err
	}
	hasPikachu := false
	for _, mon := range obs.Party {
		if mon.Species == "pikachu" {
			hasPikachu = true
			break
		}
	}
	return yellowOpeningState{
		mapID:        uint8(obs.NativeMapID),
		x:            obs.X,
		y:            obs.Y,
		controllable: obs.Controllable,
		inBattle:     obs.InBattle,
		partyCount:   len(obs.Party),
		hasPikachu:   hasPikachu,
		starter:      obs.Story.Has(yellowprofile.ProgressYellowStarterReceived),
		labRival:     obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved),
	}, nil
}

// executeYellowOpening drives Yellow's scripted Pikachu opening as a resumable
// story state machine. Navigation and the player-owned rival battle use the
// shared profile-driven skill runtime. Yellow-specific code owns only the
// cartridge's scripted decisions: Oak's interception/capture, the Eevee-ball
// interaction, nickname refusal, and the rival trigger.
func executeYellowOpening(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow opening: nil emulator")
	}
	info := game.InspectROM(romData)
	if info.SHA1 != yellowsym.ROMSHA1 {
		return fmt.Errorf("yellow opening: ROM sha1=%s, want %s", info.SHA1, yellowsym.ROMSHA1)
	}

	policy := skill.StatAwareMove(romData)
	start := m.FrameCount()
	for int(m.FrameCount()-start) <= yellowOpeningFrameBudget {
		state, err := observeYellowOpening(m, romData)
		if err != nil {
			return fmt.Errorf("yellow opening: observe: %w", err)
		}
		phase := yellowOpeningPhaseFor(state, yellowOpeningNicknamePrompt(m))
		switch phase {
		case yellowOpeningDone:
			return nil

		case yellowOpeningBattle:
			if state.starter {
				if _, err := skill.Battle(m, policy); err != nil {
					return fmt.Errorf("yellow opening: resolve lab rival battle: %w", err)
				}
			} else if err := advanceYellowScriptedCaptureBattle(m, romData); err != nil {
				return err
			}

		case yellowOpeningNickname:
			if err := declineYellowOpeningNickname(m); err != nil {
				return err
			}

		case yellowOpeningScript, yellowOpeningAwaitStarter:
			advanceYellowOpeningScriptFrame(m)

		case yellowOpeningBedroomUpstairs:
			if err := yellowOpeningGoTo(m, romData, skill.MapDestination(yellowOpeningRedsHouse1F), false); err != nil {
				return fmt.Errorf("yellow opening: leave upstairs bedroom: %w", err)
			}

		case yellowOpeningBedroomDownstairs:
			if err := yellowOpeningGoTo(m, romData, skill.MapDestination(yellowOpeningPalletTown), false); err != nil {
				return fmt.Errorf("yellow opening: leave house: %w", err)
			}

		case yellowOpeningOakGate:
			if err := yellowOpeningGoTo(m, romData, skill.ExactDestination(yellowOpeningPalletTown, 10, 0), true); err != nil {
				return fmt.Errorf("yellow opening: reach Oak interception: %w", err)
			}

		case yellowOpeningEeveeBall:
			if err := interactYellowOpeningTarget(m, romData, 7, 3); err != nil {
				return fmt.Errorf("yellow opening: trigger Eevee-ball script: %w", err)
			}

		case yellowOpeningRivalTrigger:
			if err := yellowOpeningGoTo(m, romData, skill.ExactDestination(yellowOpeningOaksLab, 5, 6), true); err != nil {
				return fmt.Errorf("yellow opening: reach rival trigger: %w", err)
			}

		case yellowOpeningUnexpected:
			return fmt.Errorf(
				"yellow opening: unexpected state map=%#02x (%d,%d) controllable=%v battle=%v party=%d pikachu=%v starter=%v lab_rival=%v",
				state.mapID, state.x, state.y, state.controllable, state.inBattle,
				state.partyCount, state.hasPikachu, state.starter, state.labRival,
			)
		}
	}

	state, _ := observeYellowOpening(m, romData)
	return fmt.Errorf(
		"yellow opening: exceeded %d frames at map=%#02x (%d,%d) controllable=%v battle=%v party=%d starter=%v lab_rival=%v",
		yellowOpeningFrameBudget, state.mapID, state.x, state.y, state.controllable,
		state.inBattle, state.partyCount, state.starter, state.labRival,
	)
}

func yellowOpeningGoTo(m *emu.Emu, romData []byte, dest skill.Destination, allowStoryInterrupt bool) error {
	err := skill.GoTo(m, romData, dest)
	if err == nil {
		return nil
	}
	if allowStoryInterrupt && (errors.Is(err, skill.ErrDialogueInterrupted) ||
		errors.Is(err, skill.ErrBattleInterrupted) || errors.Is(err, skill.ErrBattle)) {
		return nil
	}
	return err
}

func interactYellowOpeningTarget(m *emu.Emu, romData []byte, tx, ty uint8) error {
	if err := yellowOpeningGoTo(m, romData, skill.InteractionDestination(yellowOpeningOaksLab, tx, ty), false); err != nil {
		return err
	}
	state, err := observeYellowOpening(m, romData)
	if err != nil {
		return err
	}
	btn, ok := yellowOpeningFacingButton(state.x, state.y, tx, ty)
	if !ok {
		return fmt.Errorf("target (%d,%d) is not adjacent to player (%d,%d)", tx, ty, state.x, state.y)
	}
	m.Tap(btn, 3, 7)
	m.Tap(emu.A, 3, 7)
	return nil
}

func yellowOpeningFacingButton(x, y, tx, ty uint8) (emu.Button, bool) {
	switch {
	case x == tx && y+1 == ty:
		return emu.Down, true
	case x == tx && y == ty+1:
		return emu.Up, true
	case x+1 == tx && y == ty:
		return emu.Right, true
	case x == tx+1 && y == ty:
		return emu.Left, true
	default:
		return 0, false
	}
}

func advanceYellowOpeningScriptFrame(m *emu.Emu) {
	if m.Peek8(yellowsym.FontLoaded) != 0 {
		m.Tap(emu.A, 3, 7)
		return
	}
	m.StepFrame()
}

func yellowOpeningScreenText(m *emu.Emu) string {
	buf := make([]byte, yellowsym.TileMapLen)
	m.PeekInto(yellowsym.TileMap, buf)
	return gen1.NormalizeDisplayText(gen1.DecodeTiles(buf))
}

func yellowOpeningNicknamePrompt(m *emu.Emu) bool {
	return m.Peek8(yellowsym.MaxMenuItem) == 1 &&
		strings.Contains(strings.ToLower(yellowOpeningScreenText(m)), "give a nickname")
}

func declineYellowOpeningNickname(m *emu.Emu) error {
	if !yellowOpeningNicknamePrompt(m) {
		return fmt.Errorf("yellow opening: nickname prompt disappeared before selection")
	}
	if m.Peek8(yellowsym.CurrentMenuItem) != 1 {
		m.Tap(emu.Down, 3, 7)
		if _, err := m.StepUntil(yellowOpeningMenuBudget, func(m *emu.Emu) bool {
			return m.Peek8(yellowsym.CurrentMenuItem) == 1
		}); err != nil {
			return fmt.Errorf("yellow opening: nickname NO cursor did not settle: %w", err)
		}
	}
	m.Tap(emu.A, 3, 7)
	return nil
}

// Oak's Pikachu encounter is a scripted capture, not a player-owned battle.
// Advance only rendered text/script frames until Yellow clears the battle flag;
// using the generic battle controller here would invent a FIGHT/ITEM decision
// that the cartridge never offers.
func advanceYellowScriptedCaptureBattle(m *emu.Emu, romData []byte) error {
	start := m.FrameCount()
	for int(m.FrameCount()-start) <= yellowOpeningBattleBudget {
		state, err := observeYellowOpening(m, romData)
		if err != nil {
			return fmt.Errorf("yellow opening: observe scripted capture: %w", err)
		}
		if !state.inBattle {
			return nil
		}
		advanceYellowOpeningScriptFrame(m)
	}
	return fmt.Errorf("yellow opening: scripted Pikachu capture exceeded %d frames", yellowOpeningBattleBudget)
}
