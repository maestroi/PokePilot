package game

import (
	"errors"
	"fmt"
)

var (
	// ErrClockUnavailable means the adapter has no emulated cartridge clock.
	ErrClockUnavailable = errors.New("game: clock unavailable")
	// ErrClockHalted means a supported cartridge clock is not ticking.
	ErrClockHalted = errors.New("game: clock halted")
	// ErrUnknownClockPeriod means the leg requests a period the executor cannot name.
	ErrUnknownClockPeriod = errors.New("game: unknown clock period")
	// ErrClockPostcondition means execution did not produce the required observed period.
	ErrClockPostcondition = errors.New("game: clock postcondition failed")
)

// ClockLegExecutor is the optional adapter-owned execution capability for a
// planned wait. Clockless games do not implement it.
type ClockLegExecutor interface {
	ExecuteClockLeg(ClockLeg) (TimeOfDay, error)
}

// PerformClockLeg executes and verifies the *observed* period, never merely an
// unverified emulator mutation. Generic callers know no native RTC registers.
func PerformClockLeg(executor ClockLegExecutor, leg ClockLeg) (TimeOfDay, error) {
	if !knownClockPeriod(leg.Until) {
		return "", fmt.Errorf("%w: %q", ErrUnknownClockPeriod, leg.Until)
	}
	if executor == nil {
		return "", ErrClockUnavailable
	}
	observed, err := executor.ExecuteClockLeg(leg)
	if err != nil {
		return observed, err
	}
	if observed != leg.Until {
		return observed, fmt.Errorf("%w: wanted %s, observed %s", ErrClockPostcondition, leg.Until, observed)
	}
	return observed, nil
}
