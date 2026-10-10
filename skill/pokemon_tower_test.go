package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
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

func TestResolvePokemonTowerBattleOwnsOrdinaryWildThroughFleeThenFight(t *testing.T) {
	// A partial Flee copy that only fell back on ErrTrainerBattle returned
	// errBattleRestarted as a dirty objective boundary (issue #2533). Ordinary
	// Tower wilds must keep Travel's fleeThenFight owner.
	ordinaryCalls, storyCalls := 0, 0
	ordinary := fleeThenFightWith(
		func(int) error { return errBattleRestarted },
		func() (game.BattleResult, error) {
			ordinaryCalls++
			return game.BattleWon, nil
		},
		func() game.BattleKind { return game.BattleWild },
		guaranteedWildFleeAttempts,
	)
	story := func() (battleResolution, error) {
		storyCalls++
		return battleResolution{outcome: game.BattleWon}, nil
	}

	got, err := resolvePokemonTowerBattle(
		pokemonTower6FMap,
		&state.BattleState{Kind: state.BattleWild, EnemySpecies: 0x19},
		ordinary,
		story,
	)
	if err != nil {
		t.Fatalf("ordinary wild: %v", err)
	}
	if ordinaryCalls != 1 || storyCalls != 0 || got.outcome != game.BattleWon || !got.trainer {
		t.Fatalf("ordinary wild resolution=%+v ordinary=%d story=%d; want fleeThenFight fallback", got, ordinaryCalls, storyCalls)
	}

	ordinaryCalls, storyCalls = 0, 0
	got, err = resolvePokemonTowerBattle(
		pokemonTower6FMap,
		&state.BattleState{Kind: state.BattleWild, EnemySpecies: marowakSpecies},
		ordinary,
		story,
	)
	if err != nil {
		t.Fatalf("story Marowak: %v", err)
	}
	if ordinaryCalls != 0 || storyCalls != 1 || got.outcome != game.BattleWon {
		t.Fatalf("Marowak resolution=%+v ordinary=%d story=%d; want forced story fight", got, ordinaryCalls, storyCalls)
	}

	ordinaryCalls, storyCalls = 0, 0
	got, err = resolvePokemonTowerBattle(
		pokemonTower7FMap,
		&state.BattleState{Kind: state.BattleTrainer},
		ordinary,
		story,
	)
	if err != nil {
		t.Fatalf("trainer: %v", err)
	}
	if ordinaryCalls != 0 || storyCalls != 1 {
		t.Fatalf("trainer ordinary=%d story=%d; want forced story fight", ordinaryCalls, storyCalls)
	}

	ordinaryCalls, storyCalls = 0, 0
	_, err = resolvePokemonTowerBattle(pokemonTower6FMap, nil, ordinary, story)
	if err == nil {
		t.Fatal("nil battle: want outside-battle error")
	}
	if ordinaryCalls != 0 || storyCalls != 0 {
		t.Fatalf("nil battle invoked owners ordinary=%d story=%d", ordinaryCalls, storyCalls)
	}
}
