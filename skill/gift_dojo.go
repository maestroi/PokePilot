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

func init() {
	// Keep the mutually exclusive prize balls interaction-owned. Ordinary
	// routing may reach the adjacent stand tiles (and resolve the dojo's trainer
	// battles on the way), but only this skill is allowed to answer the prize
	// confirmation and consume one branch of the choice.
	interactionPlaces["fighting dojo hitmonlee"] = Destination{Map: fightingDojoMap, X: hitmonleeGiftX, Y: hitmonleeGiftY + 1}
	interactionPlaces["fighting dojo hitmonchan"] = Destination{Map: fightingDojoMap, X: hitmonchanGiftX, Y: hitmonchanGiftY + 1}
}

// ReceiveFightingDojoGift consumes exactly one Fighting Dojo prize. The ROM
// asks YES/NO before GivePokemon and then asks the normal nickname question;
// the shared gift driver intentionally answers YES to the former and NO to the
// latter. Once either species is owned the other branch is permanently
// unavailable in this save, matching the catalog's fighting_dojo exclusivity.
func ReceiveFightingDojoGift(m *emu.Emu, romData []byte, policy MovePolicy, species uint8) (CatchResult, error) {
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
	if giftPokemonAlreadyOwned(&mem, romData, species) {
		return CatchResult{Outcome: OutcomeCaught, Species: species}, nil
	}
	if giftPokemonAlreadyOwned(&mem, romData, other) {
		return CatchResult{}, fmt.Errorf("skill: %s: the mutually exclusive Fighting Dojo prize was already consumed", name)
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
