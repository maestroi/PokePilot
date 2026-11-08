package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// RadioTower1F.asm: accept the quiz, then YES, YES, NO, YES, NO.
// The script checks ENGINE_RADIO_CARD only after all five answers. Choices
// are deliberate gameplay decisions and belong to this owned objective, not
// NormalizeBoundary or the generic A-to-advance interruption controller.
var gsRadioQuizAnswers = [...]int{0, 0, 0, 1, 0, 1}

const (
	gsRadioQuizFrameBudget uint64 = 30_000
	gsRadioQuizMaxTextPresses = 320
)

// gsRadioQuizAdvance handles one bounded live Radio Card quiz transaction.
// Every YES/NO must be rendered on screen before we answer, and no completion
// is claimed unless the cartridge really awards wPokegearFlags bit 1.
func gsRadioQuizAdvance(m *emu.Emu, profile *gsprofile.Profile) error {
	const label = "goldenrod:radio-card-quiz"
	start := m.FrameCount()
	answered, textPresses := 0, 0
	for m.FrameCount()-start < gsRadioQuizFrameBudget {
		world := profile.DecodeOverworld(m)
		if gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRadioCardReceived) {
			if world.Controllable {
				return nil
			}
		}
		if world.InBattle {
			return fmt.Errorf("%w: unexpected battle in Radio Card quiz", errGSSecondBadgeUnexpectedState)
		}
		if _, visible := profile.DecodeTwoOption(m); visible {
			if answered == len(gsRadioQuizAnswers) {
				return fmt.Errorf("%w: Radio Card quiz opened an unowned seventh choice", errGSSecondBadgeUnexpectedState)
			}
			index := gsRadioQuizAnswers[answered]
			if err := skill.SelectMenuItem(m, index); err != nil {
				return fmt.Errorf("gen2 %s: answer prompt %d (option %d): %w", label, answered+1, index, err)
			}
			answered++
			// The next prompt may use the same YES/NO menu RAM. Do not
			// mistake the previous unconsumed box for the next question.
			closed := false
			for waited := 0; waited < 90; waited++ {
				if _, shown := profile.DecodeTwoOption(m); !shown {
					closed = true
					break
				}
				m.StepFrames(2)
			}
			if !closed {
				return fmt.Errorf("%w: Radio Card prompt %d remained visible after choice",
					errGSSecondBadgeStalled, answered)
			}
			continue
		}
		if world.Controllable {
			if answered > 0 {
				return fmt.Errorf("%w: Radio Card quiz ended after %d of %d choices without a card",
					errGSSecondBadgeUnexpectedState, answered, len(gsRadioQuizAnswers))
			}
			return fmt.Errorf("%w: Radio Card quiz interaction did not begin", errGSSecondBadgeUnexpectedState)
		}
		if world.InDialogue && world.MovementIdle {
			if textPresses >= gsRadioQuizMaxTextPresses {
				return fmt.Errorf("%w: Radio Card quiz exceeded %d text presses", errGSSecondBadgeStalled, gsRadioQuizMaxTextPresses)
			}
			// A text page can finish drawing while YES/NO is being
			// laid out. The prompt detector above prevents answering
			// a choice via this text-progress input.
			m.Tap(emu.A, 3, 7)
			textPresses++
			continue
		}
		m.StepFrame()
	}
	return fmt.Errorf("%w: Radio Card quiz exceeded %d frames after %d prompts",
		errGSSecondBadgeStalled, gsRadioQuizFrameBudget, answered)
}

// executeGSRadioCardQuiz obtains the basic Radio Card in Goldenrod while
// the player is already nearby after Whitney. Leaving it until Kanto risks
// an unsupported return-trip transport when Diglett's Cave requires radio.
func executeGSRadioCardQuiz(m *emu.Emu, romData []byte) error {
	const label = "goldenrod:radio-card-quiz"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressRadioCardReceived, gsprofile.ProgressPlainBadgeEarned, label)
	if err != nil || done {
		return err
	}
	tower, err := gsOpeningMapID("RADIO_TOWER_1F")
	if err != nil {
		return err
	}
	if !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: cannot begin Radio Card quiz in a scripted scene",
			errGSSecondBadgeUnexpectedState)
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(tower, 12, 7), 12, 6, label); err != nil {
		return fmt.Errorf("gen2 Radio Card: reach Goldenrod quiz woman: %w", err)
	}
	m.StepFrames(gsLakeScriptSettleFrames)
	m.Tap(emu.A, 3, 7)
	if err := gsRadioQuizAdvance(m, profile); err != nil {
		return fmt.Errorf("gen2 Radio Card: quiz and receipt: %w", err)
	}
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRadioCardReceived) {
		return fmt.Errorf("%w: Radio Card quiz ended without the retail ownership flag", errGSSecondBadgeUnexpectedState)
	}
	return nil
}
