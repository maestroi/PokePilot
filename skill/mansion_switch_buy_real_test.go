package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestEnsureItemStockRecoversMansionSwitchSealRealROM is the deterministic
// regression for run-3dxv0agt5di8x1ddvib3x0enk5 (triage d3a8d73bd07ae38d):
// buy hyper potions from a switch-sealed Mansion B1F pocket after Secret Key.
//
// POKEPILOT_MANSION_SWITCH_BUY_STATE is
// round-021-frame-0002038052-buy-2-HYPER-POTION.state from that run.
func TestEnsureItemStockRecoversMansionSwitchSealRealROM(t *testing.T) {
	path := os.Getenv("POKEPILOT_MANSION_SWITCH_BUY_STATE")
	romPath := os.Getenv("POKEMON_BLUE_ROM")
	if romPath == "" {
		romPath = os.Getenv("POKEPILOT_FARM_ROM")
	}
	if path == "" || romPath == "" {
		t.Skip("set POKEPILOT_MANSION_SWITCH_BUY_STATE and POKEMON_BLUE_ROM")
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
	if got := m.Peek8(sym.CurMap); got != pokemonMansionB1FMap {
		t.Fatalf("prepared state on map %#02x, want Mansion B1F %#02x", got, pokemonMansionB1FMap)
	}
	if mansionTileReachable(m, romData, mansionB1FExitX, mansionB1FExitY) {
		t.Fatal("prepared state already reaches B1F stairs; seal regression is not exercised")
	}

	have, err := EnsureItemStock(m, romData, StatAwareMove(romData), itemHyperPotion, 2, 1)
	if err != nil {
		t.Fatalf("EnsureItemStock from sealed Mansion B1F: %v", err)
	}
	if have < 1 {
		t.Fatalf("hyper potion count = %d, want at least 1", have)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if got := mem.U8(sym.CurMap); got != 0xac {
		t.Fatalf("after EnsureItemStock map=%#02x, want Cinnabar mart %#02x", got, 0xac)
	}
}
