package skill_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestPickupTrainerInterceptionReplay is the exact round-1 checkpoint from
// run-jxh8lk19wv6on. The saved state is an external farm artifact, never a
// checked-in fixture. Set PICKUP_TRAINER_REPRO_STATE to its downloaded path.
// The trainer intercepts the first standing tile; Pickup must reselect a live
// side and collect the item instead of facing it diagonally.
func TestPickupTrainerInterceptionReplay(t *testing.T) {
	statePath := os.Getenv("PICKUP_TRAINER_REPRO_STATE")
	if statePath == "" {
		t.Skip("set PICKUP_TRAINER_REPRO_STATE to the run-jxh8lk19wv6on round-1 .state artifact")
	}
	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	const stateSHA = "407836f33f5d9a8e9831637ec832c799e66b4259d4a8c3a3e57b52a8c6e613f8"
	if got := fmt.Sprintf("%x", sha256.Sum256(stateBytes)); got != stateSHA {
		t.Fatalf("replay state SHA-256 = %s, want %s", got, stateSHA)
	}
	m, err := emu.Open(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		t.Fatal(err)
	}
	const fullHeal = 0x34
	before := bagQty(t, m, fullHeal)
	if err := skill.Pickup(m, m.ROM(), 18, 9, fullHeal, skill.StatAwareMove(m.ROM())); err != nil {
		t.Fatal(err)
	}
	if after := bagQty(t, m, fullHeal); after != before+1 {
		t.Fatalf("FULL HEAL count = %d, want %d", after, before+1)
	}
}

// TestPickupPokeBall is the S7-6 postcondition: from the post-errand state
// (Viridian City), travel into Viridian Forest and take the POKE BALL at
// (1,31) on map 0x33;
// the bag's POKE BALL count must rise by exactly one. The approach leg ends
// on (2,31), the walkable tile directly east of the ball, so Pickup itself
// only has to face and press A. It is a full journey — minutes of emulation
// and stochastic wild battles — so it runs only outside -short.
func TestPickupPokeBall(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; not part of the -short gate")
	}
	// post_errand stands in Viridian City, the same start TestGymBoulderBadge
	// uses for this city -> Route 2 -> forest crossing.
	m := fixture.Load(t, "post_errand")
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	before := bagQty(t, m, skill.ItemPokeBall)

	forest, ok := skill.Place("viridian forest")
	if !ok {
		t.Fatal(`Place "viridian forest" not found`)
	}
	if _, err := fixture.Travel(m, forest, policy, 20); err != nil {
		t.Fatalf("Travel to the forest: %v", err)
	}

	// (2,31) is the walkable tile east of the ball at (1,31), measured off
	// the ROM grid. Travel, not GoTo: the leg crosses tall grass and a wild
	// battle there is an ordinary outcome, not a failure.
	if _, err := fixture.Travel(m, skill.Destination{Map: 0x33, X: 2, Y: 31}, policy, 20); err != nil {
		t.Fatalf("Travel to (2,31): %v", err)
	}

	if err := skill.Pickup(m, romData, 1, 31, skill.ItemPokeBall, policy); err != nil {
		t.Fatalf("Pickup: %v", err)
	}

	if after := bagQty(t, m, skill.ItemPokeBall); after != before+1 {
		t.Fatalf("postcondition: POKE BALL count = %d, want %d (before=%d)", after, before+1, before)
	}
}
