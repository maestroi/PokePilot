package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const giftPokemonBudget = 6000

type giftPokemonSpec struct {
	Name          string
	Map           uint8
	X, Y          uint8
	Species       uint8
	ConfirmChoice bool
}

func giftPokemonAlreadyOwned(mem *state.Mem, romData []byte, species uint8) bool {
	wantDex := wantedDexNumbers(romData, []uint8{species})
	have := dexSet(state.DecodePokedex(mem).Owned)
	for _, dex := range wantDex {
		if dex != 0 && have[dex] {
			return true
		}
	}
	return false
}

// receiveGiftPokemonAt drives one GivePokemon-backed map interaction. Generic
// Talk is intentionally not used: GivePokemon may ask whether to nickname the
// gift, and blindly paging that two-option menu with A accepts YES and enters
// the naming keyboard. For gifts that first ask whether Red wants the Pokemon,
// ConfirmChoice makes the first verified two-option selection YES and the
// later GivePokemon nickname selection NO. Success is positively verified via
// party, active box, or Pokédex ownership.
func receiveGiftPokemonAt(m *emu.Emu, romData []byte, policy MovePolicy, spec giftPokemonSpec) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: %s: nil policy", spec.Name)
	}
	if got := m.Peek8(sym.CurMap); got != spec.Map {
		return CatchResult{}, fmt.Errorf("skill: %s: expected map %#04x, on %#04x", spec.Name, spec.Map, got)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	if giftPokemonAlreadyOwned(&before, romData, spec.Species) {
		return CatchResult{Outcome: OutcomeCaught, Species: spec.Species}, nil
	}
	partyBefore := int(state.DecodeParty(&before).Count)
	boxBefore := int(state.DecodeBox(&before).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&before).Owned...)
	want := []uint8{spec.Species}
	wantDex := wantedDexNumbers(romData, want)

	if err := talkBeside(m, romData, spec.X, spec.Y, policy); err != nil {
		return CatchResult{}, fmt.Errorf("skill: %s: approach gift: %w", spec.Name, err)
	}
	if err := Face(m, spec.X, spec.Y); err != nil {
		return CatchResult{}, fmt.Errorf("skill: %s: face gift: %w", spec.Name, err)
	}
	m.Tap(emu.A, 3, 7)

	res := CatchResult{}
	confirmed := !spec.ConfirmChoice
	for spent := 0; spent < giftPokemonBudget; spent += 10 {
		var mem state.Mem
		state.Snapshot(m, &mem)

		if state.DecodeTwoOptionMenu(&mem) != nil {
			if !confirmed {
				// Choice-backed gifts such as the Fighting Dojo balls ask whether
				// Red wants this Pokemon before GivePokemon runs. Accept exactly
				// that first prompt, then treat every later choice as nickname.
				if err := selectTwoOption(m, 0); err != nil {
					return res, fmt.Errorf("skill: %s: accept gift choice: %w", spec.Name, err)
				}
				confirmed = true
				continue
			}
			// GivePokemon's nickname prompt defaults to YES. Keep canonical
			// species names for deterministic collection runs.
			if err := selectTwoOption(m, 1); err != nil {
				return res, fmt.Errorf("skill: %s: decline nickname prompt: %w", spec.Name, err)
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
			return res, fmt.Errorf("skill: %s: unexpected menu while resolving gift: %q", spec.Name, state.ScreenText(&mem))
		}
		if mem.U8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		if state.Controllable(&mem) && spent >= 100 {
			return res, fmt.Errorf("skill: %s: gift script returned without verified species %#02x ownership", spec.Name, spec.Species)
		}
		m.StepFrames(10)
	}

	return res, fmt.Errorf("skill: %s: gift script did not settle within %d frames", spec.Name, giftPokemonBudget)
}
