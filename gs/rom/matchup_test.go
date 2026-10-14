package rom

import (
	"slices"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func syntheticGen2ChartROM() []byte {
	rom := make([]byte, 512)
	// Real TypeMatchups prefix, followed by enough valid records for the
	// locator to distinguish a chart from an incidental byte sequence.
	rows := []byte{0, 5, 5, 0, 9, 5}
	for i := 0; i < 70; i++ {
		rows = append(rows, 1, 5, 10)
	}
	rows = append(rows,
		27, 24, 20, // Dark beats Psychic
		24, 27, 0,  // Psychic is immune on Dark
		20, 9, 20,  // Fire beats Steel
		1, 27, 20,  // Fighting beats Dark
		0xff,
	)
	copy(rom[64:], rows)
	return rom
}

func TestGen2TypeChartIsCartridgeBacked(t *testing.T) {
	chart, err := NewTypeChart(syntheticGen2ChartROM())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(chart.AttackTypes(), game.TypeID("dark")) {
		t.Fatalf("dark missing from attacking types: %v", chart.AttackTypes())
	}
	for _, tc := range []struct {
		attack game.TypeID
		defend []game.TypeID
		want   int
	}{
		{"dark", []game.TypeID{"psychic"}, 20},
		{"psychic", []game.TypeID{"dark"}, 0},
		{"fire", []game.TypeID{"steel"}, 20},
		{"normal", []game.TypeID{"steel", "ground"}, 5},
		{"fire", []game.TypeID{"steel", "steel"}, 20},
		{"fire", []game.TypeID{"unknown"}, 10},
	} {
		if got := chart.Effectiveness(tc.attack, tc.defend); got != tc.want {
			t.Errorf("%s against %v = %d, want %d", tc.attack, tc.defend, got, tc.want)
		}
	}
	dark := game.AssessMatchup(chart, []game.ChallengeOpponent{{Species: "umbreon", Level: 42, Types: []game.TypeID{"dark"}}})
	if !dark.Prefers("fighting") || !dark.IsUseless("psychic") {
		t.Fatalf("dark matchup not reflected in challenge readiness: %+v", dark)
	}
	steel := game.AssessMatchup(chart, []game.ChallengeOpponent{{Species: "steelix", Level: 35, Types: []game.TypeID{"steel", "ground"}}})
	if !steel.Prefers("fire") || steel.Prefers("normal") {
		t.Fatalf("steel matchup not reflected in challenge readiness: %+v", steel)
	}
}

func TestGen2SpeciesTypesComeFromBaseData(t *testing.T) {
	rom := tmhmTestROM()
	steelix := testBaseDataOffset + (0xd0-1)*gen2BaseDataEntrySize + 7
	rom[steelix], rom[steelix+1] = 9, 4
	umbreon := testBaseDataOffset + (0xc5-1)*gen2BaseDataEntrySize + 7
	rom[umbreon], rom[umbreon+1] = 27, 27
	for _, tc := range []struct {
		species uint8
		want    []game.TypeID
	}{
		{0xd0, []game.TypeID{"steel", "ground"}},
		{0xc5, []game.TypeID{"dark"}},
	} {
		got, err := SpeciesTypes(rom, tc.species)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Fatalf("SpeciesTypes(%#x) = %v, %v; want %v", tc.species, got, err, tc.want)
		}
	}
	rom[steelix] = 0xff
	if _, err := SpeciesTypes(rom, 0xd0); err == nil {
		t.Fatal("unsupported native type should not become neutral")
	}
}

func TestGoldTypeChartReadsRealDarkSteelRows(t *testing.T) {
	rom := loadGold(t) // Optional fixture; independent synthetic test above always runs.
	chart, err := NewTypeChart(rom)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		attack game.TypeID
		defend []game.TypeID
		want   int
	}{
		{"dark", []game.TypeID{"psychic"}, 20},
		{"psychic", []game.TypeID{"dark"}, 0},
		{"fire", []game.TypeID{"steel"}, 20},
		{"normal", []game.TypeID{"steel"}, 5},
	} {
		if got := chart.Effectiveness(tc.attack, tc.defend); got != tc.want {
			t.Errorf("ROM %s vs %v = %d, want %d", tc.attack, tc.defend, got, tc.want)
		}
	}
}
