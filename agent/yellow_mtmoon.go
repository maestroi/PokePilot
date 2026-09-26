package agent

import (
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

const (
	yellowMtMoonB2FMap       uint8 = 0x3d
	yellowMtMoonJessieJamesX uint8 = 3
	yellowMtMoonJessieJamesY uint8 = 5
)

// yellowMtMoonExitTrigger is Yellow-owned data, not a Yellow-owned execution
// loop. The generic scripted-progress executor owns navigation, interruption
// handling, battle resolution, resumption and semantic completion.
func yellowMtMoonExitTrigger() scriptedProgressTrigger {
	return scriptedProgressTrigger{
		Name:        "yellow Mt. Moon Jessie/James",
		Destination: skill.ExactDestination(yellowMtMoonB2FMap, yellowMtMoonJessieJamesX, yellowMtMoonJessieJamesY),
		Progress:    yellowprofile.ProgressYellowMtMoonExitResolved,
		MaxFrames:   30000,
		IdleFrames:  90,
		AdvanceA:    true,
	}
}
