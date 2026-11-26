package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGoldSilverPokegearChannel20IsObservedNotInvented(t *testing.T) {
	if sym.RadioTuningKnob != sym.PokegearFlags+1 {
		t.Fatalf("wRadioTuningKnob address drift: %#04x", sym.RadioTuningKnob)
	}
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		var mem fakeMemory
		if s := p.DecodePokegearRadio(&mem); s.PokeFluteTuned() || s.HasRadio || s.HasExpansion {
			t.Fatalf("%s: fresh game falsely tuned channel 20: %+v", p.ID(), s)
		}
		mem[sym.PokegearFlags] = gsPokegearRadioCardMask
		mem[sym.JumptableIndex] = gsPokegearRadioJoypad
		mem[sym.RadioTuningKnob] = gsPokeFluteFrequency
		if p.DecodePokegearRadio(&mem).PokeFluteTuned() {
			t.Fatalf("%s: radio without Kanto expansion card falsely tuned Poké Flute", p.ID())
		}
		mem[sym.PokegearFlags] |= gsPokegearExpansionCardMask
		mem[sym.RadioTuningKnob] = gsPokeFluteFrequency - 2
		if p.DecodePokegearRadio(&mem).PokeFluteTuned() {
			t.Fatalf("%s: nearby station falsely counted as channel 20", p.ID())
		}
		mem[sym.RadioTuningKnob] = gsPokeFluteFrequency
		if s := p.DecodePokegearRadio(&mem); !s.PokeFluteTuned() {
			t.Fatalf("%s: station 20 with both cards not recognized: %+v", p.ID(), s)
		}
		mem[sym.JumptableIndex] = gsPokegearClockJoypad
		if p.DecodePokegearRadio(&mem).PokeFluteTuned() {
			t.Fatalf("%s: clock page with stale tuning knob counted as active radio", p.ID())
		}
		if s := p.DecodePokegearRadio(nil); s.HasRadio || s.PokeFluteTuned() {
			t.Fatalf("%s: nil reader manufactured tuning: %+v", p.ID(), s)
		}
	}
}

func TestGoldSilverStartMenuPokegearRequiresRealEntryAndCountsOptionalRows(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		var mem fakeMemory
		mem[sym.StatusFlags] = 1 // Pokédex
		mem[sym.PartyCount] = 2
		mem[sym.PokegearFlags] = gsPokegearRadioCardMask
		putGSText(&mem, "PACK SAVE OPTION EXIT POKEGEAR")
		if i, ok := p.StartMenuEntryIndex(&mem, game.StartMenuPokegear); ok {
			t.Fatalf("%s: fake label without obtained flag exposed Pokégear at %d", p.ID(), i)
		}
		mem[sym.PokegearFlags] |= 1 << 7
		if i, ok := p.StartMenuEntryIndex(&mem, game.StartMenuPokegear); !ok || i != 3 {
			t.Fatalf("%s: Pokégear after Dex, Pokémon, PACK = %d,%v; want 3", p.ID(), i, ok)
		}
		mem[sym.StatusFlags] = 0
		if i, ok := p.StartMenuEntryIndex(&mem, game.StartMenuPokegear); !ok || i != 2 {
			t.Fatalf("%s: menu without Dex = %d,%v; want 2", p.ID(), i, ok)
		}
		mem[sym.PartyCount] = 0
		if i, ok := p.StartMenuEntryIndex(&mem, game.StartMenuPokegear); !ok || i != 1 {
			t.Fatalf("%s: menu without Pokémon = %d,%v; want 1", p.ID(), i, ok)
		}
		putGSText(&mem, "SAVE OPTION EXIT POKEGEAR") // no PACK in contest/link
		if i, ok := p.StartMenuEntryIndex(&mem, game.StartMenuPokegear); !ok || i != 0 {
			t.Fatalf("%s: menu without PACK = %d,%v; want 0", p.ID(), i, ok)
		}
		putGSText(&mem, "PACK SAVE OPTION EXIT")
		if i, ok := p.StartMenuEntryIndex(&mem, game.StartMenuPokegear); ok {
			t.Fatalf("%s: stale obtained flag with no visible GEAR exposed entry %d", p.ID(), i)
		}
	}
}
