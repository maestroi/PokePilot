package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/gen1rom"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

const yellowDexIncompleteReason = "yellow evolution/fishing/trade sources not yet modeled"

func buildYellowDexCatalog(romData []byte, owned, seen []SpeciesID) (DexCatalog, error) {
	entries, err := yellowDexSpecies(romData)
	if err != nil {
		return DexCatalog{}, err
	}
	sources, err := yellowDexSources(romData)
	if err != nil {
		return DexCatalog{}, err
	}
	cat := assembleDexCatalogWithPolicy(entries, sources, owned, seen, yellowExclusiveChoices(), yellowEventOnly())
	cat.IncompleteReason = yellowDexIncompleteReason
	return cat, nil
}

func yellowDexSpecies(romData []byte) ([]DexEntry, error) {
	out := make([]DexEntry, 0, 151)
	for dex := 1; dex <= 151; dex++ {
		raw, err := yellowrom.DexNumberInternalSpecies(romData, uint8(dex))
		if err != nil {
			return nil, fmt.Errorf("agent: Yellow Dex #%d: %w", dex, err)
		}
		id, ok := gen1.Species(raw)
		if !ok {
			return nil, fmt.Errorf("agent: Yellow Dex #%d maps to unknown internal species %#02x", dex, raw)
		}
		out = append(out, DexEntry{Species: SpeciesID(id), Dex: uint8(dex)})
	}
	return out, nil
}

func yellowDexSources(romData []byte) (map[SpeciesID][]DexSource, error) {
	out := map[SpeciesID][]DexSource{}
	addRaw := func(raw uint8, src DexSource) {
		id, ok := gen1.Species(raw)
		if !ok {
			return
		}
		out[SpeciesID(id)] = append(out[SpeciesID(id)], src)
	}

	wild, err := yellowrom.WildEncounters(romData)
	if err != nil {
		return nil, err
	}
	for _, enc := range wild {
		id, ok := gen1.Species(enc.Species)
		if !ok {
			continue
		}
		kind, req := AcquireWildGrass, ""
		if enc.Habitat == gen1rom.HabitatWater {
			kind, req = AcquireWildWater, "surf"
		}
		if yellowSafariRequirement(enc.MapID) {
			req = joinReq(req, "safari_zone")
		}
		name := yellowrom.MapName(enc.MapID)
		place := PlaceID("")
		if name != "" {
			place = PlaceID(semanticLocation(name))
		}
		out[SpeciesID(id)] = append(out[SpeciesID(id)], DexSource{
			Kind: kind, Place: place, Level: enc.Level, Requirement: req,
		})
	}

	for _, scripted := range yellowScriptedDexSources() {
		addRaw(scripted.internal, scripted.source)
	}
	return out, nil
}

func yellowSafariRequirement(mapID uint8) bool {
	return mapID >= 0xd9 && mapID <= 0xdc
}

func yellowScriptedDexSources() []scriptedDexSource {
	return []scriptedDexSource{
		{0x54, DexSource{Kind: AcquireStarter}}, // Pikachu
		// Yellow makes the other Kanto starters independent gifts instead of a
		// mutually-exclusive Oak starter choice.
		{0x99, DexSource{Kind: AcquireGift, Place: "cerulean melanies house", Requirement: "yellow_pikachu_happiness_147"}},
		{0xb0, DexSource{Kind: AcquireGift, Place: "route 24"}},
		{0xb1, DexSource{Kind: AcquireGift, Place: "vermilion city", Requirement: "thunder_badge"}},

		{0x62, DexSource{Kind: AcquireFossil, ExclusiveGroup: "mt_moon_fossil", Requirement: "helix_fossil"}},
		{0x5a, DexSource{Kind: AcquireFossil, ExclusiveGroup: "mt_moon_fossil", Requirement: "dome_fossil"}},
		{0xab, DexSource{Kind: AcquireFossil, Requirement: "old_amber"}},
		{0x66, DexSource{Kind: AcquireGift, Place: "celadon mansion eevee"}},
		{0x13, DexSource{Kind: AcquireGift, Place: "silph co lapras", Requirement: "card_key"}},
		{0x2b, DexSource{Kind: AcquireGift, Place: "fighting dojo hitmonlee", Requirement: dexRequirementSaffronGateOpen, ExclusiveGroup: "fighting_dojo"}},
		{0x2c, DexSource{Kind: AcquireGift, Place: "fighting dojo hitmonchan", Requirement: dexRequirementSaffronGateOpen, ExclusiveGroup: "fighting_dojo"}},
		{0xaa, DexSource{Kind: AcquireGift, Requirement: "game_corner"}},
		{0x84, DexSource{Kind: AcquireStatic, Requirement: "poke_flute"}},
		{0x4a, DexSource{Kind: AcquireStatic}},
		{0x4b, DexSource{Kind: AcquireStatic}},
		{0x49, DexSource{Kind: AcquireStatic}},
		{0x83, DexSource{Kind: AcquireStatic}},
	}
}

func yellowExclusiveChoices() []exclusiveChoice {
	return []exclusiveChoice{
		{
			Group: "mt_moon_fossil",
			Alternatives: [][]SpeciesID{
				{"omanyte", "omastar"},
				{"kabuto", "kabutops"},
			},
		},
		{
			Group:        "fighting_dojo",
			Alternatives: [][]SpeciesID{{"hitmonlee"}, {"hitmonchan"}},
		},
		{
			Group:        "eevee_stone",
			Alternatives: [][]SpeciesID{{"flareon"}, {"jolteon"}, {"vaporeon"}},
		},
	}
}


func yellowEventOnly() map[SpeciesID]bool {
	return map[SpeciesID]bool{"mew": true}
}
