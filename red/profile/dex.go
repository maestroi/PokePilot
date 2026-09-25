package profile

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const dexRequirementSaffronGateOpen = "saffron_gate_open"

// BuildDexCatalog makes attainable Dex content a profile capability. ROM-backed
// encounter/evolution/trade data and Red/Blue scripted one-time sources stay on
// the game side of the profile boundary; generic planner policy sees only
// semantic acquisition methods and availability.
func (*Profile) BuildDexCatalog(romData []byte, owned, seen []game.SpeciesID) (game.DexCatalog, error) {
	sources, err := collectDexSources(romData)
	if err != nil {
		return game.DexCatalog{}, err
	}
	entries, err := dexSpecies(romData)
	if err != nil {
		return game.DexCatalog{}, err
	}
	return game.AssembleDexCatalog(entries, sources, owned, seen, redExclusiveChoices(), redEventOnly()), nil
}

func projectPokedex(romData []byte, dex state.PokedexState) (owned, seen []game.SpeciesID) {
	return projectDexList(romData, dex.Owned), projectDexList(romData, dex.Seen)
}

func projectDexList(romData []byte, numbers []uint8) []game.SpeciesID {
	out := make([]game.SpeciesID, 0, len(numbers))
	for _, n := range numbers {
		internal, err := rom.DexNumberInternalSpecies(romData, n)
		if err != nil {
			continue
		}
		id, ok := reddata.Species(internal)
		if !ok || id == "" || id == "unknown" {
			continue
		}
		out = append(out, id)
	}
	return out
}

func dexSpecies(romData []byte) ([]game.DexEntry, error) {
	out := make([]game.DexEntry, 0, sym.PokedexCount)
	for dex := uint16(1); dex <= uint16(sym.PokedexCount); dex++ {
		internal, err := rom.DexNumberInternalSpecies(romData, uint8(dex))
		if err != nil {
			continue
		}
		id, ok := reddata.Species(internal)
		if !ok || id == "" || id == "unknown" {
			continue
		}
		out = append(out, game.DexEntry{Species: id, Dex: dex})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("red profile: dex catalog: ROM maps no Pokedex species")
	}
	return out, nil
}

func collectDexSources(romData []byte) (map[game.SpeciesID][]game.DexSource, error) {
	out := map[game.SpeciesID][]game.DexSource{}
	add := func(internal uint8, src game.DexSource) {
		id, ok := reddata.Species(internal)
		if !ok || id == "" || id == "unknown" {
			return
		}
		out[id] = append(out[id], src)
	}

	wild, err := rom.WildEncounters(romData)
	if err != nil {
		return nil, err
	}
	for _, enc := range wild {
		kind := game.AcquireWildGrass
		req := ""
		if enc.Habitat == rom.HabitatWater {
			kind = game.AcquireWildWater
			req = "surf"
		}
		if dexSafariRequirement(enc.MapID) {
			req = dexJoinReq(req, "safari_zone")
		}
		add(enc.Species, game.DexSource{
			Kind:        kind,
			Place:       dexMapPlace(enc.MapID),
			Level:       enc.Level,
			Requirement: req,
		})
	}

	fish, err := rom.FishingEncounters(romData)
	if err != nil {
		return nil, err
	}
	for _, enc := range fish {
		src := game.DexSource{
			Kind:        game.AcquireFishing,
			Level:       enc.Level,
			Requirement: enc.Rod + "_rod",
		}
		if enc.MapID != 0 {
			src.Place = dexMapPlace(enc.MapID)
		}
		if dexSafariRequirement(enc.MapID) {
			src.Requirement = dexJoinReq(src.Requirement, "safari_zone")
		}
		add(enc.Species, src)
	}

	evos, err := rom.Evolutions(romData)
	if err != nil {
		return nil, err
	}
	for _, evo := range evos {
		from, _ := reddata.Species(evo.From)
		src := game.DexSource{From: from, Level: evo.Level}
		switch evo.Method {
		case rom.EvoLevel:
			src.Kind = game.AcquireLevelEvo
		case rom.EvoItem:
			src.Kind = game.AcquireItemEvo
			src.Item = dexEvolutionItem(evo.Item)
		case rom.EvoTrade:
			src.Kind = game.AcquireTradeEvo
		default:
			continue
		}
		add(evo.To, src)
	}

	trades, err := rom.NPCTrades(romData)
	if err != nil {
		return nil, err
	}
	for _, tr := range trades {
		give, _ := reddata.Species(tr.Give)
		add(tr.Get, game.DexSource{Kind: game.AcquireInGameTrade, Give: give})
	}

	for _, src := range redScriptedSources() {
		add(src.internal, src.source)
	}
	return out, nil
}

