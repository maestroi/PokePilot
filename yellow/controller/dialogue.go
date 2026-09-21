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


// TalkAt approaches one map object, faces it, presses A once, and requires the
// interaction to produce a real control-state transition before it is counted.
// Ordinary dialogue is recovered to a stable boundary; unknown choices fail
// closed so a generic talk objective cannot make story decisions by accident.
func TalkAt(m *emu.Emu, romData []byte, x, y uint8) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("yellow talk: nil emulator")
	}
	if err := interactAt(m, romData, int(x), int(y)); err != nil {
		return 0, fmt.Errorf("yellow talk: approach (%d,%d): %w", x, y, err)
	}

	transitioned := false
	for frame := 0; frame < 240; frame++ {
		obs, err := yellowprofile.New().DecodeObservation(m, romData)
		if err != nil {
			return 0, fmt.Errorf("yellow talk: observe interaction: %w", err)
		}
		if obs.InBattle || m.Peek8(sym.FontLoaded) != 0 || !obs.Controllable {
			transitioned = true
			break
		}
		m.StepFrame()
	}
	if !transitioned {
		return 0, fmt.Errorf("yellow talk: interaction at (%d,%d) produced no dialogue, battle, or script transition", x, y)
	}

	presses := 1
	if m.Peek8(sym.IsInBattle) != 0 {
		if _, err := Battle(m, romData); err != nil {
			return presses, fmt.Errorf("yellow talk: battle after interaction: %w", err)
		}
	}
	pages, err := RecoverDialogue(m, romData)
	presses += pages
	if err != nil {
		return presses, fmt.Errorf("yellow talk: recover dialogue: %w", err)
	}
	return presses, nil
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
