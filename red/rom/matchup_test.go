package rom

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

// Every Gen-I gym and League room resolves its boss party from map objects,
// and the derived matchup names the facts a player would: Electric is
// useless into Giovanni's Ground team and into Brock's Rock/Ground.
func TestChallengeMatchupsComeFromTheROM(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	chart, err := NewTypeChart(romData)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		mapID   uint8
		max     int
		useless game.TypeID
		prefer  game.TypeID
	}{
		{"pewter gym", 0x36, 14, "electric", "water"},
		{"cerulean gym", 0x41, 21, "", "electric"},
		{"viridian gym", 0x2D, 50, "electric", "water"},
		{"lorelei", 0xF5, 56, "", ""},
		{"lance", 0x71, 62, "", "ice"},
	}
	for _, tc := range cases {
		opps, ok, err := MapChallengeOpponents(romData, tc.mapID)
		if err != nil || !ok {
			t.Fatalf("%s: opponents ok=%v err=%v", tc.name, ok, err)
		}
		m := game.AssessMatchup(chart, opps)
		if m.MaxLevel != tc.max {
			t.Errorf("%s: max level %d, want %d (%+v)", tc.name, m.MaxLevel, tc.max, opps)
		}
		if tc.useless != "" && !m.IsUseless(tc.useless) {
			t.Errorf("%s: %s not useless: %+v", tc.name, tc.useless, m)
		}
		if tc.prefer != "" && !m.Prefers(tc.prefer) {
			t.Errorf("%s: %s not preferred: %+v", tc.name, tc.prefer, m)
		}
	}
}
