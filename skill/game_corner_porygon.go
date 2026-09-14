package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	gameCornerMap          uint8 = 0x87
	gameCornerPrizeRoomMap uint8 = 0x89
	celadonDinerMap        uint8 = 0x8A

	coinCaseItem   uint8 = 0x45
	porygonSpecies uint8 = 0xAA
	porygonCost          = 9999
	coinPurchaseSize     = 50
	coinPurchaseYen      = 1000

	coinCaseGiverX uint8 = 0
	coinCaseGiverY uint8 = 1
	coinClerkX     uint8 = 5
	coinClerkY     uint8 = 6
	porygonVendorX uint8 = 4
	porygonVendorY uint8 = 2
)

const (
	gameCornerCoinCasePlace = "celadon diner coin case"
	gameCornerCoinClerkPlace = "game corner coin clerk"
	gameCornerPorygonPlace = "game corner porygon prize"
)

func init() {
	interactionPlaces[gameCornerCoinCasePlace] = Destination{Map: celadonDinerMap, X: 0, Y: 2}
	interactionPlaces[gameCornerCoinClerkPlace] = Destination{Map: gameCornerMap, X: 5, Y: 7}
	interactionPlaces[gameCornerPorygonPlace] = Destination{Map: gameCornerPrizeRoomMap, X: 4, Y: 3}
}

// PorygonPrizePlace is the semantic destination used by Dex planning.
func PorygonPrizePlace() string { return gameCornerPorygonPlace }

// IsGameCornerDexActor keeps the coin clerk out of generic Talk. Buying coins
// is a money-spending YES/NO transaction owned by the Porygon acquisition path.
func IsGameCornerDexActor(mapID, x, y uint8) bool {
	return mapID == gameCornerMap && x == coinClerkX && y == coinClerkY
}

// MaxPorygonCoinBudget is the worst-case cash required when the Coin Case is
// empty. The ROM adds 50 coins for each Y1000 purchase and AddBCD saturates an
// overflowing two-byte counter to 9999, so 200 purchases are sufficient.
func MaxPorygonCoinBudget() uint32 { return 200 * coinPurchaseYen }

func gameCornerMoney(mem *state.Mem) uint32 {
	return state.DecodeInventory(mem).Money
}

func ensureCoinCase(m *emu.Emu, romData []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if bagHasItem(&mem, coinCaseItem) {
		return nil
	}
	if err := EnsureBagSpaceFor(m, coinCaseItem); err != nil {
		return fmt.Errorf("skill: Porygon: make room for Coin Case: %w", err)
	}
	dest, ok := Place(gameCornerCoinCasePlace)
	if !ok {
		return fmt.Errorf("skill: Porygon: Coin Case destination is not registered")
	}
	if _, err := TravelFlee(m, romData, dest, policy, 80); err != nil {
		return fmt.Errorf("skill: Porygon: reach Coin Case giver: %w", err)
	}
	state.Snapshot(m, &mem)
	before := bagCount(state.DecodeInventory(&mem).Items, coinCaseItem)
	if _, err := TalkAt(m, romData, coinCaseGiverX, coinCaseGiverY, policy); err != nil {
		return fmt.Errorf("skill: Porygon: receive Coin Case: %w", err)
	}
	state.Snapshot(m, &mem)
	after := bagCount(state.DecodeInventory(&mem).Items, coinCaseItem)
	if after != before+1 {
		return fmt.Errorf("skill: Porygon: Coin Case count was %d before and %d after", before, after)
	}
	return nil
}

func buyPorygonCoins(m *emu.Emu, romData []byte, policy MovePolicy) error {
	dest, ok := Place(gameCornerCoinClerkPlace)
	if !ok {
		return fmt.Errorf("skill: Porygon: Game Corner coin clerk destination is not registered")
	}
	if _, err := TravelFlee(m, romData, dest, policy, 80); err != nil {
		return fmt.Errorf("skill: Porygon: reach Game Corner coin clerk: %w", err)
	}

	for purchases := 0; purchases < 200; purchases++ {
		var before state.Mem
		state.Snapshot(m, &before)
		coinsBefore := state.DecodeCoins(&before)
		if coinsBefore >= porygonCost {
			return nil
		}
		// The clerk refuses another purchase at 9990..9998. Our deterministic
		// purchase path never creates those values: from any value below 9990,
		// a +50 overflow saturates to 9999. Treat a pre-existing edge state as
		// blocked rather than gambling to perturb the counter.
		if coinsBefore >= 9990 {
			return fmt.Errorf("skill: Porygon: Coin Case is %d; clerk refuses another deterministic purchase", coinsBefore)
		}
		moneyBefore := gameCornerMoney(&before)
		if moneyBefore < coinPurchaseYen {
			return fmt.Errorf("skill: Porygon: need Y%d more Game Corner coin purchase money (have Y%d)", coinPurchaseYen, moneyBefore)
		}
		if _, err := TalkAtChoice(m, romData, coinClerkX, coinClerkY, 0, policy); err != nil {
			return fmt.Errorf("skill: Porygon: buy Game Corner coins: %w", err)
		}
		var after state.Mem
		state.Snapshot(m, &after)
		coinsAfter := state.DecodeCoins(&after)
		moneyAfter := gameCornerMoney(&after)
		if coinsAfter <= coinsBefore || moneyAfter+coinPurchaseYen != moneyBefore {
			return fmt.Errorf("skill: Porygon: coin purchase did not verify: coins %d->%d money %d->%d", coinsBefore, coinsAfter, moneyBefore, moneyAfter)
		}
		if coinsAfter != porygonCost && coinsAfter-coinsBefore != coinPurchaseSize {
			return fmt.Errorf("skill: Porygon: unexpected coin delta %d->%d", coinsBefore, coinsAfter)
		}
	}
	return fmt.Errorf("skill: Porygon: coin purchase budget exhausted at %d coins", currentGameCornerCoins(m))
}

