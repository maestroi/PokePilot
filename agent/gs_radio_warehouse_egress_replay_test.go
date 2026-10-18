package agent

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// TestGSRadioShutterLeavesWarehouseByElevatorReplay replays farm run
// run-a018a6cmhnkr round 3 (triage 781cdee33ef70f00): the Card Key holder
// stands in the sealed switch-room pocket at (22,11), whose only exit is the
// warehouse -> Dept Store B1F -> elevator (choose 1F). Keep the private state
// outside Git; supply its path as POKEPILOT_GS_RADIO_SHUTTER_STATE.
func TestGSRadioShutterLeavesWarehouseByElevatorReplay(t *testing.T) {
	path, romPath := os.Getenv("POKEPILOT_GS_RADIO_SHUTTER_STATE"), os.Getenv("POKEMON_GOLD_ROM")
	if testing.Short() || path == "" || romPath == "" {
		t.Skip("requires private Radio Tower shutter state and Gold ROM")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != "c3cc4782f428bd49341e5749643816d6fef7afe5f9d73aaabafffd12764d7c39" {
		t.Fatalf("unexpected replay state: %s", got)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}
	if err := executeGSRadioTowerShutter(m, m.ROM()); err != nil {
		t.Fatal(err)
	}
	profile := gsprofile.NewGold()
	shutter3F, err := gsOpeningMapID("RADIO_TOWER_3F")
	if err != nil {
		t.Fatal(err)
	}
	world := profile.DecodeOverworld(m)
	if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressRadioTowerShutterOpened) ||
		world.NativeMapID != shutter3F || !world.Controllable {
		t.Fatalf("shutter not proven open at a stable boundary: map=%#04x (%d,%d) controllable=%v",
			world.NativeMapID, world.X, world.Y, world.Controllable)
	}
}
