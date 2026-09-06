package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestBillsPCRoundTrip is ROM-backed coverage for the reusable PC primitives.
// It starts after Oak's Poké Balls, catches one common Route 1 species so the
// party has a depositable partner, stores that exact slot, then withdraws the
// appended boxed Pokémon and verifies the original party shape is restored.
func TestBillsPCRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("emulator journey test")
	}
	m := fixture.Load(t, "post_pokeballs")
	policy := skill.StatAwareMove(m.ROM())
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal(`Place "route 1" missing`)
	}
	if _, err := skill.TravelFlee(m, m.ROM(), route1, policy, 20); err != nil {
		t.Fatalf("reach Route 1: %v", err)
	}

	wild, err := skill.WildGrass(m.ROM(), route1.Map)
	if err != nil || len(wild) == 0 {
		t.Fatalf("Route 1 WildGrass: %v entries=%d", err, len(wild))
	}
	want := make([]uint8, 0, len(wild))
	for _, w := range wild {
		want = append(want, w.ID)
	}
	caught, err := skill.Catch(m, m.ROM(), want, policy, 5)
	if err != nil {
		t.Fatalf("catch a Route 1 partner: %v", err)
	}
	if caught.Outcome != skill.OutcomeCaught {
		t.Fatalf("catch outcome = %d, want caught", caught.Outcome)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	partyBefore := state.DecodeParty(&before)
	boxBefore := state.DecodeBox(&before)
	if partyBefore.Count < 2 {
		t.Fatalf("party count after catch = %d, want >=2", partyBefore.Count)
	}
	depositSlot := int(partyBefore.Count) - 1
	wantSpecies := partyBefore.Mons[depositSlot].Species

	if err := skill.DepositPartyMon(m, m.ROM(), policy, depositSlot); err != nil {
		t.Fatalf("DepositPartyMon(%d): %v", depositSlot, err)
	}
	var stored state.Mem
	state.Snapshot(m, &stored)
	partyStored, boxStored := state.DecodeParty(&stored), state.DecodeBox(&stored)
	if partyStored.Count != partyBefore.Count-1 || boxStored.Count != boxBefore.Count+1 {
		t.Fatalf("deposit counts party %d->%d box %d->%d", partyBefore.Count, partyStored.Count, boxBefore.Count, boxStored.Count)
	}
	withdrawIndex := int(boxStored.Count) - 1
	if err := skill.WithdrawBoxMon(m, m.ROM(), policy, withdrawIndex); err != nil {
		t.Fatalf("WithdrawBoxMon(%d): %v", withdrawIndex, err)
	}
	var after state.Mem
	state.Snapshot(m, &after)
	partyAfter, boxAfter := state.DecodeParty(&after), state.DecodeBox(&after)
	if partyAfter.Count != partyBefore.Count || boxAfter.Count != boxBefore.Count {
		t.Fatalf("roundtrip counts party %d->%d box %d->%d", partyBefore.Count, partyAfter.Count, boxBefore.Count, boxAfter.Count)
	}
	if got := partyAfter.Mons[len(partyAfter.Mons)-1].Species; got != wantSpecies {
		t.Fatalf("withdrawn species = %#02x, want %#02x", got, wantSpecies)
	}
}
