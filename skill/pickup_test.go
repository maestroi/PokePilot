package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

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
