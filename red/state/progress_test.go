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
