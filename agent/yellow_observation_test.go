package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
)

// A Yellow ROM must produce a semantic observation through the same generic
// path the run uses. This exercises ObserveChecked end to end against live
// Yellow RAM from the boot fixture.
func TestYellowObserve(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	state, err := os.ReadFile("../skill/failure/yellow_overworld.state")
	if err != nil {
		t.Skip("yellow_overworld.state not available")
	}
	m, err := emu.OpenCGBBytes(romData)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.LoadState(state); err != nil {
		t.Fatal(err)
	}
	obs, err := ObserveChecked(m, romData)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("GameID=%s map=0x%02x (%s) x=%d y=%d money=%d party=%d",
		obs.GameID, obs.Map, obs.MapName, obs.X, obs.Y,
		obs.Money, obs.PartyCount)
	if obs.GameID == "" {
		t.Error("empty GameID")
	}
	if obs.Map != 0x26 {
		t.Errorf("map = 0x%02x, want 0x26", obs.Map)
	}
}
