package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

var namedEvents = []Event{
	EventFollowedOakIntoLab,
	EventOakAskedToChooseMon,
	EventGotStarter,
	EventBattledRivalInOaksLab,
	EventGotPokeballsFromOak,
	EventGotPokedex,
	EventOakAppearedInPallet,
}

func TestHasEventByteBitArithmetic(t *testing.T) {
	// EventGotStarter is bit 34 of wEventFlags: byte 0xD747+34/8 = 0xD74B,
	// bit 34%8 = 2.
	if got, want := sym.EventFlags+uint16(EventGotStarter)/8, uint16(0xD74B); got != want {
		t.Fatalf("EventGotStarter byte = %#x, want %#x", got, want)
	}
	if got, want := uint16(EventGotStarter)%8, uint16(2); got != want {
		t.Fatalf("EventGotStarter bit = %d, want %d", got, want)
	}
	if got, want := sym.EventFlags+uint16(EventFollowedOakIntoLab)/8, uint16(0xD747); got != want {
		t.Fatalf("EventFollowedOakIntoLab byte = %#x, want %#x", got, want)
	}
	if got, want := uint16(EventFollowedOakIntoLab)%8, uint16(0); got != want {
		t.Fatalf("EventFollowedOakIntoLab bit = %d, want %d", got, want)
	}
}

func TestHasEvent(t *testing.T) {
	tests := []struct {
		name string
		mem  func() *Mem
		want map[Event]bool
	}{
		{
			name: "unset bitfield reports false for all named events",
			mem:  func() *Mem { return &Mem{} },
			want: map[Event]bool{},
		},
		{
			name: "0xD747 bit 0 drives EventFollowedOakIntoLab",
			mem: func() *Mem {
				m := &Mem{}
				m[0xD747] = 0x01
				return m
			},
			want: map[Event]bool{EventFollowedOakIntoLab: true},
		},
		{
			name: "0xD74B bit 2 drives EventGotStarter and no other named event",
			mem: func() *Mem {
				m := &Mem{}
				m[0xD74B] = 0x04
				return m
			},
			want: map[Event]bool{EventGotStarter: true},
		},
		{
			name: "all bits set reports true for every named event",
			mem: func() *Mem {
				m := &Mem{}
				m[0xD747] = 0xFF
				m[0xD74B] = 0xFF
				return m
			},
			want: map[Event]bool{
				EventFollowedOakIntoLab:    true,
				EventOakAskedToChooseMon:   true,
				EventGotStarter:            true,
				EventBattledRivalInOaksLab: true,
				EventGotPokeballsFromOak:   true,
				EventGotPokedex:            true,
				EventOakAppearedInPallet:   true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.mem()
			for _, e := range namedEvents {
				if got := HasEvent(m, e); got != tt.want[e] {
					t.Errorf("HasEvent(%s) = %v, want %v", e, got, tt.want[e])
				}
			}
		})
	}
}

func TestEventString(t *testing.T) {
	for _, e := range namedEvents {
		if e.String() == "" {
			t.Errorf("String(%d) is empty", uint16(e))
		}
	}
	if got, want := Event(999).String(), "unknown(999)"; got != want {
		t.Errorf("String(999) = %q, want %q", got, want)
	}
}

func TestOakAppearedInPalletIndex(t *testing.T) {
	// EventOakAppearedInPallet is bit 39 of wEventFlags: byte 0xD747+39/8 =
	// 0xD74B, bit 39%8 = 7. The shipped build had it at 38 (bit 6 of the
	// same byte) because the original count dropped two entries.
	m := &Mem{}
	m[0xD747+4] = 1 << 7
	if !HasEvent(m, EventOakAppearedInPallet) {
		t.Error("bit 7 of 0xD747+4 should set EventOakAppearedInPallet")
	}
	m = &Mem{}
	m[0xD747+4] = 1 << 6
	if HasEvent(m, EventOakAppearedInPallet) {
		t.Error("bit 6 of 0xD747+4 must not set EventOakAppearedInPallet (the off-by-one that shipped)")
	}
	// EventGotPokedex is bit 37: byte 0xD74B, bit 5 — the same byte.
	m = &Mem{}
	m[0xD747+4] = 1 << 5
	if !HasEvent(m, EventGotPokedex) {
		t.Error("bit 5 of 0xD747+4 should set EventGotPokedex")
	}
	if HasEvent(m, EventOakAppearedInPallet) {
		t.Error("bit 5 of 0xD747+4 must not set EventOakAppearedInPallet")
	}
}

func TestTookStarterBall(t *testing.T) {
	// TookStarterBall is wStatusFlags4 (0xD72E) bit 3, set the moment the
	// ball is taken. EventGotStarter is wEventFlags bit 34 (byte 0xD74B,
	// bit 2), set only later when the rival takes his mon.
	m := &Mem{}
	m[sym.StatusFlags4] = 1 << 3
	if !TookStarterBall(m) {
		t.Error("bit 3 of 0xD72E should report TookStarterBall")
	}
	// EventGotStarter set must not flip TookStarterBall.
	m = &Mem{}
	m[0xD74B] = 1 << 2
	if !HasEvent(m, EventGotStarter) {
		t.Fatal("sanity: bit 2 of 0xD74B should set EventGotStarter")
	}
	if TookStarterBall(m) {
		t.Error("EventGotStarter must not set TookStarterBall")
	}
	// And TookStarterBall must not flip EventGotStarter.
	m = &Mem{}
	m[sym.StatusFlags4] = 1 << 3
	if !TookStarterBall(m) {
		t.Fatal("sanity: bit 3 of 0xD72E should report TookStarterBall")
	}
	if HasEvent(m, EventGotStarter) {
		t.Error("TookStarterBall must not set EventGotStarter")
	}
}
