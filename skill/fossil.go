package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	cinnabarFossilRoomMap uint8 = 0xAA
	fossilScientistX     uint8 = 5
	fossilScientistY     uint8 = 2

	oldAmberItem    uint8 = 0x1F
	domeFossilItem  uint8 = 0x29
	helixFossilItem uint8 = 0x2A

	kabutoSpecies     uint8 = 0x5A
	omanyteSpecies    uint8 = 0x62
	aerodactylSpecies uint8 = 0xAB
)

const fossilRevivalPlace = "cinnabar lab fossil revival"

func init() {
	interactionPlaces[fossilRevivalPlace] = Destination{Map: cinnabarFossilRoomMap, X: 2, Y: 6}
}

// FossilRevivalPlace is the semantic Cinnabar Lab destination used by Dex
// planning. It is interaction-owned rather than a generic exploration stop.
func FossilRevivalPlace() string { return fossilRevivalPlace }

// IsFossilRevivalActor keeps the fossil scientist out of generic Talk. His
// interaction consumes a finite item, opens a filtered menu and starts a
// multi-map one-time acquisition transaction.
func IsFossilRevivalActor(mapID, x, y uint8) bool {
	return mapID == cinnabarFossilRoomMap && x == fossilScientistX && y == fossilScientistY
}

func fossilItemForSpecies(species uint8) (uint8, bool) {
	switch species {
	case kabutoSpecies:
		return domeFossilItem, true
	case omanyteSpecies:
		return helixFossilItem, true
	case aerodactylSpecies:
		return oldAmberItem, true
	default:
		return 0, false
	}
}

// ReviveFossil executes Red's complete Cinnabar Lab fossil lifecycle. The lab
// only finishes revival after the player leaves the lab and reaches Cinnabar
// Island, so this method deliberately performs that map transition and returns
// for the handoff before reporting success.
func ReviveFossil(m *emu.Emu, romData []byte, species uint8, policy MovePolicy) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: nil policy")
	}
	item, ok := fossilItemForSpecies(species)
	if !ok {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: species %#02x is not a Red fossil revival", species)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	if giftPokemonAlreadyOwned(&before, romData, species) {
		return CatchResult{Outcome: OutcomeCaught, Species: species}, nil
	}
	beforeItem := bagCount(state.DecodeInventory(&before).Items, item)
	if beforeItem <= 0 {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: required fossil item %#02x is not in the bag", item)
	}

	dest, _ := Place(fossilRevivalPlace)
	if _, err := TravelFlee(m, romData, dest, policy, 80); err != nil {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: reach Cinnabar fossil room: %w", err)
	}

	menuIndex, err := fossilFilteredMenuIndex(m, item)
	if err != nil {
		return CatchResult{}, err
	}
	_, talkErr := TalkAt(m, romData, fossilScientistX, fossilScientistY, policy)
	var menuErr *ErrTalkMenu
	if !errors.As(talkErr, &menuErr) {
		if talkErr == nil {
			return CatchResult{}, fmt.Errorf("skill: ReviveFossil: scientist did not open fossil selection menu")
		}
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: open fossil selection: %w", talkErr)
	}
	if err := SelectMenuItem(m, menuIndex); err != nil {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: choose fossil menu entry %d: %w", menuIndex, err)
	}

	if err := recoverToOwnedChoice(m, "confirm fossil handoff"); err != nil {
		return CatchResult{}, err
	}
	if err := selectTwoOption(m, 0); err != nil { // YES
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: confirm fossil handoff: %w", err)
	}
	if err := recoverOwnedDialogue(m, "finish fossil handoff"); err != nil {
		return CatchResult{}, err
	}

	var handed state.Mem
	state.Snapshot(m, &handed)
	afterItem := bagCount(state.DecodeInventory(&handed).Items, item)
	if afterItem != beforeItem-1 {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: fossil item %#02x count was %d before and %d after handoff", item, beforeItem, afterItem)
	}

	island, ok := Place("cinnabar island")
	if !ok {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: cinnabar island destination is not registered")
	}
	if _, err := TravelFlee(m, romData, island, policy, 80); err != nil {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: leave lab to finish revival: %w", err)
	}
	if _, err := TravelFlee(m, romData, dest, policy, 80); err != nil {
		return CatchResult{}, fmt.Errorf("skill: ReviveFossil: return to fossil room: %w", err)
	}

	return receiveGiftPokemonAt(m, romData, policy, giftPokemonSpec{
		Name:    "fossil revival",
		Map:     cinnabarFossilRoomMap,
		X:       fossilScientistX,
		Y:       fossilScientistY,
		Species: species,
	})
}

// fossilFilteredMenuIndex mirrors Lab4Script_GetFossilsInBag: the menu contains
// only bag-present fossils, always in DOME, HELIX, OLD AMBER order.
func fossilFilteredMenuIndex(m *emu.Emu, want uint8) (int, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	items := state.DecodeInventory(&mem).Items
	index := 0
	for _, item := range []uint8{domeFossilItem, helixFossilItem, oldAmberItem} {
		if bagCount(items, item) <= 0 {
			continue
		}
		if item == want {
			return index, nil
		}
		index++
	}
	return -1, fmt.Errorf("skill: ReviveFossil: fossil item %#02x disappeared before selection", want)
}

func recoverToOwnedChoice(m *emu.Emu, label string) error {
	res := RecoverDialogue(m, dialogueRecoveryBudget)
	if res.Stop == DialogueChoiceRequired {
		return nil
	}
	return fmt.Errorf("skill: ReviveFossil: %s: expected choice, recovery stopped with %d (%q)", label, res.Stop, res.Text)
}

func recoverOwnedDialogue(m *emu.Emu, label string) error {
	res := RecoverDialogue(m, dialogueRecoveryBudget)
	if res.Stop == DialogueRecovered {
		return nil
	}
	return fmt.Errorf("skill: ReviveFossil: %s: recovery stopped with %d (%q)", label, res.Stop, res.Text)
}