type scriptedDexSource struct {
	internal uint8
	source   game.DexSource
}

func redScriptedSources() []scriptedDexSource {
	return []scriptedDexSource{
		{0xB0, game.DexSource{Kind: game.AcquireStarter, ExclusiveGroup: "starter"}},
		{0xB1, game.DexSource{Kind: game.AcquireStarter, ExclusiveGroup: "starter"}},
		{0x99, game.DexSource{Kind: game.AcquireStarter, ExclusiveGroup: "starter"}},
		{0x62, game.DexSource{Kind: game.AcquireFossil, ExclusiveGroup: "mt_moon_fossil", Requirement: "helix_fossil"}},
		{0x5A, game.DexSource{Kind: game.AcquireFossil, ExclusiveGroup: "mt_moon_fossil", Requirement: "dome_fossil"}},
		{0xAB, game.DexSource{Kind: game.AcquireFossil, Requirement: "old_amber"}},
		{0x66, game.DexSource{Kind: game.AcquireGift, Place: "celadon mansion eevee"}},
		{0x13, game.DexSource{Kind: game.AcquireGift, Place: "silph co lapras", Requirement: "card_key"}},
		{0x2B, game.DexSource{Kind: game.AcquireGift, Place: "fighting dojo hitmonlee", Requirement: dexRequirementSaffronGateOpen, ExclusiveGroup: "fighting_dojo"}},
		{0x2C, game.DexSource{Kind: game.AcquireGift, Place: "fighting dojo hitmonchan", Requirement: dexRequirementSaffronGateOpen, ExclusiveGroup: "fighting_dojo"}},
		{0xAA, game.DexSource{Kind: game.AcquireGift, Requirement: "game_corner"}},
		{0x84, game.DexSource{Kind: game.AcquireStatic, Requirement: "poke_flute"}},
		{0x4A, game.DexSource{Kind: game.AcquireStatic}},
		{0x4B, game.DexSource{Kind: game.AcquireStatic}},
		{0x49, game.DexSource{Kind: game.AcquireStatic}},
		{0x83, game.DexSource{Kind: game.AcquireStatic}},
	}
}

func redExclusiveChoices() []game.DexExclusiveChoice {
	return []game.DexExclusiveChoice{
		{Group: "starter", Alternatives: [][]game.SpeciesID{
			{"charmander", "charmeleon", "charizard"},
			{"squirtle", "wartortle", "blastoise"},
			{"bulbasaur", "ivysaur", "venusaur"},
		}},
		{Group: "mt_moon_fossil", Alternatives: [][]game.SpeciesID{
			{"omanyte", "omastar"},
			{"kabuto", "kabutops"},
		}},
		{Group: "fighting_dojo", Alternatives: [][]game.SpeciesID{{"hitmonlee"}, {"hitmonchan"}}},
		{Group: "eevee_stone", Alternatives: [][]game.SpeciesID{{"flareon"}, {"jolteon"}, {"vaporeon"}}},
	}
}

func redEventOnly() map[game.SpeciesID]bool {
	return map[game.SpeciesID]bool{"mew": true}
}

func dexMapPlace(mapID uint8) game.PlaceID {
	name := state.MapName(mapID)
	if name == "" {
		return ""
	}
	return semanticLocation(name)
}

func dexSafariRequirement(mapID uint8) bool {
	return mapID >= 0xD9 && mapID <= 0xDC
}

func dexJoinReq(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "," + b
	}
}

func dexEvolutionItem(id uint8) game.ItemID {
	if name, ok := reddata.ItemName(id); ok {
		return game.ItemID(name)
	}
	return game.ItemID(fmt.Sprintf("item_%02x", id))
}
