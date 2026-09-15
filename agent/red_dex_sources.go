package agent

const dexRequirementSaffronGateOpen = "saffron_gate_open"

// Scripted Red acquisitions that are not sitting in a ROM table: starters,
// fossils, gifts, static encounters, and the mutually exclusive choices
// those scripts create. Evolution and wild/fish/trade tables stay ROM-owned.
type scriptedDexSource struct {
	internal uint8
	source   DexSource
}

func redScriptedSources() []scriptedDexSource {
	return []scriptedDexSource{
		{0xB0, DexSource{Kind: AcquireStarter, ExclusiveGroup: "starter"}},                                    // Charmander
		{0xB1, DexSource{Kind: AcquireStarter, ExclusiveGroup: "starter"}},                                    // Squirtle
		{0x99, DexSource{Kind: AcquireStarter, ExclusiveGroup: "starter"}},                                    // Bulbasaur
		{0x62, DexSource{Kind: AcquireFossil, ExclusiveGroup: "mt_moon_fossil", Requirement: "helix_fossil"}}, // Omanyte
		{0x5A, DexSource{Kind: AcquireFossil, ExclusiveGroup: "mt_moon_fossil", Requirement: "dome_fossil"}},  // Kabuto
		{0xAB, DexSource{Kind: AcquireFossil, Requirement: "old_amber"}},                                      // Aerodactyl
		{0x66, DexSource{Kind: AcquireGift, Place: "celadon mansion eevee"}},                                  // Eevee
		{0x13, DexSource{Kind: AcquireGift, Place: "silph co lapras", Requirement: "card_key"}},               // Lapras
		// Hitmonlee
		{0x2B, DexSource{
			Kind: AcquireGift, Place: "fighting dojo hitmonlee",
			Requirement: dexRequirementSaffronGateOpen, ExclusiveGroup: "fighting_dojo",
		}},
		// Hitmonchan
		{0x2C, DexSource{
			Kind: AcquireGift, Place: "fighting dojo hitmonchan",
			Requirement: dexRequirementSaffronGateOpen, ExclusiveGroup: "fighting_dojo",
		}},
		{0xAA, DexSource{Kind: AcquireGift, Requirement: "game_corner"}},  // Porygon
		{0x84, DexSource{Kind: AcquireStatic, Requirement: "poke_flute"}}, // Snorlax
		{0x4A, DexSource{Kind: AcquireStatic}},                            // Articuno
		{0x4B, DexSource{Kind: AcquireStatic}},                            // Zapdos
		{0x49, DexSource{Kind: AcquireStatic}},                            // Moltres
		{0x83, DexSource{Kind: AcquireStatic}},                            // Mewtwo
	}
}

type exclusiveChoice struct {
	Group        string
	Alternatives [][]SpeciesID
}

func redExclusiveChoices() []exclusiveChoice {
	return []exclusiveChoice{
		{
			Group: "starter",
			Alternatives: [][]SpeciesID{
				{"charmander", "charmeleon", "charizard"},
				{"squirtle", "wartortle", "blastoise"},
				{"bulbasaur", "ivysaur", "venusaur"},
			},
		},
		{
			Group: "mt_moon_fossil",
			Alternatives: [][]SpeciesID{
				{"omanyte", "omastar"},
				{"kabuto", "kabutops"},
			},
		},
		{
			Group: "fighting_dojo",
			Alternatives: [][]SpeciesID{
				{"hitmonlee"},
				{"hitmonchan"},
			},
		},
		{
			// Red has one one-time Eevee gift. Pokédex ownership of Eevee is
			// historical and stays set after it evolves, so it cannot be used as
			// evidence that another Eevee individual still exists. Once one of
			// these three branches is owned, the other two are forfeited for a
			// native single-save completion run.
			Group: "eevee_stone",
			Alternatives: [][]SpeciesID{
				{"flareon"},
				{"jolteon"},
				{"vaporeon"},
			},
		},
	}
}

func redEventOnly() map[SpeciesID]bool {
	return map[SpeciesID]bool{"mew": true}
}

func forfeitedSpecies(owned map[SpeciesID]bool, choices []exclusiveChoice) map[SpeciesID]string {
	out := map[SpeciesID]string{}
	for _, choice := range choices {
		chosen := -1
		for i, alt := range choice.Alternatives {
			if intersects(owned, alt) {
				chosen = i
				break
			}
		}
		if chosen < 0 {
			continue
		}
		for i, alt := range choice.Alternatives {
			if i == chosen {
				continue
			}
			for _, id := range alt {
				out[id] = choice.Group
			}
		}
	}
	return out
}

func intersects(owned map[SpeciesID]bool, ids []SpeciesID) bool {
	for _, id := range ids {
		if owned[id] {
			return true
		}
	}
	return false
}
