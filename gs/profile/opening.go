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

	GotStarter               bool
	GotMysteryEgg            bool
	HasPokedex               bool
	MrPokemonVisitComplete   bool
	CherrygroveRivalResolved bool
	RivalNamed               bool
	GaveMysteryEggToElm      bool
	RivalNamePrompt          bool
	RivalName                string
	BattleMode               uint8
	BattleResult             uint8
	ElmsLabScene             uint8
	CherrygroveCityScene     uint8
	MrPokemonsHouseScene     uint8

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
func (p *Profile) DecodeOpening(reader game.MemoryReader) OpeningFacts {
	if reader == nil {
		return OpeningFacts{}
	}
	ow := p.DecodeOverworld(reader)
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
	story := decodeEarlyStory(reader)
	rivalRaw := readGSBytes(reader, sym.RivalName, sym.RivalNameLen)
	rivalPrompt := reader.Peek8(sym.NamingScreenType) == 2 &&
		len(rivalRaw) > 0 && (rivalRaw[0] == 0xf2 || rivalRaw[0] == 0xeb)
	return OpeningFacts{
		NativeMapID:              ow.NativeMapID,
		X:                        ow.X,
		Y:                        ow.Y,
		Facing:                   ow.Facing,
		Controllable:             ow.Controllable,
		MovementIdle:             ow.MovementIdle,
		InBattle:                 ow.InBattle,
		ScriptActive:             gsScriptActive(reader),
		GotStarter:               story.StarterReceived,
		GotMysteryEgg:            story.MysteryEggReceived,
		HasPokedex:               story.PokedexAcquired,
		MrPokemonVisitComplete:   story.MrPokemonVisitComplete,
		CherrygroveRivalResolved: story.CherrygroveRivalResolved,
		RivalNamed:               story.RivalNamed,
		GaveMysteryEggToElm:      story.MysteryEggReturned,
		RivalNamePrompt:          rivalPrompt,
		RivalName:                decodeGSName(rivalRaw),
		BattleMode:               reader.Peek8(sym.BattleMode),
		BattleResult:             reader.Peek8(sym.BattleResult),
		ElmsLabScene:             reader.Peek8(sym.ElmsLabSceneID),
		CherrygroveCityScene:     reader.Peek8(sym.CherrygroveCitySceneID),
		MrPokemonsHouseScene:     reader.Peek8(sym.MrPokemonsHouseSceneID),
		Party:                    party,
	}
}
