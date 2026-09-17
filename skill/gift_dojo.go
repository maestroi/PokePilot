package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	fightingDojoMap       uint8 = 0xB1
	hitmonleeGiftSpecies  uint8 = 0x2B
	hitmonchanGiftSpecies uint8 = 0x2C
	hitmonleeGiftX        uint8 = 4
	hitmonleeGiftY        uint8 = 1
	hitmonchanGiftX       uint8 = 5
	hitmonchanGiftY       uint8 = 1
)

func fightingDojoGiftDestination(x, y uint8) Destination {
	return Destination{Map: fightingDojoMap, X: x, Y: y + 1}
}

func init() {
	// Keep the mutually exclusive prize balls interaction-owned. The dedicated
	// gift skill owns travel to the adjacent stand tile (and any trainer battles
	// on the way), then answers the prize confirmation exactly once.
	interactionPlaces["fighting dojo hitmonlee"] = fightingDojoGiftDestination(hitmonleeGiftX, hitmonleeGiftY)
	interactionPlaces["fighting dojo hitmonchan"] = fightingDojoGiftDestination(hitmonchanGiftX, hitmonchanGiftY)
}

// ReceiveFightingDojoGift consumes exactly one Fighting Dojo prize. The ROM
// asks YES/NO before GivePokemon and then asks the normal nickname question;
// the shared gift driver intentionally answers YES to the former and NO to the
// latter. Once either species is owned the other branch is permanently
// unavailable in this save, matching the catalog's fighting_dojo exclusivity.
func ReceiveFightingDojoGift(m *emu.Emu, romData []byte, policy MovePolicy, species uint8) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: ReceiveFightingDojoGift: nil policy")
	}

	var (
		x, y  uint8
		other uint8
		name  string
	)
	switch species {
	case hitmonleeGiftSpecies:
		x, y, other, name = hitmonleeGiftX, hitmonleeGiftY, hitmonchanGiftSpecies, "ReceiveHitmonleeGift"
	case hitmonchanGiftSpecies:
		x, y, other, name = hitmonchanGiftX, hitmonchanGiftY, hitmonleeGiftSpecies, "ReceiveHitmonchanGift"
	default:
		return CatchResult{}, fmt.Errorf("skill: ReceiveFightingDojoGift: unsupported species %#02x", species)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if giftPokemonAlreadyOwned(&mem, romData, species, ram(m)) {
		return CatchResult{Outcome: OutcomeCaught, Species: species}, nil
	}
	if giftPokemonAlreadyOwned(&mem, romData, other, ram(m)) {
		return CatchResult{}, fmt.Errorf("skill: %s: the mutually exclusive Fighting Dojo prize was already consumed", name)
	}

	// Like Eevee, Dojo gift objectives skip agent-level habitat travel because
	// scripted gifts own their route. Finish that contract here instead of
	// requiring the executor to have been entered on map 0xB1 already.
	if _, err := TravelFlee(m, romData, fightingDojoGiftDestination(x, y), policy, 40); err != nil {
		return CatchResult{}, fmt.Errorf("skill: %s: reach gift: %w", name, err)
	}

	return receiveGiftPokemonAt(m, romData, policy, giftPokemonSpec{
		Name:          name,
		Map:           fightingDojoMap,
		X:             x,
		Y:             y,
		Species:       species,
		ConfirmChoice: true,
	})
}
