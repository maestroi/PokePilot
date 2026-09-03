package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestPokemonTowerAvailable(t *testing.T) {
	for _, mapID := range []uint8{
		celadonCityMap,
		celadonPokemonCenterMap,
		gameCornerMap,
		rocketHideoutB1FMap,
		rocketHideoutB2FMap,
		rocketHideoutB3FMap,
		rocketHideoutB4FMap,
		route7Map,
		undergroundRoute7Map,
		undergroundWestEastMap,
		undergroundRoute8Map,
		route8Map,
		lavenderTownMap,
		lavenderPokemonCenterMap,
		pokemonTower1FMap,
		pokemonTower2FMap,
		pokemonTower3FMap,
		pokemonTower4FMap,
		pokemonTower5FMap,
		pokemonTower6FMap,
		pokemonTower7FMap,
		mrFujisHouseMap,
	} {
		if !PokemonTowerAvailable(mapID) {
			t.Errorf("PokemonTowerAvailable(%#04x) = false, want true", mapID)
		}
	}

	for _, mapID := range []uint8{0x00, 0x01, 0x0C, 0x36} {
		if PokemonTowerAvailable(mapID) {
			t.Errorf("PokemonTowerAvailable(%#04x) = true outside progression slice", mapID)
		}
	}
}

func TestPokemonTowerMarowakBattle(t *testing.T) {
	marowak := &state.BattleState{Kind: state.BattleWild, EnemySpecies: marowakSpecies}
	if !pokemonTowerMarowakBattle(pokemonTower6FMap, marowak) {
		t.Fatal("mandatory 6F wild Marowak was not classified as story battle")
	}

	cases := []struct {
		name  string
		mapID uint8
		b     *state.BattleState
	}{
		{name: "nil", mapID: pokemonTower6FMap, b: nil},
		{name: "wrong floor", mapID: pokemonTower5FMap, b: marowak},
		{name: "trainer Marowak", mapID: pokemonTower6FMap, b: &state.BattleState{Kind: state.BattleTrainer, EnemySpecies: marowakSpecies}},
		{name: "ordinary Gastly", mapID: pokemonTower6FMap, b: &state.BattleState{Kind: state.BattleWild, EnemySpecies: 0x19}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if pokemonTowerMarowakBattle(tc.mapID, tc.b) {
				t.Fatalf("pokemonTowerMarowakBattle(%#04x, %+v) = true, want false", tc.mapID, tc.b)
			}
		})
	}
}
