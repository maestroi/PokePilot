package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func clearGSEvent(mem fakeGSReader, event uint16) {
	mem[sym.EventFlags+event/8] &^= byte(1 << uint(event%8))
}

func TestDecodeIlexStateTracksVisibleFarfetchdPosition(t *testing.T) {
	mem := fakeGSReader{}
	for pos := 1; pos <= 10; pos++ {
		setGSEvent(mem, ilexFarfetchdPosition1Event+uint16(pos-1))
	}
	clearGSEvent(mem, ilexFarfetchdPosition1Event+5) // position 6 visible

	got := NewGold().DecodeIlexState(mem)
	if got.Herded || got.HM01Cut || got.Position != 6 {
		t.Fatalf("Ilex state = %+v, want active position 6", got)
	}

	setGSEvent(mem, eventHerdedFarfetchd)
	got = NewGold().DecodeIlexState(mem)
	if !got.Herded || got.Position != 10 {
		t.Fatalf("herded Ilex state = %+v, want completed position 10", got)
	}

	setGSEvent(mem, eventGotHM01Cut)
	got = NewGold().DecodeIlexState(mem)
	if !got.HM01Cut {
		t.Fatalf("HM01 event missing from Ilex state: %+v", got)
	}
}
