package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	laprasGiftSpecies uint8 = 0x13
	laprasGiftX       uint8 = 1
	laprasGiftY       uint8 = 5
)

func init() {
	// This is an interaction-owned target, not a generic Silph waypoint. The
	// dedicated skill owns the Card Key/rival-room route and then approaches
	// the worker from live geometry.
	interactionPlaces["silph co lapras"] = Destination{
		Map: silphCo7FMap,
		X:   laprasGiftX,
		Y:   laprasGiftY + 1,
	}
}

// ReceiveLaprasGift reaches Silph Co. 7F through the already-audited Card Key
// / rival-room route, resolves the rival if necessary, and accepts the worker's
// one-time Lapras through the shared nickname-safe gift driver.
func ReceiveLaprasGift(m *emu.Emu, romData []byte, policy MovePolicy) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveLaprasGift: nil policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if giftPokemonAlreadyOwned(&mem, romData, laprasGiftSpecies, ram(m)) {
		return CatchResult{Outcome: OutcomeCaught, Species: laprasGiftSpecies}, nil
	}

	facts := currentSilphFacts(m)
	if !facts.SaffronGateOpen {
		return CatchResult{}, fmt.Errorf("skill: ReceiveLaprasGift: Saffron gate is not open")
	}
	if !facts.CardKeyOwned {
		return CatchResult{}, fmt.Errorf("skill: ReceiveLaprasGift: Card Key is not owned")
	}

	if !facts.SilphCoRivalDefeated {
		if err := reachSilphRivalRoom(m, romData, policy); err != nil {
			return CatchResult{}, fmt.Errorf("skill: ReceiveLaprasGift: reach rival room: %w", err)
		}
		if err := resolveSilphRival(m, romData, policy); err != nil {
			return CatchResult{}, fmt.Errorf("skill: ReceiveLaprasGift: resolve Silph rival: %w", err)
		}
	} else if m.Peek8(ram(m).CurMap) != silphCo7FMap {
		if err := reachSilphRivalRoom(m, romData, policy); err != nil {
			return CatchResult{}, fmt.Errorf("skill: ReceiveLaprasGift: return to rival room: %w", err)
		}
	}

	return receiveGiftPokemonAt(m, romData, policy, giftPokemonSpec{
		Name:    "ReceiveLaprasGift",
		Map:     silphCo7FMap,
		X:       laprasGiftX,
		Y:       laprasGiftY,
		Species: laprasGiftSpecies,
	})
}
