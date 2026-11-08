package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

// PokegearRadioState exposes only the Gold/Silver radio's live control facts.
// PokeGear enters on CLOCK, cycling right through the owned cards. After the
// radio page reaches state 12, the up/down knob is stored at wRadioTuningKnob.
// The caller must have opened Pokégear through the verified START menu: the
// jumptable is reused by other unrelated menus outside this owned action.
type PokegearRadioState struct {
	Page         byte
	Tuning       byte
	HasRadio     bool
	HasExpansion bool
}

const (
	gsPokegearClockJoypad byte = 1
	gsPokegearRadioJoypad byte = 12
	gsPokeFluteFrequency  byte = 78 // channel 20: 4*20 - 2
)

func (*Profile) DecodePokegearRadio(reader game.MemoryReader) PokegearRadioState {
	if reader == nil {
		return PokegearRadioState{}
	}
	flags := reader.Peek8(sym.PokegearFlags)
	return PokegearRadioState{
		Page:         reader.Peek8(sym.JumptableIndex),
		Tuning:       reader.Peek8(sym.RadioTuningKnob),
		HasRadio:     flags&gsPokegearRadioCardMask != 0,
		HasExpansion: flags&gsPokegearExpansionCardMask != 0,
	}
}

// PokeFluteTuned requires the radio cursor to be on the exact Gold/Silver
// channel 20. This is tuning evidence only; Snorlax is cleared solely by the
// real EVENT_FOUGHT_SNORLAX after its battle script.
func (s PokegearRadioState) PokeFluteTuned() bool {
	return s.HasRadio && s.HasExpansion && s.Page == gsPokegearRadioJoypad &&
		s.Tuning == gsPokeFluteFrequency
}
