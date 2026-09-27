package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/gs/sym"
)

// OpeningFacts are the semantic facts needed by the Gold/Silver starter
// controller. Raw party/map/script encodings stay behind the profile.
type OpeningFacts struct {
	NativeMapID uint16
	X, Y        uint8
	Facing      string

	Controllable bool
	MovementIdle bool
	InBattle     bool
	ScriptActive bool

	Party []game.SpeciesID
}

func (f OpeningFacts) HasSpecies(species game.SpeciesID) bool {
	for _, have := range f.Party {
		if have == species {
			return true
		}
	}
	return false
}

// DecodeOpening projects only the state needed to resume the known New Bark
// opening transaction. It intentionally does not infer story completion from
// coordinates: receiving the selected species is the durable postcondition.
func (*Profile) DecodeOpening(reader game.MemoryReader) OpeningFacts {
	if reader == nil {
		return OpeningFacts{}
	}
	ow := (&Profile{}).DecodeOverworld(reader)
	count := int(reader.Peek8(sym.PartyCount))
	if count < 0 {
		count = 0
	}
	if count > 6 {
		count = 6
	}
	party := make([]game.SpeciesID, 0, count)
	for i := 0; i < count; i++ {
		raw := reader.Peek8(sym.PartyMon1 + uint16(i)*sym.PartyMonSize)
		if species, ok := gsdata.Species(raw); ok {
			party = append(party, species)
		}
	}
	return OpeningFacts{
		NativeMapID:  ow.NativeMapID,
		X:            ow.X,
		Y:            ow.Y,
		Facing:       ow.Facing,
		Controllable: ow.Controllable,
		MovementIdle: ow.MovementIdle,
		InBattle:     ow.InBattle,
		ScriptActive: ow.InDialogue,
		Party:        party,
	}
}
