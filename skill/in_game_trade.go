package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

const inGameTradeBudget = 18000

func init() {
	for _, site := range reddata.NPCTradeSites() {
		interactionPlaces[site.Place] = Destination{
			Map: site.Map,
			X:   site.TravelX,
			Y:   site.TravelY,
		}
	}
}

// InGameTrade executes one ROM-defined NPC trade that returns want. Red owns
// the actual swap, nickname/OT assignment, completion flag and animation; this
// controller owns only routing, accepting the offer, selecting the exact give
// species, and positively verifying the received species/Pokédex state.
func InGameTrade(m *emu.Emu, romData []byte, want uint8, policy MovePolicy) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: nil policy")
	}
	trades, err := rom.NPCTrades(romData)
	if err != nil {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: read trade table: %w", err)
	}
	var trade rom.NPCTrade
	var site reddata.NPCTradeSite
	found := false
	for _, candidate := range trades {
		if candidate.Get != want {
			continue
		}
		candidateSite, ok := reddata.NPCTradeSiteByIndex(candidate.Index)
		if !ok {
			continue
		}
		trade, site, found = candidate, candidateSite, true
		break
	}
	if !found {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: no used Red NPC trade returns species %#02x", want)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	if giftPokemonAlreadyOwned(&before, romData, want, ram(m)) {
		return CatchResult{Outcome: OutcomeCaught, Species: want}, nil
	}
	partyBefore := ram(m).DecodeParty(&before)
	giveSlot := -1
	for i, mon := range partyBefore.Mons {
		if mon.Species == trade.Give {
			giveSlot = i
			break
		}
	}
	if giveSlot < 0 {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: required give species %#02x is not in the party", trade.Give)
	}

	dest, ok := Place(site.Place)
	if !ok {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: trade site %q is not registered", site.Place)
	}
	if _, err := TravelFlee(m, romData, dest, policy, 80); err != nil {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: reach %s: %w", site.Place, err)
	}

	_, talkErr := TalkAt(m, romData, site.X, site.Y, policy)
	if talkErr == nil {
		// The completed-trade script is ordinary after-trade dialogue. It is
		// only success if the target's durable owned bit now proves it.
		var afterTalk state.Mem
		state.Snapshot(m, &afterTalk)
		if giftPokemonAlreadyOwned(&afterTalk, romData, want, ram(m)) {
			return CatchResult{Outcome: OutcomeCaught, Species: want}, nil
		}
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: NPC dialogue ended without offering or completing species %#02x", want)
	}
	var menuErr *ErrTalkMenu
	if !errors.As(talkErr, &menuErr) {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: open trade offer: %w", talkErr)
	}

	var offer state.Mem
	state.Snapshot(m, &offer)
	if ram(m).DecodeTwoOptionMenu(&offer) == nil {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: expected YES/NO trade offer, got %q", state.ScreenText(&offer))
	}
	if err := selectTwoOption(m, 0); err != nil { // YES
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: accept offer: %w", err)
	}

	if _, err := m.StepUntil(pcTransitionBudget, func(m *emu.Emu) bool {
		return normalPartyMenuUp(m)
	}); err != nil {
		var mem state.Mem
		state.Snapshot(m, &mem)
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: party selection did not appear: %w; screen=%q", err, state.ScreenText(&mem))
	}
	if err := movePartyCursor(m, giveSlot); err != nil {
		return CatchResult{}, fmt.Errorf("skill: InGameTrade: select required party slot %d: %w", giveSlot, err)
	}
	m.Tap(emu.A, 3, 7)

	wantDex := wantedDexNumbers(romData, []uint8{want})
	beforeOwned := append([]uint8(nil), ram(m).DecodePokedex(&before).Owned...)
	for spent := 0; spent < inGameTradeBudget; spent += talkSettle {
		var mem state.Mem
		state.Snapshot(m, &mem)
		party := ram(m).DecodeParty(&mem)
		owned := ram(m).DecodePokedex(&mem).Owned
		acquired := speciesInParty(party, want) || newlyOwnedDex(beforeOwned, owned, wantDex)
		if acquired && ram(m).Controllable(&mem) {
			return CatchResult{Outcome: OutcomeCaught, Species: want}, nil
		}
		if ram(m).Controllable(&mem) && spent >= 200 {
			return CatchResult{}, fmt.Errorf("skill: InGameTrade: trade returned to overworld without species %#02x", want)
		}
		if ram(m).DecodeTwoOptionMenu(&mem) != nil {
			return CatchResult{}, fmt.Errorf("skill: InGameTrade: unexpected choice after party selection: %q", state.ScreenText(&mem))
		}
		if mem.U8(ram(m).FontLoaded) != 0 && !ram(m).MenuUp(&mem) {
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
			continue
		}
		m.StepFrames(talkSettle)
	}
	return CatchResult{}, fmt.Errorf("skill: InGameTrade: trade did not settle within %d frames", inGameTradeBudget)
}

func speciesInParty(party state.PartyState, want uint8) bool {
	for _, mon := range party.Mons {
		if mon.Species == want {
			return true
		}
	}
	return false
}
