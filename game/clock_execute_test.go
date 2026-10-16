package game

import (
	"errors"
	"testing"
)

type stubClockExecutor struct {
	observed TimeOfDay
	err      error
}

func (s stubClockExecutor) ExecuteClockLeg(ClockLeg) (TimeOfDay, error) {
	return s.observed, s.err
}

func TestPerformClockLegRequiresObservedPeriod(t *testing.T) {
	for _, period := range []TimeOfDay{TimeMorning, TimeDay, TimeNight} {
		got, err := PerformClockLeg(stubClockExecutor{observed: period}, ClockLeg{Until: period})
		if err != nil || got != period {
			t.Fatalf("until %s: observed %s, error %v", period, got, err)
		}
	}
	got, err := PerformClockLeg(stubClockExecutor{observed: TimeNight}, ClockLeg{Until: TimeMorning})
	if got != TimeNight || !errors.Is(err, ErrClockPostcondition) {
		t.Fatalf("missing postcondition: %s, %v", got, err)
	}
	if _, err := PerformClockLeg(nil, ClockLeg{Until: TimeMorning}); !errors.Is(err, ErrClockUnavailable) {
		t.Fatalf("clockless adapter: %v", err)
	}
	if _, err := PerformClockLeg(nil, ClockLeg{Until: "twilight"}); !errors.Is(err, ErrUnknownClockPeriod) {
		t.Fatalf("invalid period: %v", err)
	}
	if _, err := PerformClockLeg(stubClockExecutor{err: ErrClockHalted}, ClockLeg{Until: TimeDay}); !errors.Is(err, ErrClockHalted) {
		t.Fatalf("halted clock failure: %v", err)
	}
}
