package skill

import (
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

func wantedDexNumbers(romData []byte, want []uint8) []uint8 {
	out := make([]uint8, 0, len(want))
	for _, species := range want {
		dex, err := rom.InternalSpeciesDexNumber(romData, species)
		if err != nil {
			continue
		}
		out = append(out, dex)
	}
	return out
}

// catchAcquiredWanted is the positive Catch postcondition: the wanted
// species is now in the party, the active box, or newly recorded as
// Pokédex-owned. Party growth is sufficient by itself so a duplicate
// hunt (needed later for evolution branches) still counts.
func catchAcquiredWanted(partyBefore int, party state.PartyState, boxBefore int, box state.BoxState, ownedBefore, ownedAfter, want, wantDex []uint8) (uint8, bool) {
	if int(party.Count) == partyBefore+1 && len(party.Mons) > 0 && speciesIn(party.Mons[len(party.Mons)-1].Species, want) {
		return party.Mons[len(party.Mons)-1].Species, true
	}
	if int(box.Count) == boxBefore+1 && len(box.Mons) > 0 && speciesIn(box.Mons[len(box.Mons)-1].Species, want) {
		return box.Mons[len(box.Mons)-1].Species, true
	}
	if newlyOwnedDex(ownedBefore, ownedAfter, wantDex) {
		if len(want) == 1 {
			return want[0], true
		}
		return 0, true
	}
	return 0, false
}

func newlyOwnedDex(before, after, wantDex []uint8) bool {
	had := dexSet(before)
	have := dexSet(after)
	for _, dex := range wantDex {
		if dex != 0 && have[dex] && !had[dex] {
			return true
		}
	}
	return false
}

func dexSet(nums []uint8) map[uint8]bool {
	out := make(map[uint8]bool, len(nums))
	for _, n := range nums {
		out[n] = true
	}
	return out
}
