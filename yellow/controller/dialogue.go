package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowDialogueFrameBudget = 6000
	yellowDialogueStableFrames = 8
)

var ErrDialogueChoiceRequired = errors.New("yellow dialogue: choice requires explicit policy")

type dialoguePhase uint8

const (
	dialoguePhaseWait dialoguePhase = iota
	dialoguePhasePage
	dialoguePhaseChoice
	dialoguePhaseDone
)

func dialoguePhaseFor(text string, maxMenu, fontLoaded uint8, controllable bool) dialoguePhase {
	upper := strings.ToUpper(text)
	if maxMenu == 1 && strings.Contains(upper, "YES") && strings.Contains(upper, "NO") {
		return dialoguePhaseChoice
	}
	if controllable && fontLoaded == 0 {
		return dialoguePhaseDone
	}
	if fontLoaded != 0 {
		return dialoguePhasePage
	}
	return dialoguePhaseWait
}

// RecoverDialogue advances an ordinary Yellow cutscene/dialogue until the
// player regains control or a battle begins. It never answers an unknown
// two-option prompt; callers must provide a story-specific policy for choices.
func RecoverDialogue(m *emu.Emu, romData []byte) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("yellow dialogue: nil emulator")
	}

	start := m.FrameCount()
	presses := 0
	stable := 0
	for int(m.FrameCount()-start) <= yellowDialogueFrameBudget {
		obs, err := yellowprofile.New().DecodeObservation(m, romData)
		if err != nil {
			return presses, fmt.Errorf("yellow dialogue: observe: %w", err)
		}
		if obs.InBattle {
			return presses, nil
		}

		phase := dialoguePhaseFor(screenText(m), m.Peek8(sym.MaxMenuItem), m.Peek8(sym.FontLoaded), obs.Controllable)
		switch phase {
		case dialoguePhaseChoice:
			return presses, fmt.Errorf("%w: screen=%q", ErrDialogueChoiceRequired,
				strings.Join(strings.Fields(screenText(m)), " "))
		case dialoguePhaseDone:
			stable++
			if stable >= yellowDialogueStableFrames {
				return presses, nil
			}
			m.StepFrame()
		case dialoguePhasePage:
			stable = 0
			m.Tap(emu.A, 3, 7)
			presses++
		case dialoguePhaseWait:
			stable = 0
			m.StepFrame()
		}
	}

	return presses, fmt.Errorf("yellow dialogue: exceeded %d frames map=%#02x at (%d,%d) screen=%q",
		yellowDialogueFrameBudget, m.Peek8(sym.CurMap), m.Peek8(sym.XCoord), m.Peek8(sym.YCoord),
		strings.Join(strings.Fields(screenText(m)), " "))
}
