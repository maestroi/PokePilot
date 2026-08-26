package skill_test

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestGetParcel is S5b-2: from the post-starter state, collect Oak's parcel
// from the Viridian Mart and return with the parcel flag set and the parcel
// in the bag. It does not deliver the parcel to Oak; that is the next task,
// so EVENT_GOT_POKEDEX is left unset.
func TestGetParcel(t *testing.T) {
	m := fixture.Load(t, "post_starter")
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.HasEvent(&mem, state.EventGotOaksParcel) {
		t.Fatal("precondition: parcel flag already set in post_starter fixture")
	}
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == skill.ItemOaksParcel {
			t.Fatalf("precondition: parcel already in bag: %+v", state.DecodeInventory(&mem).Items)
		}
	}

	start := time.Now()
	if err := skill.GetParcel(m, romData, policy); err != nil {
		t.Fatalf("GetParcel: %v", err)
	}
	t.Logf("GetParcel took %v", time.Since(start))

	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventGotOaksParcel) {
		t.Fatal("postcondition: parcel flag not set")
	}
	bag := state.DecodeInventory(&mem)
	parcel := false
	for _, it := range bag.Items {
		if it.ID == skill.ItemOaksParcel {
			parcel = true
			if it.Quantity != 1 {
				t.Fatalf("postcondition: parcel quantity %d, want 1", it.Quantity)
			}
		}
	}
	if !parcel {
		t.Fatalf("postcondition: no OAK's PARCEL in bag: %+v", bag.Items)
	}
	if !state.Controllable(&mem) {
		t.Fatalf("postcondition: player not controllable: %+v", state.DecodePlayer(&mem))
	}
	p := state.DecodePlayer(&mem)
	if p.MapID != 0x2A || p.X != 2 || p.Y != 5 {
		t.Fatalf("postcondition: expected (2,5) on map 0x2a, got (%d,%d) on %#04x", p.X, p.Y, p.MapID)
	}
}
