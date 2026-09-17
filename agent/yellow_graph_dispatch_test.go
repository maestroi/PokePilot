package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/profiles"
)

// The run path must build Yellow's graph with Yellow's tables, not Red's.
// This is the whole point of buildMapGraph: a Yellow ROM detected by the
// profile registry must dispatch to the Yellow provider. Red's tables on a
// Yellow ROM parse only 33 of 249 maps, which fails this test loudly.
func TestYellowGraphDispatch(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	profile, _, err := profiles.Detect(romData)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("detected: %s @ %s", profile.ID(), profile.Revision())

	g, err := buildMapGraph(profile, romData)
	if err != nil {
		t.Fatal(err)
	}
	n := len(g.Edges)
	if n < 200 {
		t.Errorf("Yellow graph has %d nodes; Red tables would have been used", n)
	}
	t.Logf("buildMapGraph produced %d nodes", n)

	// The decisive check: the node count must differ from what the Red
	// default produces on this same ROM.
	redStyle, _ := buildMapGraph(nil, romData)
	t.Logf("(Red-default tables on this ROM: %d nodes)", len(redStyle.Edges))
	if len(redStyle.Edges) == n {
		t.Error("dispatch produced the same node count as Red's default tables; provider not selected")
	}

	// SUMMER_BEACH_HOUSE is Yellow-only, so its presence is direct evidence
	// Yellow's tables were used rather than Red's.
	if _, ok := g.Edges[0xF8]; !ok {
		t.Error("SUMMER_BEACH_HOUSE (0xF8) missing: Yellow tables were not used")
	}
}
