package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestPokegearRadioGateRequiresBothCards(t *testing.T) {
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		var mem fakeMemory
		state := profile.DecodeWorldOpportunities(&mem)
		if len(state.RouteGates) != 1 {
			t.Fatalf("%s: missing Snorlax gate: %+v", profile.ID(), state)
		}
		gate := state.RouteGates[0]
		if gate.Available || gate.Transition.ID != gsSnorlaxGateID ||
			!gate.Transition.Gate || !gate.Transition.ChoiceRequired ||
			gate.Transition.From == "" || gate.Transition.To == "" {
			t.Fatalf("%s: incorrect Snorlax transition %+v", profile.ID(), gate)
		}
		if len(gate.Missing) != 2 || gate.Missing[0] != gsCapRadioCard || gate.Missing[1] != gsCapExpansionCard {
			t.Fatalf("%s: missing flags = %v, want radio and expansion cards", profile.ID(), gate.Missing)
		}

		mem[sym.PokegearFlags] = gsPokegearRadioCardMask
		state = profile.DecodeWorldOpportunities(&mem)
		if len(state.Capabilities) != 1 || state.Capabilities[0] != gsCapRadioCard {
			t.Fatalf("%s: radio ownership not projected: %+v", profile.ID(), state)
		}
		if state.RouteGates[0].Available || len(state.RouteGates[0].Missing) != 1 ||
			state.RouteGates[0].Missing[0] != gsCapExpansionCard {
			t.Fatalf("%s: basic radio alone opened Snorlax: %+v", profile.ID(), state.RouteGates[0])
		}

		mem[sym.PokegearFlags] |= gsPokegearExpansionCardMask
		state = profile.DecodeWorldOpportunities(&mem)
		if !state.RouteGates[0].Available || len(state.RouteGates[0].Missing) != 0 {
			t.Fatalf("%s: both cards did not unlock route prerequisites: %+v", profile.ID(), state.RouteGates[0])
		}
		mem[sym.PokegearFlags] = 0
		mem[sym.EventFlags+gsEventFoughtSnorlax/8] |= 1 << uint(gsEventFoughtSnorlax%8)
		if gates := profile.DecodeWorldOpportunities(&mem).RouteGates; len(gates) != 0 {
			t.Fatalf("%s: already cleared Snorlax reappeared: %+v", profile.ID(), gates)
		}
	}
}

func TestPokegearRegisteredNumbersYieldStructuredRematchSources(t *testing.T) {
	var mem fakeMemory
	mem[sym.PokegearFlags] = gsPokegearPhoneCardMask
	mem[sym.PhoneList] = 15 // PHONE_YOUNGSTER_JOEY
	mem[sym.PhoneList+1] = 16 // PHONE_BUG_CATCHER_WADE
	// A ready event without the number must never make an offered battle.
	mem[sym.EventFlags+gsRematchContacts[0].ReadyEvent/8] |= 1 << uint(gsRematchContacts[0].ReadyEvent%8)

	state := NewGold().DecodeWorldOpportunities(&mem)
	if len(state.PhoneContacts) != 2 || len(state.BattleSources) != 2 {
		t.Fatalf("registered phone contacts = %+v; battle sources = %+v", state.PhoneContacts, state.BattleSources)
	}
	if state.PhoneContacts[0].ID != "joey" || state.PhoneContacts[1].ID != "wade" {
		t.Fatalf("phone list order not semantic: %+v", state.PhoneContacts)
	}
	for _, source := range state.BattleSources {
		if source.Kind != game.BattleSourcePhoneRematch || source.Location == "" || source.Ready {
			t.Fatalf("premature or unlocated rematch: %+v", source)
		}
	}

	mem[sym.EventFlags+628/8] |= 1 << uint(628%8) // EVENT_JOEY_READY_FOR_REMATCH
	state = NewSilver().DecodeWorldOpportunities(&mem)
	if state.BattleSources[0].ID != "phone_rematch:joey" || !state.BattleSources[0].Ready ||
		state.BattleSources[1].Ready {
		t.Fatalf("ready flag should unlock Joey only: %+v", state.BattleSources)
	}
	mem[sym.PhoneList] = 0
	state = NewSilver().DecodeWorldOpportunities(&mem)
	if len(state.BattleSources) != 1 || state.BattleSources[0].Trainer != "Wade" {
		t.Fatalf("removed number still supplies rematch: %+v", state.BattleSources)
	}
}

func TestPokegearNoPhoneCardOrNilReader(t *testing.T) {
	var mem fakeMemory
	mem[sym.PhoneList] = 15
	mem[sym.PokegearFlags] = gsPokegearRadioCardMask
	world := NewGold().DecodeWorldOpportunities(&mem)
	if len(world.PhoneContacts) != 0 || len(world.BattleSources) != 0 {
		t.Fatalf("no phone card should not surface contacts: %+v", world)
	}
	if world := NewGold().DecodeWorldOpportunities(nil); len(world.RouteGates) != 0 ||
		len(world.Capabilities) != 0 || len(world.BattleSources) != 0 {
		t.Fatalf("nil reader invented state: %+v", world)
	}
}
