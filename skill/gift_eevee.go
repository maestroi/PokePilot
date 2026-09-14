package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	celadonMansionRoofHouseMap uint8 = 0x84
	eeveeGiftSpecies           uint8 = 0x66
	eeveeGiftX                 uint8 = 4
	eeveeGiftY                 uint8 = 3
	giftPokemonBudget                = 6000
)

func init() {
	// Keep the gift itself out of ordinary PlaceNames: travelling to the room
	// without consuming the one-time script is not a useful standalone goal.
	// Dex execution can still resolve this interaction-owned destination.
	interactionPlaces["celadon mansion eevee"] = Destination{
		Map: celadonMansionRoofHouseMap,
		X:   eeveeGiftX,
		Y:   eeveeGiftY + 1,
	}
}

// ReceiveEeveeGift consumes Celadon Mansion's one-time Eevee Poké Ball gift.
// The interaction is deliberately not generic Talk: GivePokemon can ask for a
// nickname, whose default YES would enter the naming keyboard and recreate the
// historical AAAAA... naming bug if dialogue were advanced blindly with A.
// This driver recognizes that exact two-option prompt, chooses NO, and returns
// only after Eevee ownership is positively visible in party/box/Pokédex state.
func ReceiveEeveeGift(m *emu.Emu, romData []byte, policy MovePolicy) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveEeveeGift: nil policy")
	}
	if got := m.Peek8(sym.CurMap); got != celadonMansionRoofHouseMap {
		return CatchResult{}, fmt.Errorf("skill: ReceiveEeveeGift: expected Celadon Mansion roof house %#04x, on %#04x", celadonMansionRoofHouseMap, got)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	partyBefore := int(state.DecodeParty(&before).Count)
	boxBefore := int(state.DecodeBox(&before).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&before).Owned...)
	want := []uint8{eeveeGiftSpecies}
	wantDex := wantedDexNumbers(romData, want)
	if species, ok := catchAcquiredWanted(partyBefore, state.DecodeParty(&before), boxBefore, state.DecodeBox(&before), nil, state.DecodePokedex(&before).Owned, want, wantDex); ok {
		return CatchResult{Outcome: OutcomeCaught, Species: species}, nil
	}

	if err := talkBeside(m, romData, eeveeGiftX, eeveeGiftY, policy); err != nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveEeveeGift: approach gift: %w", err)
	}
	if err := Face(m, eeveeGiftX, eeveeGiftY); err != nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveEeveeGift: face gift: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	res := CatchResult{}
	for spent := 0; spent < giftPokemonBudget; spent += 10 {
		var mem state.Mem
		state.Snapshot(m, &mem)

		if state.DecodeTwoOptionMenu(&mem) != nil {
			// GivePokemon's nickname prompt defaults to YES. Always keep the
			// species' canonical name for deterministic collection runs.
			if err := selectTwoOption(m, 1); err != nil {
				return res, fmt.Errorf("skill: ReceiveEeveeGift: decline nickname prompt: %w", err)
			}
			continue
		}

		party := state.DecodeParty(&mem)
		box := state.DecodeBox(&mem)
		owned := state.DecodePokedex(&mem).Owned
		species, acquired := catchAcquiredWanted(partyBefore, party, boxBefore, box, ownedBefore, owned, want, wantDex)
		if acquired && state.Controllable(&mem) {
			res.Outcome = OutcomeCaught
			res.Species = species
			return res, nil
		}

		if state.MenuUp(&mem) {
			return res, fmt.Errorf("skill: ReceiveEeveeGift: unexpected menu while resolving gift: %q", state.ScreenText(&mem))
		}
		if mem.U8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		if state.Controllable(&mem) && spent >= 100 {
			return res, fmt.Errorf("skill: ReceiveEeveeGift: gift script returned without verified Eevee ownership")
		}
		m.StepFrames(10)
	}

	return res, fmt.Errorf("skill: ReceiveEeveeGift: gift script did not settle within %d frames", giftPokemonBudget)
}
