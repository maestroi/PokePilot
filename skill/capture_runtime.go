package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func captureProfileFor(m *emu.Emu) (game.CaptureProfile, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: capture: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: capture: detect profile: %w", err)
	}
	capture, ok := profile.(game.CaptureProfile)
	if !ok {
		return nil, fmt.Errorf("skill: capture: profile %s@%s does not expose capture semantics", profile.ID(), profile.Revision())
	}
	return capture, nil
}

func nativeSpeciesList(want []uint8) []uint16 {
	out := make([]uint16, len(want))
	for i, species := range want {
		out[i] = uint16(species)
	}
	return out
}

func wantedDexNumbersWithProfile(profile game.CaptureProfile, romData []byte, want []uint8) []uint16 {
	out := make([]uint16, 0, len(want))
	for _, species := range want {
		if dex, ok := profile.CaptureDexNumber(romData, uint16(species)); ok && dex != 0 {
			out = append(out, dex)
		}
	}
	return out
}

func captureAcquiredWantedState(before, after game.CaptureState, want, wantDex []uint16) (uint16, bool) {
	if len(after.PartySpecies) == len(before.PartySpecies)+1 && len(after.PartySpecies) > 0 {
		species := after.PartySpecies[len(after.PartySpecies)-1]
		if nativeSpeciesIn(species, want) {
			return species, true
		}
	}
	if len(after.ActiveBoxSpecies) == len(before.ActiveBoxSpecies)+1 && len(after.ActiveBoxSpecies) > 0 {
		species := after.ActiveBoxSpecies[len(after.ActiveBoxSpecies)-1]
		if nativeSpeciesIn(species, want) {
			return species, true
		}
	}
	if newlyOwnedDexNative(before.OwnedDex, after.OwnedDex, wantDex) {
		if len(want) == 1 {
			return want[0], true
		}
		return 0, true
	}
	return 0, false
}

func nativeSpeciesIn(species uint16, want []uint16) bool {
	for _, candidate := range want {
		if candidate == species {
			return true
		}
	}
	return false
}

func newlyOwnedDexNative(before, after, want []uint16) bool {
	had := make(map[uint16]bool, len(before))
	have := make(map[uint16]bool, len(after))
	for _, n := range before {
		had[n] = true
	}
	for _, n := range after {
		have[n] = true
	}
	for _, n := range want {
		if n != 0 && have[n] && !had[n] {
			return true
		}
	}
	return false
}

func inventoryItemQuantity(state game.InventoryState, nativeItemID uint16) int {
	for _, item := range state.Items {
		if item.NativeItemID == nativeItemID {
			return item.Quantity
		}
	}
	return 0
}

func ordinaryCaptureBall(state game.InventoryState, order []uint16) (uint16, bool) {
	for _, item := range order {
		if inventoryItemQuantity(state, item) > 0 {
			return item, true
		}
	}
	return 0, false
}

func ordinaryCaptureBallCount(state game.InventoryState, order []uint16) int {
	total := 0
	for _, item := range order {
		total += inventoryItemQuantity(state, item)
	}
	return total
}

func legacyItemID(native uint16) (uint8, error) {
	if native == 0 || native > 0xff {
		return 0, fmt.Errorf("skill: capture: native item id %#04x exceeds current item-execution range", native)
	}
	return uint8(native), nil
}

func legacySpeciesID(native uint16) (uint8, error) {
	if native == 0 || native > 0xff {
		return 0, fmt.Errorf("skill: capture: native species id %#04x exceeds current result range", native)
	}
	return uint8(native), nil
}
