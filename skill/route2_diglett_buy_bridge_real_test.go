package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestEnsureItemStockCutsOutOfDiglettsCavePocket pins triage 32aba985e5980b75
// (issue #2654, run-1q3fqxduf06zc3jt9rsfjcqzjt attempt 4): buy 1 POTION from
// Diglett's Cave exited onto Route 2's Diglett pocket (12,10), where every
// cross-map route reported the remote can_clear_snorlax gate as a
// RouteBlockedError. GoTo returned that blockage without trying the Cut tree
// that opens the pocket toward Viridian, although Cut was usable.
//
// POKEPILOT_DIGLETT_BUY_STATE is that attempt's
// round-422-frame-0002782847-buy-1-POTION.state, saved from Yellow.
func TestEnsureItemStockCutsOutOfDiglettsCavePocket(t *testing.T) {
	path, romPath := os.Getenv("POKEPILOT_DIGLETT_BUY_STATE"), os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" || romPath == "" {
		t.Skip("set POKEPILOT_DIGLETT_BUY_STATE and POKEMON_YELLOW_ROM")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}
	const diglettsCaveMap, pewterMartMap = 0xc5, 0x38
	if got := m.Peek8(sym.CurMap); got != diglettsCaveMap {
		t.Fatalf("prepared state on map %#02x, want Diglett's Cave %#02x", got, diglettsCaveMap)
	}

	have, err := EnsureItemStock(m, romData, StatAwareMove(romData), 0x14, 1, 1)
	if err != nil {
		t.Fatalf("EnsureItemStock POTION from Diglett's Cave: %v", err)
	}
	if have < 1 {
		t.Fatalf("POTION count = %d, want at least 1", have)
	}
	if got := m.Peek8(sym.CurMap); got != viridianMartMap && got != pewterMartMap {
		t.Fatalf("after EnsureItemStock map=%#02x, want Viridian Mart %#02x or Pewter Mart %#02x",
			got, viridianMartMap, pewterMartMap)
	}
}
