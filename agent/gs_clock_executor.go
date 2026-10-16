package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

const gsClockSettleFrames = 180

var _ game.ClockLegExecutor = (*gsObjectiveAdapter)(nil)

// ExecuteClockLeg is the Gold/Silver execution half of the portable
// wait-until leg. The live MBC3 RTC is advanced once; a bounded handful of
// idle frames allows the cartridge's UpdateTime/GetTimeOfDay to refresh RAM.
// Red has no executor and never needs one.
func (a *gsObjectiveAdapter) ExecuteClockLeg(leg game.ClockLeg) (game.TimeOfDay, error) {
	if a == nil || a.m == nil {
		return "", game.ErrClockUnavailable
	}
	profile := gsprofile.New(a.gameID)
	if leg.Until != game.TimeMorning && leg.Until != game.TimeDay && leg.Until != game.TimeNight {
		return "", fmt.Errorf("%w: %q", game.ErrUnknownClockPeriod, leg.Until)
	}
	rtc, err := a.m.RTC()
	if err != nil {
		if errors.Is(err, emu.ErrRTCUnavailable) {
			return "", game.ErrClockUnavailable
		}
		if errors.Is(err, emu.ErrRTCHalted) {
			return "", game.ErrClockHalted
		}
		return "", err
	}
	delta, err := profile.ClockUntilSeconds(a.m, rtc, leg.Until)
	if err != nil {
		return "", err
	}
	if delta > 0 {
		if err := a.m.AdvanceRTCSeconds(delta); err != nil {
			switch {
			case errors.Is(err, emu.ErrRTCUnavailable):
				return "", game.ErrClockUnavailable
			case errors.Is(err, emu.ErrRTCHalted):
				return "", game.ErrClockHalted
			default:
				return "", err
			}
		}
		// Do not accept a stale, pre-advance wTimeOfDay value as success.
		a.m.StepFrame()
	}
	for i := 0; i < gsClockSettleFrames; i++ {
		observed := profile.ObservedClockPeriod(a.m)
		if observed == leg.Until {
			return observed, nil
		}
		a.m.StepFrame()
	}
	observed := profile.ObservedClockPeriod(a.m)
	return observed, fmt.Errorf("%w: wanted %s, Gold/Silver observed %s after %d settle frames",
		game.ErrClockPostcondition, leg.Until, observed, gsClockSettleFrames)
}
