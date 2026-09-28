package profile

import (
	"github.com/maestroi/pokepilot/game"
)

const ilexFarfetchdPosition1Event uint16 = 1769

// IlexState is the semantic, resumable projection of the Farfetch'd story
// puzzle. The retail map represents the bird as ten mutually-exclusive object
// events; keeping that encoding here prevents the agent from depending on
// event ids or object slots.
type IlexState struct {
	Herded    bool
	Position  int
	HM01Cut   bool
}

// DecodeIlexState reports the currently visible Farfetch'd position while the
// puzzle is active. Position 10 is the final bird interaction before HERDED is
// set. Once the bird has been returned, Position remains 10 as the semantic
// completed endpoint even though the object itself is hidden again.
func (*Profile) DecodeIlexState(reader game.MemoryReader) IlexState {
	if reader == nil {
		return IlexState{}
	}
	out := IlexState{
		Herded:  hasGSEvent(reader, eventHerdedFarfetchd),
		HM01Cut: hasGSEvent(reader, eventGotHM01Cut),
	}
	if out.Herded {
		out.Position = 10
		return out
	}
	for pos := 1; pos <= 10; pos++ {
		if !hasGSEvent(reader, ilexFarfetchdPosition1Event+uint16(pos-1)) {
			out.Position = pos
			break
		}
	}
	return out
}
