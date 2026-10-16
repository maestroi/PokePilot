package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

const secondsPerDay = 24 * 60 * 60

// ObservedClockPeriod reads the game's authoritative wTimeOfDay byte. A raw
// cartridge RTC hour is not by itself evidence the game has updated its time.
func (*Profile) ObservedClockPeriod(reader game.MemoryReader) game.TimeOfDay {
	if reader == nil {
		return ""
	}
	return timeOfDay(reader.Peek8(sym.TimeOfDay))
}

// ClockUntilSeconds translates a portable leg into emulated RTC seconds.
// Gold/Silver FixTime adds wStartHour/Minute/Second to the raw MBC3 RTC (with
// carries); target boundaries come from TimesOfDay in engine/rtc/rtc.asm.
// No wall time, native map assumptions or real-hour frame stepping.
func (*Profile) ClockUntilSeconds(reader game.MemoryReader, rtc emu.RTCState, until game.TimeOfDay) (uint32, error) {
	var boundary int
	switch until {
	case game.TimeMorning:
		boundary = 4 * 3600
	case game.TimeDay:
		boundary = 10 * 3600
	case game.TimeNight:
		boundary = 18 * 3600
	default:
		return 0, fmt.Errorf("%w: %q", game.ErrUnknownClockPeriod, until)
	}
	if reader == nil {
		return 0, game.ErrClockUnavailable
	}
	if rtc.Halted {
		return 0, game.ErrClockHalted
	}
	h, m, s := reader.Peek8(sym.StartHour), reader.Peek8(sym.StartMinute), reader.Peek8(sym.StartSecond)
	if h >= 24 || m >= 60 || s >= 60 || rtc.Hours >= 24 || rtc.Minutes >= 60 || rtc.Seconds >= 60 {
		return 0, fmt.Errorf("%w: invalid Gold/Silver RTC or start-time offset", game.ErrClockUnavailable)
	}
	now := (int(rtc.Hours)*3600 + int(rtc.Minutes)*60 + int(rtc.Seconds) +
		int(h)*3600 + int(m)*60 + int(s)) % secondsPerDay
	// The observed period can lag a fresh RTC latch; use the *actual* clock
	// for the delta. Already in the target period? Just settle the observation.
	if (until == game.TimeMorning && now >= 4*3600 && now < 10*3600) ||
		(until == game.TimeDay && now >= 10*3600 && now < 18*3600) ||
		(until == game.TimeNight && (now >= 18*3600 || now < 4*3600)) {
		return 0, nil
	}
	return uint32((boundary - now + secondsPerDay) % secondsPerDay), nil
}