func currentGameCornerCoins(m *emu.Emu) int {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeCoins(&mem)
}

func redeemPorygon(m *emu.Emu, romData []byte, policy MovePolicy) (CatchResult, error) {
	var before state.Mem
	state.Snapshot(m, &before)
	if giftPokemonAlreadyOwned(&before, romData, porygonSpecies) {
		return CatchResult{Outcome: OutcomeCaught, Species: porygonSpecies}, nil
	}
	if state.DecodeCoins(&before) < porygonCost {
		return CatchResult{}, fmt.Errorf("skill: Porygon: prize costs %d coins; have %d", porygonCost, state.DecodeCoins(&before))
	}

	dest, ok := Place(gameCornerPorygonPlace)
	if !ok {
		return CatchResult{}, fmt.Errorf("skill: Porygon: prize room destination is not registered")
	}
	if _, err := TravelFlee(m, romData, dest, policy, 80); err != nil {
		return CatchResult{}, fmt.Errorf("skill: Porygon: reach prize room: %w", err)
	}

	state.Snapshot(m, &before)
	partyBefore := int(state.DecodeParty(&before).Count)
	boxBefore := int(state.DecodeBox(&before).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&before).Owned...)
	want := []uint8{porygonSpecies}
	wantDex := wantedDexNumbers(romData, want)

	_, talkErr := TalkAt(m, romData, porygonVendorX, porygonVendorY, policy)
	var menuErr *ErrTalkMenu
	if !errors.As(talkErr, &menuErr) {
		if talkErr == nil {
			return CatchResult{}, fmt.Errorf("skill: Porygon: second prize vendor did not open prize menu")
		}
		return CatchResult{}, fmt.Errorf("skill: Porygon: open prize menu: %w", talkErr)
	}
	if err := SelectMenuItem(m, 2); err != nil { // DRATINI, SCYTHER, PORYGON
		return CatchResult{}, fmt.Errorf("skill: Porygon: select Porygon prize: %w", err)
	}
	if err := recoverToOwnedChoice(m, "confirm Porygon prize"); err != nil {
		return CatchResult{}, err
	}
	if err := selectTwoOption(m, 0); err != nil {
		return CatchResult{}, fmt.Errorf("skill: Porygon: confirm prize: %w", err)
	}

	res := CatchResult{}
	for spent := 0; spent < giftPokemonBudget; spent += 10 {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeTwoOptionMenu(&mem) != nil {
			// GivePokemon's only choice here is the nickname prompt.
			if err := selectTwoOption(m, 1); err != nil {
				return res, fmt.Errorf("skill: Porygon: decline nickname: %w", err)
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
			return res, fmt.Errorf("skill: Porygon: unexpected menu after prize selection: %q", state.ScreenText(&mem))
		}
		if mem.U8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		if state.Controllable(&mem) && spent >= 100 {
			return res, fmt.Errorf("skill: Porygon: prize script returned without verified ownership")
		}
		m.StepFrames(10)
	}
	return res, fmt.Errorf("skill: Porygon: prize script did not settle within %d frames", giftPokemonBudget)
}

// ReceivePorygonPrize performs the complete deterministic Red acquisition path:
// obtain the Coin Case, buy enough coins, then redeem Porygon. It never plays
// slots; coin purchases exploit Red's normal saturating BCD counter behavior.
func ReceivePorygonPrize(m *emu.Emu, romData []byte, policy MovePolicy) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: Porygon: nil policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if giftPokemonAlreadyOwned(&mem, romData, porygonSpecies) {
		return CatchResult{Outcome: OutcomeCaught, Species: porygonSpecies}, nil
	}
	if err := ensureCoinCase(m, romData, policy); err != nil {
		return CatchResult{}, err
	}
	if err := buyPorygonCoins(m, romData, policy); err != nil {
		return CatchResult{}, err
	}
	return redeemPorygon(m, romData, policy)
}
