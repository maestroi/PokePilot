package profile

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestClockUntilSecondsMatchesGoldBoundaries(t *testing.T) {
	tests := []struct {
		name string
		rtc  emu.RTCState
		h, m, s byte
		until game.TimeOfDay
		want uint32
	}{
		{"night 03:59:59 to morning", emu.RTCState{Hours: 3, Minutes: 59, Seconds: 59}, 0, 0, 0, game.TimeMorning, 1},
		{"day 17:59:59 to night", emu.RTCState{Hours: 17, Minutes: 59, Seconds: 59}, 0, 0, 0, game.TimeNight, 1},
		{"morning 09:59:59 to day", emu.RTCState{Hours: 9, Minutes: 59, Seconds: 59}, 0, 0, 0, game.TimeDay, 1},
		{"night 20:15 to morning", emu.RTCState{Hours: 20, Minutes: 15}, 0, 0, 0, game.TimeMorning, 7*3600 + 45*60},
		{"offset 09:50 to day", emu.RTCState{Hours: 0, Minutes: 49, Seconds: 30}, 9, 10, 30, game.TimeDay, 0},
		{"offset carry into 10:00", emu.RTCState{Hours: 0, Minutes: 49, Seconds: 30}, 9, 10, 29, game.TimeDay, 1},
		{"already day", emu.RTCState{Hours: 12}, 0, 0, 0, game.TimeDay, 0},
		{"day to night", emu.RTCState{Hours: 12}, 0, 0, 0, game.TimeNight, 6 * 3600},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var memory fakeMemory
			memory[sym.StartHour], memory[sym.StartMinute], memory[sym.StartSecond] = tc.h, tc.m, tc.s
			for _, profile := range []*Profile{NewGold(), NewSilver()} {
				got, err := profile.ClockUntilSeconds(&memory, tc.rtc, tc.until)
				if err != nil || got != tc.want {
					t.Fatalf("%s: delta = %d, %v, want %d", profile.ID(), got, err, tc.want)
				}
			}
		})
	}
}

func TestClockLegErrorsAreTyped(t *testing.T) {
	var memory fakeMemory
	p := NewGold()
	if _, err := p.ClockUntilSeconds(&memory, emu.RTCState{}, "dawn"); !errors.Is(err, game.ErrUnknownClockPeriod) {
		t.Fatalf("unknown period: %v", err)
	}
	if _, err := p.ClockUntilSeconds(nil, emu.RTCState{}, game.TimeNight); !errors.Is(err, game.ErrClockUnavailable) {
		t.Fatalf("no memory: %v", err)
	}
	if _, err := p.ClockUntilSeconds(&memory, emu.RTCState{Halted: true}, game.TimeNight); !errors.Is(err, game.ErrClockHalted) {
		t.Fatalf("halted: %v", err)
	}
	memory[sym.StartHour] = 99
	if _, err := p.ClockUntilSeconds(&memory, emu.RTCState{}, game.TimeNight); !errors.Is(err, game.ErrClockUnavailable) {
		t.Fatalf("invalid offset: %v", err)
	}
	memory[sym.TimeOfDay] = 2
	if got := p.ObservedClockPeriod(&memory); got != game.TimeNight {
		t.Fatalf("wTimeOfDay = %q, want night", got)
	}
}
