package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
)

// openRocketB4FFarmState loads a run-12vowvyawgx0b3jl0srufdx8tq farm state.
// They were saved from the farm's Mewtwo-starter ROM; set POKEPILOT_FARM_ROM.
func openRocketB4FFarmState(t *testing.T, env string) (*emu.Emu, []byte) {
	t.Helper()
	path, farmPath := os.Getenv(env), os.Getenv("POKEPILOT_FARM_ROM")
	if path == "" || farmPath == "" {
		t.Skipf("set %s and POKEPILOT_FARM_ROM", env)
	}
	m := openEmuCGB(t)
	base := append([]byte(nil), m.ROM()...)
	farm, err := os.ReadFile(farmPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoadDerivedROM(base, farm, "farm"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}
	return m, base
}

// Round 57: the Lift Key appears only when the beaten Rocket is spoken to
// again, on the tile beside him. The skill never re-talked, and a re-talk from
// that tile spawned the key under Red.
// POKEPILOT_ROCKET_LIFT_KEY_STATE is failure-frame-0000527426-progress-silph-scope-acquired.state.
func TestRocketLiftKeyRevealRealROM(t *testing.T) {
	m, base := openRocketB4FFarmState(t, "POKEPILOT_ROCKET_LIFT_KEY_STATE")
	if err := acquireRocketLiftKey(m, base, StatAwareMove(base)); err != nil {
		t.Fatalf("acquireRocketLiftKey: %v", err)
	}
	if !rocketBagHas(m, liftKeyItem) {
		t.Fatal("Lift Key not in bag")
	}
}

// Round 61: the boss room shares no walkable path with B4F's stair, so leaving
// for the Tower must ride the elevator.
// POKEPILOT_ROCKET_BOSS_EXIT_STATE is failure-frame-0000576115-progress-poke-flute-acquired.state.
func TestRocketBossRoomExitRealROM(t *testing.T) {
	m, base := openRocketB4FFarmState(t, "POKEPILOT_ROCKET_BOSS_EXIT_STATE")
	if err := leaveRocketHideoutForTower(m, base, StatAwareMove(base)); err != nil {
		t.Fatalf("leaveRocketHideoutForTower: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != gameCornerMap {
		t.Fatalf("after leaving map=%#02x, want Game Corner %#02x", got, gameCornerMap)
	}
}
